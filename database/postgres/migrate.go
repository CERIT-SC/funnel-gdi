package postgres

import (
	"context"
	"embed"
	"fmt"
	"io/fs"
	"path"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"
)

// migrationFiles holds the schema migrations, migrations/NNNN_<name>.sql.
//
//go:embed migrations/*.sql
var migrationFiles embed.FS

// migrationLock is the PostgreSQL advisory lock key that keeps two Funnel
// servers from migrating the same database at the same time.
const migrationLock = 0x66756e6e656c // "funnel"

var migrationName = regexp.MustCompile(`^(\d+)_[^/]+\.sql$`)

type migration struct {
	version int
	name    string
	sql     string
}

// loadMigrations reads the *.sql files in the "migrations" directory of fsys,
// sorted by their version number.
func loadMigrations(fsys fs.FS) ([]migration, error) {
	entries, err := fs.ReadDir(fsys, "migrations")
	if err != nil {
		return nil, fmt.Errorf("reading migrations: %w", err)
	}

	var migrations []migration
	seen := map[int]string{}
	for _, e := range entries {
		if e.IsDir() || path.Ext(e.Name()) != ".sql" {
			continue
		}
		m := migrationName.FindStringSubmatch(e.Name())
		if m == nil {
			return nil, fmt.Errorf("migration %s: name must be <number>_<name>.sql", e.Name())
		}
		version, err := strconv.Atoi(m[1])
		if err != nil || version < 1 {
			return nil, fmt.Errorf("migration %s: version must be a positive number", e.Name())
		}
		if other, ok := seen[version]; ok {
			return nil, fmt.Errorf("migrations %s and %s have the same version %d", other, e.Name(), version)
		}
		seen[version] = e.Name()

		data, err := fs.ReadFile(fsys, path.Join("migrations", e.Name()))
		if err != nil {
			return nil, fmt.Errorf("reading migration %s: %w", e.Name(), err)
		}
		if strings.TrimSpace(string(data)) == "" {
			return nil, fmt.Errorf("migration %s is empty", e.Name())
		}
		migrations = append(migrations, migration{version: version, name: e.Name(), sql: string(data)})
	}

	sort.Slice(migrations, func(i, j int) bool { return migrations[i].version < migrations[j].version })
	return migrations, nil
}

// migrate brings the database schema up to date: it applies the migrations
// of fsys that are newer than the version recorded in schema_version, in
// order, each in its own transaction. It fails if the database has a newer
// schema than the latest migration (a database used by a newer Funnel).
func migrate(ctx context.Context, pool *pgxpool.Pool, fsys fs.FS) error {
	migrations, err := loadMigrations(fsys)
	if err != nil {
		return err
	}

	conn, err := pool.Acquire(ctx)
	if err != nil {
		return fmt.Errorf("acquiring a connection for migrations: %w", err)
	}
	defer conn.Release()

	if _, err := conn.Exec(ctx, "SELECT pg_advisory_lock($1)", migrationLock); err != nil {
		return fmt.Errorf("locking the database for migrations: %w", err)
	}
	defer conn.Exec(context.Background(), "SELECT pg_advisory_unlock($1)", migrationLock)

	_, err = conn.Exec(ctx, `CREATE TABLE IF NOT EXISTS schema_version (
		version INTEGER PRIMARY KEY,
		name TEXT NOT NULL,
		applied_at TIMESTAMP WITH TIME ZONE DEFAULT NOW() NOT NULL
	)`)
	if err != nil {
		return fmt.Errorf("creating the schema_version table: %w", err)
	}

	var current int
	if err := conn.QueryRow(ctx, "SELECT COALESCE(MAX(version), 0) FROM schema_version").Scan(&current); err != nil {
		return fmt.Errorf("reading the schema version: %w", err)
	}
	latest := 0
	if len(migrations) > 0 {
		latest = migrations[len(migrations)-1].version
	}
	if current > latest {
		return fmt.Errorf("the database schema (version %d) is newer than this Funnel supports (version %d): "+
			"run a Funnel version that supports it, or restore a backup made before the upgrade", current, latest)
	}

	for _, m := range migrations {
		if m.version <= current {
			continue
		}
		tx, err := conn.Begin(ctx)
		if err != nil {
			return fmt.Errorf("migration %s: %w", m.name, err)
		}
		if _, err := tx.Exec(ctx, m.sql); err != nil {
			tx.Rollback(ctx)
			return fmt.Errorf("migration %s: %w", m.name, err)
		}
		if _, err := tx.Exec(ctx, "INSERT INTO schema_version (version, name) VALUES ($1, $2)", m.version, m.name); err != nil {
			tx.Rollback(ctx)
			return fmt.Errorf("migration %s: recording it in schema_version: %w", m.name, err)
		}
		if err := tx.Commit(ctx); err != nil {
			return fmt.Errorf("migration %s: %w", m.name, err)
		}
	}
	return nil
}
