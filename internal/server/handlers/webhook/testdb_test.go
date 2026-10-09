package webhook_test

import (
	"context"
	"database/sql"
	"fmt"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/go-sql-driver/mysql"
	"github.com/pressly/goose/v3"
	"github.com/support-loop/backend/internal/db/migrations"
	"github.com/uptrace/bun"
	"github.com/uptrace/bun/dialect/mysqldialect"
)

const (
	testDBName    = "support_loop_test"
	defaultDBURL  = "mariadb://support-loop:support-loop@127.0.0.1:3307/support-loop?charset=utf8mb4&parseTime=True&loc=UTC"
	migrationLock = "support_loop_test_migrate"
)

// appMySQLDSN converts the configured URL into a go-sql-driver DSN the same
// way sqlfx does for the application.
func appMySQLDSN() (string, error) {
	raw := os.Getenv("DATABASE__URL")
	if raw == "" {
		raw = defaultDBURL
	}
	u, err := url.Parse(raw)
	if err != nil {
		return "", fmt.Errorf("parse url: %w", err)
	}
	if u.Scheme != "mysql" && u.Scheme != "mariadb" {
		return "", fmt.Errorf("unsupported scheme: %s", u.Scheme)
	}
	port := u.Port()
	if port == "" {
		port = "3306"
	}
	password, _ := u.User.Password()
	dsn := fmt.Sprintf("%s:%s@tcp(%s:%s)/%s",
		u.User.Username(),
		password,
		u.Hostname(),
		port,
		strings.TrimPrefix(u.Path, "/"),
	)
	params := u.Query()
	if params.Get("charset") == "" {
		params.Set("charset", "utf8mb4")
	}
	if params.Get("parseTime") == "" {
		params.Set("parseTime", "True")
	}
	if params.Get("loc") == "" {
		params.Set("loc", "Local")
	}
	return fmt.Sprintf("%s?%s", dsn, params.Encode()), nil
}

// setupTestDB connects to the local MariaDB, provisions the dedicated test
// database and applies migrations. It skips the test when the DB is
// unreachable or the test database cannot be created.
func setupTestDB(t *testing.T) *bun.DB {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	appDSN, dsnErr := appMySQLDSN()
	if dsnErr != nil {
		t.Fatalf("build DSN: %v", dsnErr)
	}
	cfg, parseErr := mysql.ParseDSN(appDSN)
	if parseErr != nil {
		t.Fatalf("parse DSN: %v", parseErr)
	}
	addr := cfg.Addr
	cfg.DBName = testDBName
	testDSN := cfg.FormatDSN()

	sqlDB, openErr := sql.Open("mysql", testDSN)
	if openErr != nil {
		t.Fatalf("open test DB: %v", openErr)
	}
	if pingErr := sqlDB.PingContext(ctx); pingErr != nil {
		_ = sqlDB.Close()
		t.Skipf("local MariaDB unreachable at %s: %v", addr, pingErr)
	}

	adminDB, adminErr := sql.Open("mysql", appDSN)
	if adminErr != nil {
		t.Fatalf("open app DB: %v", adminErr)
	}
	if _, createErr := adminDB.ExecContext(ctx, "CREATE DATABASE IF NOT EXISTS "+testDBName); createErr != nil {
		_ = adminDB.Close()
		_ = sqlDB.Close()
		t.Skipf("cannot create test database %s: %v", testDBName, createErr)
	}
	_ = adminDB.Close()

	if migrateErr := migrateUp(ctx, sqlDB); migrateErr != nil {
		_ = sqlDB.Close()
		t.Fatalf("apply migrations: %v", migrateErr)
	}

	db := bun.NewDB(sqlDB, mysqldialect.New(mysqldialect.WithTimeLocation("UTC")))
	release := acquireSerialLock(t, db)
	t.Cleanup(func() {
		release()
		_ = sqlDB.Close()
	})

	return db
}

const serialLockName = "support_loop_test_serial"

// acquireSerialLock serializes DB integration tests within and across test
// packages: real workers otherwise claim jobs enqueued by parallel tests.
// The advisory lock is connection-scoped, so it lives on a dedicated conn.
func acquireSerialLock(t *testing.T, db *bun.DB) func() {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	conn, connErr := db.Conn(ctx)
	if connErr != nil {
		t.Fatalf("acquire serial lock connection: %v", connErr)
	}
	var locked int
	if lockErr := conn.QueryRowContext(ctx, "SELECT GET_LOCK(?, 60)", serialLockName).Scan(&locked); lockErr != nil {
		_ = conn.Close()
		t.Fatalf("acquire serial lock: %v", lockErr)
	}
	if locked != 1 {
		_ = conn.Close()
		t.Fatalf("failed to acquire serial lock")
	}
	return func() {
		_, _ = conn.ExecContext(context.Background(), "SELECT RELEASE_LOCK(?)", serialLockName)
		_ = conn.Close()
	}
}

// migrateUp applies goose migrations under a MySQL advisory lock so parallel
// test packages can migrate the same test database safely. The lock is held
// on a dedicated connection because GET_LOCK is connection-scoped.
func migrateUp(ctx context.Context, sqlDB *sql.DB) error {
	conn, connErr := sqlDB.Conn(ctx)
	if connErr != nil {
		return fmt.Errorf("acquire migration connection: %w", connErr)
	}
	defer func() { _ = conn.Close() }()

	var locked int
	if lockErr := conn.QueryRowContext(ctx, "SELECT GET_LOCK(?, 30)", migrationLock).Scan(&locked); lockErr != nil {
		return fmt.Errorf("acquire migration lock: %w", lockErr)
	}
	if locked != 1 {
		return fmt.Errorf("failed to acquire migration lock")
	}
	defer func() { _, _ = conn.ExecContext(context.Background(), "SELECT RELEASE_LOCK(?)", migrationLock) }()

	provider, providerErr := goose.NewProvider(goose.DialectMySQL, sqlDB, migrations.FS)
	if providerErr != nil {
		return fmt.Errorf("init goose provider: %w", providerErr)
	}
	if _, upErr := provider.Up(ctx); upErr != nil {
		return fmt.Errorf("goose up: %w", upErr)
	}
	return nil
}

func countMarkers(t *testing.T, db *bun.DB, caseID int64) int {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	count, err := db.NewSelect().Table("processed_events").Where("case_id = ?", caseID).Count(ctx)
	if err != nil {
		t.Fatalf("count markers failed: %v", err)
	}
	return count
}
