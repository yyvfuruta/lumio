package app

import (
	"context"
	"crypto/sha256"
	_ "embed"
	"errors"
	"log/slog"
	"sync"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"gocloud.dev/blob"
)

//go:embed schema.sql
var schema string

type App struct {
	cfg         Config
	db          *pgxpool.Pool
	bucket      *blob.Bucket
	passwordTag [32]byte
	uploadGate  chan struct{}
	cancel      context.CancelFunc
	workers     sync.WaitGroup
}

func New(cfg Config) (*App, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	db, err := pgxpool.New(ctx, cfg.DatabaseURL)
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
	tx, err := db.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(context.Background())
	if _, err := tx.Exec(ctx, "SELECT pg_advisory_xact_lock(739421016)"); err != nil {
		return nil, err
	}
	if _, err := tx.Exec(ctx, schema); err != nil {
		return nil, err
	}
	if _, err := tx.Exec(ctx, "INSERT INTO lumio_settings(id,storage_url) VALUES(true,$1) ON CONFLICT DO NOTHING", cfg.StorageURL); err != nil {
		return nil, err
	}
	var stored string
	if err := tx.QueryRow(ctx, "SELECT storage_url FROM lumio_settings WHERE id=true").Scan(&stored); err != nil {
		return nil, err
	}
	if stored != cfg.StorageURL {
		return nil, errors.New("storage does not match PostgreSQL settings; run migrate-storage before switching backends")
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	bucket, err := openBucket(ctx, cfg.StorageURL)
	if err != nil {
		return nil, err
	}
	a := &App{cfg: cfg, db: db, bucket: bucket, passwordTag: sha256.Sum256([]byte(cfg.Password)), uploadGate: make(chan struct{}, 1)}
	background, cancelWorker := context.WithCancel(context.Background())
	a.cancel = cancelWorker
	a.workers.Add(1)
	go a.maintenance(background)
	ok = true
	return a, nil
}

func (a *App) Close() { a.cancel(); a.workers.Wait(); a.bucket.Close(); a.db.Close() }

func (a *App) maintenance(ctx context.Context) {
	defer a.workers.Done()
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()
	for {
		for range 50 {
			job, cancel := context.WithTimeout(ctx, 20*time.Second)
			more, err := a.collectOne(job)
			cancel()
			if err != nil {
				if ctx.Err() == nil {
					slog.Error("photo cleanup pending", "error", err)
				}
				break
			}
			if !more {
				break
			}
		}
		cleanup, cancel := context.WithTimeout(ctx, 5*time.Second)
		_, err := a.db.Exec(cleanup, "DELETE FROM lumio_sessions WHERE expires_at<now()")
		if err == nil {
			_, err = a.db.Exec(cleanup, "DELETE FROM lumio_login_limits WHERE minute<now()-interval '5 minutes'")
		}
		if err == nil {
			_, err = a.db.Exec(cleanup, "DELETE FROM lumio_photo_archives WHERE expires_at<now()")
		}
		cancel()
		if err != nil && ctx.Err() == nil {
			slog.Error("session cleanup failed", "error", err)
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}
