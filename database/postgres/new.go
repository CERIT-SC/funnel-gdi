package postgres

import (
	"context"
	"fmt"
	"net/url"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/ohsu-comp-bio/funnel/compute/scheduler"
	"github.com/ohsu-comp-bio/funnel/config"
	"github.com/ohsu-comp-bio/funnel/util"
)

// Postgres provides a PostgreSQL database server backend.
type Postgres struct {
	scheduler.UnimplementedSchedulerServiceServer
	client *pgxpool.Pool
	conf   *config.Postgres
	active bool
}

func NewPostgres(conf *config.Postgres) (*Postgres, error) {
	ctx := context.Background()

	// Initialize the connection pool
	pool, err := pgxpool.New(ctx, getConnStr(conf))
	if err != nil {
		return nil, err
	}

	return &Postgres{
		conf:   conf,
		active: true,
		client: pool,
	}, nil
}

func (db *Postgres) Init() error {
	ctx := context.Background()

	retrier := util.NewRetrier()
	retrier.MaxElapsedTime = time.Second * 300

	return retrier.Retry(ctx, func() error {
		// Check/create resources (Roles/DBs)
		if err := ensureDatabaseExists(ctx, db.conf); err != nil {
			return err
		}

		// Create or update the tables and indices (migrations/*.sql)
		return migrate(ctx, db.client, migrationFiles)
	})
}

func (db *Postgres) context() (context.Context, context.CancelFunc) {
	// Use a timeout from the configuration
	return context.WithTimeout(context.Background(), db.conf.GetTimeout().GetDuration().AsDuration())
}

func buildConnStr(host, user, password, database string) string {
	connURL := url.URL{
		Scheme: "postgres",
		User:   url.UserPassword(user, password),
		Host:   host,
		Path:   database,
	}
	return connURL.String()
}

func getConnStr(conf *config.Postgres) string {
	return buildConnStr(conf.Host, conf.User, conf.Password, conf.Database)
}

func ensureDatabaseExists(ctx context.Context, conf *config.Postgres) error {
	// First check that we even need to connect as an "admin" in order to create the Funnel Role + DB
	connStr := getConnStr(conf)
	probeConn, err := pgx.Connect(ctx, connStr)
	if err == nil {
		defer probeConn.Close(ctx)
		var hasFullAccess bool
		checkSQL := `
			SELECT EXISTS (
				SELECT 1 FROM pg_database d
				WHERE d.datname = $1 
				AND has_database_privilege($2, $1, 'CONNECT')
				AND has_schema_privilege($2, 'public', 'CREATE')
			)`

		err = probeConn.QueryRow(ctx, checkSQL, conf.Database, conf.User).Scan(&hasFullAccess)

		// Role + DB already exist, no need to try connecting as admin
		if err == nil && hasFullAccess {
			return nil
		}
	}

	return createResources(ctx, conf)
}

func createResources(ctx context.Context, conf *config.Postgres) error {
	connStr := buildConnStr(conf.Host, conf.AdminUser, conf.AdminPassword, "postgres")
	conn, err := pgx.Connect(ctx, connStr)
	if err != nil {
		return fmt.Errorf("failed to connect as admin for bootstrap: %w", err)
	}
	defer conn.Close(ctx)

	// Role
	createUser := fmt.Sprintf(
		"DO $$ BEGIN IF NOT EXISTS (SELECT FROM pg_catalog.pg_roles WHERE rolname = '%s') "+
			"THEN CREATE ROLE %s WITH LOGIN PASSWORD '%s'; END IF; END $$",
		conf.User, conf.User, conf.Password,
	)
	if _, err := conn.Exec(ctx, createUser); err != nil {
		return fmt.Errorf("failed to ensure role exists: %w", err)
	}

	// Database
	createDB := fmt.Sprintf("CREATE DATABASE %s OWNER %s", conf.Database, conf.User)
	var dbExists int
	conn.QueryRow(ctx, "SELECT 1 FROM pg_database WHERE datname=$1", conf.Database).Scan(&dbExists)
	if dbExists != 1 {
		if _, err := conn.Exec(ctx, createDB); err != nil {
			return fmt.Errorf("failed to create app database: %w", err)
		}
	}

	// Now reconnect to the funnel database to grant schema permissions
	funnelConnStr := buildConnStr(conf.Host, conf.AdminUser, conf.AdminPassword, conf.Database)
	funnelConn, err := pgx.Connect(ctx, funnelConnStr)
	if err != nil {
		return fmt.Errorf("failed to connect to funnel database: %w", err)
	}
	defer funnelConn.Close(ctx)

	grantSchema := fmt.Sprintf("GRANT ALL ON SCHEMA public TO %s", conf.User)
	if _, err := funnelConn.Exec(ctx, grantSchema); err != nil {
		return fmt.Errorf("failed to grant schema privileges: %w", err)
	}

	return nil
}

// Close closes the database session.
func (db *Postgres) Close() {
	if db.active {
		db.client.Close()
		db.active = false
	}
}
