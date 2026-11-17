-- Migration 1. Serialized with a PostgreSQL advisory lock at startup.
CREATE TABLE IF NOT EXISTS lumio_settings (
  id boolean PRIMARY KEY DEFAULT true CHECK (id),
  storage_url text NOT NULL
);
CREATE TABLE IF NOT EXISTS lumio_photos (
  id text PRIMARY KEY CHECK (id ~ '^[a-f0-9]{64}$'),
  name text NOT NULL,
  content_type text NOT NULL,
  size bigint NOT NULL,
  thumb_size bigint NOT NULL,
  width integer NOT NULL,
  height integer NOT NULL,
  added_at timestamptz NOT NULL DEFAULT now(),
  favorite boolean NOT NULL DEFAULT false,
  deleted_at timestamptz
);
CREATE INDEX IF NOT EXISTS lumio_photos_timeline ON lumio_photos (added_at DESC, id DESC) WHERE deleted_at IS NULL;
CREATE INDEX IF NOT EXISTS lumio_photos_favorites ON lumio_photos (added_at DESC, id DESC) WHERE deleted_at IS NULL AND favorite;
CREATE TABLE IF NOT EXISTS lumio_sessions (
  token_hash bytea PRIMARY KEY,
  password_tag bytea NOT NULL,
  expires_at timestamptz NOT NULL
);
CREATE INDEX IF NOT EXISTS lumio_sessions_expiry ON lumio_sessions (expires_at);
CREATE TABLE IF NOT EXISTS lumio_login_limits (
  minute timestamptz PRIMARY KEY,
  attempts integer NOT NULL
);
