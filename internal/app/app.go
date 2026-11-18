package app

import (
	"context"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/yyvfuruta/lumio/internal/adapters/httpapi"
	"github.com/yyvfuruta/lumio/internal/adapters/media"
	"github.com/yyvfuruta/lumio/internal/adapters/objectstore"
	"github.com/yyvfuruta/lumio/internal/adapters/postgres"
	"github.com/yyvfuruta/lumio/internal/core"
	"gocloud.dev/blob"
)

type App struct {
	cfg      Config
	db       *pgxpool.Pool
	bucket   *blob.Bucket
	repo     *postgres.Repository
	library  *core.Library
	accounts *core.Accounts
	handler  http.Handler
	cancel   context.CancelFunc
	workers  sync.WaitGroup
}

func New(cfg Config) (*App, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	db, err := postgres.Connect(ctx, cfg.DatabaseURL, cfg.StorageURL, !cfg.SkipMigrations)
	if err != nil {
		return nil, err
	}
	ok := false
	defer func() {
		if !ok {
			db.Close()
		}
	}()
	bucket, err := openBucket(ctx, cfg.StorageURL)
	if err != nil {
		return nil, err
	}
	a := &App{cfg: cfg, db: db, bucket: bucket}
	a.repo = postgres.New(db, cfg.StorageURL)
	a.library = core.NewLibrary(a.repo, objectstore.New(bucket), media.Processor{})
	a.accounts = core.NewAccounts(a.repo, cfg.Password)
	a.handler = httpapi.New(httpapi.Config{StorageURL: cfg.StorageURL, SecureCookie: cfg.SecureCookie, MaxUploadBytes: cfg.MaxUploadBytes}, a.library, a.accounts, a.repo.Ready)
	background, cancelWorker := context.WithCancel(context.Background())
	a.cancel = cancelWorker
	if cfg.Maintenance {
		a.workers.Add(1)
		go a.maintenance(background)
	}
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
		err := a.repo.Prune(cleanup)
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

func (a *App) Handler() http.Handler { return a.handler }
