package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/yyvfuruta/lumio/internal/adapters/postgres"
	"github.com/yyvfuruta/lumio/internal/app"
)

var version = "dev"
var commit = "none"
var date = "unknown"

func main() {
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stderr, nil)))
	if err := run(); err != nil {
		slog.Error("lumio stopped", "error", err)
		os.Exit(1)
	}
}

func run() error {
	if len(os.Args) > 1 {
		switch os.Args[1] {
		case "version":
			if len(os.Args) != 2 {
				return errors.New("version accepts no arguments")
			}
			fmt.Printf("lumio %s (%s, %s)\n", version, commit, date)
			return nil
		case "migrate":
			if len(os.Args) != 2 {
				return errors.New("migrate accepts no arguments")
			}
			cfg, err := app.ConfigFromEnv()
			if err != nil {
				return err
			}
			ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
			defer stop()
			ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
			defer cancel()
			db, err := postgres.Open(ctx, cfg.DatabaseURL, cfg.StorageURL)
			if err != nil {
				return err
			}
			db.Close()
			return nil
		case "worker":
			slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, nil)))
			if len(os.Args) != 2 {
				return errors.New("worker uses environment configuration and accepts no arguments")
			}
			cfg, err := app.ConfigFromEnv()
			if err != nil {
				return err
			}
			cfg.Maintenance = true
			ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
			defer stop()
			a, err := app.New(cfg)
			if err != nil {
				return err
			}
			defer a.Close()
			slog.Info("lumio worker started")
			<-ctx.Done()
			return nil
		case "migrate-storage":
			if len(os.Args) != 2 {
				return errors.New("migrate-storage accepts no arguments")
			}
			ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
			defer stop()
			return app.MigrateStorage(ctx, os.Getenv("DATABASE_URL"), os.Getenv("LUMIO_MIGRATE_TO"))
		case "serve":
			if len(os.Args) != 2 {
				return errors.New("serve uses environment configuration and accepts no arguments")
			}
		case "help", "--help", "-h":
			fmt.Println("Usage: lumio [serve|worker|migrate|migrate-storage|version]\nRemote client: lumio-cli help")
			return nil
		default:
			return fmt.Errorf("unknown server command %q; use lumio help or lumio-cli help", os.Args[1])
		}
	}
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, nil)))
	cfg, err := app.ConfigFromEnv()
	if err != nil {
		return err
	}
	a, err := app.New(cfg)
	if err != nil {
		return err
	}
	defer a.Close()
	srv := &http.Server{Addr: cfg.Addr, Handler: a.Handler(), ErrorLog: slog.NewLogLogger(slog.Default().Handler(), slog.LevelError), ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 2 * time.Minute, WriteTimeout: 2 * time.Minute, IdleTimeout: 60 * time.Second, MaxHeaderBytes: 1 << 20}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	done := make(chan error, 1)
	go func() { slog.Info("lumio started", "addr", cfg.Addr); done <- srv.ListenAndServe() }()
	select {
	case err := <-done:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return err
	case <-ctx.Done():
		shutdown, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		return srv.Shutdown(shutdown)
	}
}
