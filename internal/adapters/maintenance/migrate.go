package maintenance

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"log/slog"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/yyvfuruta/lumio/internal/adapters/objectstore"
	"github.com/yyvfuruta/lumio/internal/adapters/postgres"
	"gocloud.dev/blob"
)

// MigrateStorage keeps source objects intact and only changes the registered
// backend after every copy has been verified. Retry safely after interruption.
func MigrateStorage(ctx context.Context, databaseURL, targetURL string) error {
	if databaseURL == "" {
		return errors.New("DATABASE_URL is required")
	}
	if err := objectstore.ValidateURL(targetURL); err != nil {
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
	if _, err := tx.Exec(ctx, "SELECT pg_advisory_xact_lock(739421016)"); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, postgres.Schema); err != nil {
		return err
	}
	var sourceURL string
	if err := tx.QueryRow(ctx, "SELECT storage_url FROM lumio_settings WHERE id=true FOR UPDATE").Scan(&sourceURL); err != nil {
		return err
	}
	if sourceURL == targetURL {
		return errors.New("source and destination are identical")
	}
	source, err := objectstore.Open(ctx, sourceURL)
	if err != nil {
		return err
	}
	defer source.Close()
	target, err := objectstore.Open(ctx, targetURL)
	if err != nil {
		return err
	}
	defer target.Close()
	rows, err := tx.Query(ctx, "SELECT "+postgres.Columns+" FROM lumio_photos ORDER BY id")
	if err != nil {
		return err
	}
	defer rows.Close()
	count := 0
	for rows.Next() {
		p, err := postgres.ScanPhoto(rows)
		if err != nil {
			return err
		}
		for _, obj := range []struct {
			key, typ, expectedHash string
			size                   int64
		}{{p.ID + "/original", p.Type, p.ID, p.Size}, {p.ID + "/thumb.jpg", "image/jpeg", "", p.ThumbSize}} {
			if err := CopyVerified(ctx, source, target, obj.key, obj.typ, obj.size, obj.expectedHash); err != nil {
				return fmt.Errorf("object %s: %w", obj.key, err)
			}
		}
		if p.PreviewSize > 0 {
			if err := CopyVerified(ctx, source, target, p.ID+"/preview.jpg", "image/jpeg", p.PreviewSize, ""); err != nil {
				return fmt.Errorf("preview %s: %w", p.ID, err)
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

func CopyVerified(ctx context.Context, source, target *blob.Bucket, key, typ string, size int64, expectedHash string) error {
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
