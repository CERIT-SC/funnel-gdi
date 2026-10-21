package postgres

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/postgres"
)

// startMigrationPostgres starts one PostgreSQL container for the migration
// tests and returns a function that creates a new, empty database in it.
func startMigrationPostgres(t *testing.T) func(t *testing.T) *pgxpool.Pool {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	container, err := postgres.Run(ctx,
		"postgres:16-alpine",
		postgres.WithDatabase("funnel"),
		postgres.WithUsername("funnel"),
		postgres.WithPassword("example"),
		postgres.BasicWaitStrategies(),
	)
	if err != nil {
		t.Fatalf("Failed to start PostgreSQL container: %v", err)
	}
	t.Cleanup(func() { testcontainers.TerminateContainer(container) })

	host, err := container.Host(ctx)
	if err != nil {
		t.Fatal(err)
	}
	port, err := container.MappedPort(ctx, "5432")
	if err != nil {
		t.Fatal(err)
	}
	addr := fmt.Sprintf("%s:%s", host, port.Port())

	admin, err := pgxpool.New(ctx, buildConnStr(addr, "funnel", "example", "funnel"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(admin.Close)

	n := 0
	return func(t *testing.T) *pgxpool.Pool {
		t.Helper()
		n++
		name := fmt.Sprintf("migrate_%d", n)
		if _, err := admin.Exec(context.Background(), "CREATE DATABASE "+name); err != nil {
			t.Fatal(err)
		}
		pool, err := pgxpool.New(context.Background(), buildConnStr(addr, "funnel", "example", name))
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(pool.Close)
		return pool
	}
}

func queryInt(t *testing.T, pool *pgxpool.Pool, sql string) int {
	t.Helper()
	var n int
	if err := pool.QueryRow(context.Background(), sql).Scan(&n); err != nil {
		t.Fatalf("%s: %v", sql, err)
	}
	return n
}

func tableExists(t *testing.T, pool *pgxpool.Pool, table string) bool {
	t.Helper()
	return queryInt(t, pool, fmt.Sprintf("SELECT COUNT(*) FROM information_schema.tables WHERE table_name = '%s'", table)) == 1
}

func TestMigrations(t *testing.T) {
	newDB := startMigrationPostgres(t)
	ctx := context.Background()

	embedded, err := loadMigrations(migrationFiles)
	if err != nil {
		t.Fatal(err)
	}
	latest := embedded[len(embedded)-1].version

	t.Run("fresh database", func(t *testing.T) {
		pool := newDB(t)
		if err := migrate(ctx, pool, migrationFiles); err != nil {
			t.Fatal(err)
		}
		if v := queryInt(t, pool, "SELECT MAX(version) FROM schema_version"); v != latest {
			t.Errorf("schema version %d, want %d", v, latest)
		}
		for _, table := range []string{"tasks", "nodes"} {
			if !tableExists(t, pool, table) {
				t.Errorf("table %s missing", table)
			}
		}
	})

	t.Run("restart applies nothing again", func(t *testing.T) {
		pool := newDB(t)
		for i := 0; i < 2; i++ {
			if err := migrate(ctx, pool, migrationFiles); err != nil {
				t.Fatalf("run %d: %v", i+1, err)
			}
		}
		if n := queryInt(t, pool, "SELECT COUNT(*) FROM schema_version"); n != len(embedded) {
			t.Errorf("%d rows in schema_version, want %d", n, len(embedded))
		}
	})

	t.Run("database created before migrations", func(t *testing.T) {
		pool := newDB(t)
		// The schema as created by Funnel versions without migrations.
		for _, sql := range []string{
			`CREATE TABLE IF NOT EXISTS tasks (id VARCHAR(255) PRIMARY KEY, state VARCHAR(50) NOT NULL, owner VARCHAR(255),
				creation_time TIMESTAMP WITH TIME ZONE DEFAULT NOW(), version BIGINT DEFAULT 0 NOT NULL, data JSONB)`,
			`CREATE TABLE IF NOT EXISTS nodes (id VARCHAR(255) PRIMARY KEY, state VARCHAR(50) NOT NULL, owner VARCHAR(255),
				version BIGINT DEFAULT 0 NOT NULL, last_heartbeat TIMESTAMP WITH TIME ZONE, data JSONB)`,
			"CREATE INDEX IF NOT EXISTS idx_tasks_state ON tasks (state)",
			`INSERT INTO tasks (id, state, owner, data) VALUES ('old-task', 'COMPLETE', 'alice', '{"id": "old-task"}')`,
		} {
			if _, err := pool.Exec(ctx, sql); err != nil {
				t.Fatal(err)
			}
		}

		if err := migrate(ctx, pool, migrationFiles); err != nil {
			t.Fatal(err)
		}
		if n := queryInt(t, pool, "SELECT COUNT(*) FROM tasks WHERE id = 'old-task' AND owner = 'alice' AND data->>'id' = 'old-task'"); n != 1 {
			t.Error("existing task lost or changed")
		}
		if v := queryInt(t, pool, "SELECT MAX(version) FROM schema_version"); v != latest {
			t.Errorf("schema version %d, want %d", v, latest)
		}
	})

	t.Run("failing migration is rolled back", func(t *testing.T) {
		pool := newDB(t)
		fsys := fstest.MapFS{
			"migrations/0001_a.sql":      {Data: []byte("CREATE TABLE a (x INT);")},
			"migrations/0002_broken.sql": {Data: []byte("CREATE TABLE b (x INT); SELECT no_such_function();")},
		}
		err := migrate(ctx, pool, fsys)
		if err == nil || !strings.Contains(err.Error(), "0002_broken.sql") {
			t.Fatalf("error %v should name the failing file", err)
		}
		if v := queryInt(t, pool, "SELECT MAX(version) FROM schema_version"); v != 1 {
			t.Errorf("schema version %d, want 1", v)
		}
		if !tableExists(t, pool, "a") || tableExists(t, pool, "b") {
			t.Error("0001 should be applied and 0002 rolled back completely")
		}
	})

	t.Run("numeric order", func(t *testing.T) {
		pool := newDB(t)
		fsys := fstest.MapFS{
			"migrations/0010_c.sql": {Data: []byte("INSERT INTO seq VALUES (10);")},
			"migrations/0002_b.sql": {Data: []byte("INSERT INTO seq VALUES (2);")},
			"migrations/0001_a.sql": {Data: []byte("CREATE TABLE seq (n INT, at SERIAL); INSERT INTO seq VALUES (1);")},
			"migrations/README.md":  {Data: []byte("not a migration")},
		}
		if err := migrate(ctx, pool, fsys); err != nil {
			t.Fatal(err)
		}
		var order []int
		rows, err := pool.Query(ctx, "SELECT n FROM seq ORDER BY at")
		if err != nil {
			t.Fatal(err)
		}
		for rows.Next() {
			var n int
			if err := rows.Scan(&n); err != nil {
				t.Fatal(err)
			}
			order = append(order, n)
		}
		if fmt.Sprint(order) != "[1 2 10]" {
			t.Errorf("applied in order %v, want [1 2 10]", order)
		}
	})

	t.Run("database newer than Funnel", func(t *testing.T) {
		pool := newDB(t)
		if err := migrate(ctx, pool, migrationFiles); err != nil {
			t.Fatal(err)
		}
		if _, err := pool.Exec(ctx, "INSERT INTO schema_version (version, name) VALUES (9999, '9999_future.sql')"); err != nil {
			t.Fatal(err)
		}
		err := migrate(ctx, pool, migrationFiles)
		if err == nil || !strings.Contains(err.Error(), "newer") {
			t.Fatalf("got %v, want an error about a newer schema", err)
		}
	})
}

func TestLoadMigrationsRejectsBadNames(t *testing.T) {
	for name, fsys := range map[string]fstest.MapFS{
		"no number": {"migrations/initial.sql": {Data: []byte("SELECT 1;")}},
		"duplicate version": {
			"migrations/0001_a.sql": {Data: []byte("SELECT 1;")},
			"migrations/1_b.sql":    {Data: []byte("SELECT 1;")},
		},
		"version zero": {"migrations/0000_a.sql": {Data: []byte("SELECT 1;")}},
		"empty file":   {"migrations/0001_a.sql": {Data: []byte("  \n")}},
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := loadMigrations(fsys); err == nil {
				t.Fatal("expected an error")
			}
		})
	}
}
