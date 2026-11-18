package postgres

import (
	"context"
	_ "embed"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
)

//go:embed schema.sql
var Schema string

// Open initializes the idempotent schema under the shared administrative lock.
func Open(ctx context.Context, databaseURL, storageURL string) (*pgxpool.Pool, error) {
	return Connect(ctx, databaseURL, storageURL, true)
}

// Connect can verify an existing schema without running DDL in web/worker processes.
func Connect(ctx context.Context, databaseURL, storageURL string, initialize bool) (*pgxpool.Pool, error) {
	db, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		return nil, err
	}
	ok := false
	defer func() {
		if !ok {
			db.Close()
		}
	}()
	if err := db.Ping(ctx); err != nil {
		return nil, err
	}
	if !initialize {
		if err := New(db, storageURL).Ready(ctx); err != nil {
			return nil, fmt.Errorf("database schema unavailable or storage mismatched; run lumio migrate and verify configuration: %w", err)
		}
		ok = true
		return db, nil
	}
	tx, err := db.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(context.Background())
	if _, err := tx.Exec(ctx, "SELECT pg_advisory_xact_lock(739421016)"); err != nil {
		return nil, err
	}
	if _, err := tx.Exec(ctx, Schema); err != nil {
		return nil, err
	}
	if _, err := tx.Exec(ctx, "INSERT INTO lumio_settings(id,storage_url) VALUES(true,$1) ON CONFLICT DO NOTHING", storageURL); err != nil {
		return nil, err
	}
	var stored string
	if err := tx.QueryRow(ctx, "SELECT storage_url FROM lumio_settings WHERE id=true").Scan(&stored); err != nil {
		return nil, err
	}
	if stored != storageURL {
		return nil, errors.New("storage does not match PostgreSQL settings; run migrate-storage before switching backends")
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	ok = true
	return db, nil
}
