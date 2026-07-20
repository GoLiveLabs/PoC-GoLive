// Package testdb spins up a throwaway Postgres container for tests that
// need real database behavior (unique/FK constraint violations, CHECK
// constraints) that a mock cannot exercise honestly. Every domain service
// test in this repo (client, ingest, streamplatform, liveid) uses it instead
// of a mocked repository.
package testdb

import (
	"context"
	"testing"
	"time"

	tcpostgres "github.com/testcontainers/testcontainers-go/modules/postgres"
	gormpostgres "gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"live-orchestrator/backend/internal/dbconn"
)

// New starts a Postgres 16 container, connects via GORM, creates the schema,
// and registers container/connection teardown via t.Cleanup. It calls
// t.Skip if Docker is not available in the current environment, so unit
// tests that don't need a database still run without it via `go test -short`.
func New(t *testing.T) *gorm.DB {
	t.Helper()
	if testing.Short() {
		t.Skip("skipping testcontainers-backed test in -short mode")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	container, err := tcpostgres.Run(ctx, "postgres:16-alpine",
		tcpostgres.WithDatabase("testdb"),
		tcpostgres.WithUsername("postgres"),
		tcpostgres.WithPassword("postgres"),
		tcpostgres.BasicWaitStrategies(),
	)
	if err != nil {
		t.Skipf("could not start postgres testcontainer (is Docker running?): %v", err)
	}
	t.Cleanup(func() {
		_ = container.Terminate(context.Background())
	})

	dsn, err := container.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		t.Fatalf("getting connection string: %v", err)
	}

	db, err := gorm.Open(gormpostgres.Open(dsn), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		t.Fatalf("connecting to test database: %v", err)
	}
	if err := dbconn.Migrate(db); err != nil {
		t.Fatalf("migrating test database: %v", err)
	}

	sqlDB, err := db.DB()
	if err == nil {
		t.Cleanup(func() { _ = sqlDB.Close() })
	}

	return db
}
