package app

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"log/slog"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"gocloud.dev/blob"
)

// storageGuard takes a shared row lock. Migration takes an exclusive row lock,
// so writes/collection cannot race a backend switch, including on other replicas.
func (a *App) storageGuard(ctx context.Context, tx pgx.Tx) error {
	var current string
	if err := tx.QueryRow(ctx, "SELECT storage_url FROM lumio_settings WHERE id=true FOR SHARE").Scan(&current); err != nil {
		return err
	}
	if current != a.cfg.StorageURL {
		return errors.New("storage has been migrated; update LUMIO_STORAGE_URL and restart this replica")
	}
	return nil
}

// MigrateStorage keeps source objects intact and only changes the registered
// backend after every copy has been verified. Retry safely after interruption.
func MigrateStorage(ctx context.Context, databaseURL, targetURL string) error {
	if databaseURL == "" {
		return errors.New("DATABASE_URL is required")
	}
	if err := validateStorageURL(targetURL); err != nil {
		return err
	}
	db, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		return err
	}
	defer db.Close()
	tx, err := db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(context.Background())
	var sourceURL string
	if err := tx.QueryRow(ctx, "SELECT storage_url FROM lumio_settings WHERE id=true FOR UPDATE").Scan(&sourceURL); err != nil {
		return err
	}
	if sourceURL == targetURL {
		return errors.New("source and destination are identical")
	}
	source, err := openBucket(ctx, sourceURL)
	if err != nil {
		return err
	}
	defer source.Close()
	target, err := openBucket(ctx, targetURL)
	if err != nil {
		return err
	}
	defer target.Close()
	rows, err := tx.Query(ctx, "SELECT "+columns+" FROM lumio_photos ORDER BY id")
	if err != nil {
		return err
	}
	defer rows.Close()
	count := 0
	for rows.Next() {
		p, err := scanPhoto(rows)
		if err != nil {
			return err
		}
		for _, obj := range []struct {
			key, typ, expectedHash string
			size                   int64
		}{{p.ID + "/original", p.Type, p.ID, p.Size}, {p.ID + "/thumb.jpg", "image/jpeg", "", p.ThumbSize}} {
			if err := copyVerified(ctx, source, target, obj.key, obj.typ, obj.size, obj.expectedHash); err != nil {
				return fmt.Errorf("object %s: %w", obj.key, err)
			}
		}
		count++
		slog.Info("photo migrated and verified", "photo_id", p.ID, "photos", count)
	}
	if err := rows.Err(); err != nil {
		return err
	}
	rows.Close()
	if _, err := tx.Exec(ctx, "UPDATE lumio_settings SET storage_url=$1 WHERE id=true", targetURL); err != nil {
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return err
	}
	slog.Info("migration complete; update LUMIO_STORAGE_URL and restart all replicas", "photos", count)
	return nil
}

func digestObject(ctx context.Context, b *blob.Bucket, key string) (string, int64, error) {
	r, err := b.NewReader(ctx, key, nil)
	if err != nil {
		return "", 0, err
	}
	defer r.Close()
	h := sha256.New()
	n, err := io.Copy(h, r)
	return hex.EncodeToString(h.Sum(nil)), n, err
}

func copyVerified(ctx context.Context, source, target *blob.Bucket, key, typ string, size int64, expectedHash string) error {
	want, n, err := digestObject(ctx, source, key)
	if err != nil {
		return err
	}
	if n != size || (expectedHash != "" && want != expectedHash) {
		return errors.New("source failed size or SHA-256 verification")
	}
	exists, err := target.Exists(ctx, key)
	if err != nil {
		return err
	}
	if !exists {
		r, err := source.NewReader(ctx, key, nil)
		if err != nil {
			return err
		}
		err = target.Upload(ctx, key, r, &blob.WriterOptions{ContentType: typ})
		r.Close()
		if err != nil {
			return err
		}
	}
	got, n, err := digestObject(ctx, target, key)
	if err != nil {
		return err
	}
	if got != want || n != size {
		return errors.New("destination differs from source; it will not be overwritten automatically")
	}
	return nil
}
