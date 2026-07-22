// Package dbconn opens the Postgres connection used by the domain packages
// (clients, ingests, streaming platforms, live ids) and creates their schema.
//
// Schema is plain, idempotent SQL rather than GORM's AutoMigrate: AutoMigrate
// only creates FK/check constraints when the Go structs hold a direct
// association to each other, which would force internal/ingest and
// internal/liveid to import internal/client and internal/streamplatform —
// exactly the cross-package coupling the consumer-declared-interface pattern
// used throughout this codebase avoids. Raw SQL gives full control over the
// constraints that carry the domain's actual invariants (composite
// uniqueness, FK ON DELETE behavior) without that coupling.
package dbconn

import (
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// schema is idempotent: every statement is safe to re-run on every process
// start, so no separate migration-tracking table is needed at this scale.
const schema = `
CREATE TABLE IF NOT EXISTS clients (
	id         uuid PRIMARY KEY DEFAULT gen_random_uuid(),
	name       text NOT NULL CHECK (length(btrim(name)) > 0),
	email      text,
	created_at timestamptz NOT NULL DEFAULT now(),
	updated_at timestamptz NOT NULL DEFAULT now(),
	deleted_at timestamptz
);
CREATE INDEX IF NOT EXISTS clients_created_idx
	ON clients (created_at DESC, id DESC) WHERE deleted_at IS NULL;

CREATE TABLE IF NOT EXISTS streaming_platforms (
	id           uuid PRIMARY KEY DEFAULT gen_random_uuid(),
	slug         text NOT NULL CHECK (length(btrim(slug)) > 0),
	display_name text NOT NULL CHECK (length(btrim(display_name)) > 0),
	created_at   timestamptz NOT NULL DEFAULT now(),
	updated_at   timestamptz NOT NULL DEFAULT now()
);
CREATE UNIQUE INDEX IF NOT EXISTS streaming_platforms_slug_idx ON streaming_platforms (slug);

CREATE TABLE IF NOT EXISTS ingests (
	id         uuid PRIMARY KEY DEFAULT gen_random_uuid(),
	client_id  uuid NOT NULL REFERENCES clients (id) ON DELETE CASCADE,
	url        text NOT NULL CHECK (length(btrim(url)) > 0),
	protocol   text NOT NULL CHECK (protocol IN ('http', 'https', 'ftp', 'sftp', 's3')),
	is_active  boolean NOT NULL DEFAULT true,
	created_at timestamptz NOT NULL DEFAULT now(),
	updated_at timestamptz NOT NULL DEFAULT now()
);
CREATE UNIQUE INDEX IF NOT EXISTS ingests_client_url_idx ON ingests (client_id, url);
CREATE INDEX IF NOT EXISTS ingests_created_idx ON ingests (created_at DESC, id DESC);

-- platform_id has no ON DELETE clause on purpose: deleting a streaming
-- platform that a live id still references must fail (409), not cascade.
CREATE TABLE IF NOT EXISTS client_live_ids (
	id          uuid PRIMARY KEY DEFAULT gen_random_uuid(),
	client_id   uuid NOT NULL REFERENCES clients (id) ON DELETE CASCADE,
	platform_id uuid NOT NULL REFERENCES streaming_platforms (id),
	live_id     text NOT NULL CHECK (length(btrim(live_id)) > 0),
	is_active   boolean NOT NULL DEFAULT true,
	created_at  timestamptz NOT NULL DEFAULT now(),
	updated_at  timestamptz NOT NULL DEFAULT now()
);
CREATE UNIQUE INDEX IF NOT EXISTS client_live_ids_tuple_idx
	ON client_live_ids (client_id, platform_id, live_id);
CREATE INDEX IF NOT EXISTS client_live_ids_created_idx ON client_live_ids (created_at DESC, id DESC);

-- Additive columns for RTMP push credentials (ADR-008). Kept as ALTER …
-- ADD COLUMN IF NOT EXISTS so existing deployments pick them up on restart
-- without a separate migration runner.
ALTER TABLE streaming_platforms
	ADD COLUMN IF NOT EXISTS ingest_url_template text NOT NULL DEFAULT '';
ALTER TABLE client_live_ids
	ADD COLUMN IF NOT EXISTS stream_key text NOT NULL DEFAULT '';
`

// Open connects to Postgres and creates the schema for the domain models.
func Open(dsn string) (*gorm.DB, error) {
	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		return nil, err
	}
	if err := Migrate(db); err != nil {
		return nil, err
	}
	return db, nil
}

// Migrate creates the schema. Exported so tests can migrate a
// testcontainers-provided *gorm.DB directly.
func Migrate(db *gorm.DB) error {
	return db.Exec(schema).Error
}
