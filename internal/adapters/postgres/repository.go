// Package postgres implements the core persistence ports, including locking
// required to coordinate imports, deletion and storage migration across replicas.
package postgres

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/yyvfuruta/lumio/internal/core"
)

const Columns = "id,name,content_type,size,thumb_size,preview_size,width,height,added_at,favorite,duration"

func ScanPhoto(row pgx.Row) (p core.Photo, err error) {
	err = row.Scan(&p.ID, &p.Name, &p.Type, &p.Size, &p.ThumbSize, &p.PreviewSize, &p.Width, &p.Height, &p.AddedAt, &p.Favorite, &p.Duration)
	return
}
func domainError(err error) error {
	if errors.Is(err, pgx.ErrNoRows) {
		return core.ErrNotFound
	}
	return err
}

type Repository struct {
	db         *pgxpool.Pool
	storageURL string
}

func New(db *pgxpool.Pool, storageURL string) *Repository {
	return &Repository{db: db, storageURL: storageURL}
}

var _ core.Repository = (*Repository)(nil)
var _ core.Sessions = (*Repository)(nil)

func (r *Repository) List(ctx context.Context, f core.Filter) ([]core.Photo, error) {
	rows, err := r.db.Query(ctx, "SELECT "+Columns+` FROM lumio_photos WHERE deleted_at IS NULL AND (NOT $1::boolean OR favorite)
 AND strpos(lower(name),lower($2))>0 AND (added_at,id)<($3,$4)
 AND ($5::timestamptz IS NULL OR added_at >= $5) AND ($6::timestamptz IS NULL OR added_at < $6)
 ORDER BY added_at DESC,id DESC LIMIT 61`, f.Favorite, f.Query, f.Before, f.BeforeID, f.From, f.Until)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	photos := make([]core.Photo, 0, 61)
	for rows.Next() {
		p, err := ScanPhoto(rows)
		if err != nil {
			return nil, err
		}
		photos = append(photos, p)
	}
	return photos, rows.Err()
}
func (r *Repository) Get(ctx context.Context, id string) (core.Photo, error) {
	p, err := ScanPhoto(r.db.QueryRow(ctx, "SELECT "+Columns+" FROM lumio_photos WHERE id=$1 AND deleted_at IS NULL", id))
	return p, domainError(err)
}
func (r *Repository) SetFavorite(ctx context.Context, id string, favorite bool) (core.Photo, error) {
	p, err := ScanPhoto(r.db.QueryRow(ctx, "UPDATE lumio_photos SET favorite=$2 WHERE id=$1 AND deleted_at IS NULL RETURNING "+Columns, id, favorite))
	return p, domainError(err)
}
func Guard(ctx context.Context, tx pgx.Tx, storageURL string) error {
	var current string
	if err := tx.QueryRow(ctx, "SELECT storage_url FROM lumio_settings WHERE id=true FOR SHARE").Scan(&current); err != nil {
		return err
	}
	if current != storageURL {
		return errors.New("storage has been migrated; update LUMIO_STORAGE_URL and restart this replica")
	}
	return nil
}
func Lock(ctx context.Context, tx pgx.Tx, id string) error {
	hash := sha256.Sum256([]byte(id))
	_, err := tx.Exec(ctx, "SELECT pg_advisory_xact_lock($1)", int64(binary.BigEndian.Uint64(hash[:])))
	return err
}
func (r *Repository) beginPhoto(ctx context.Context, id string) (pgx.Tx, error) {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return nil, err
	}
	if err = Guard(ctx, tx, r.storageURL); err == nil {
		err = Lock(ctx, tx, id)
	}
	if err != nil {
		tx.Rollback(context.Background())
		return nil, err
	}
	return tx, nil
}
func (r *Repository) Delete(ctx context.Context, id string) error {
	tx, err := r.beginPhoto(ctx, id)
	if err != nil {
		return err
	}
	defer tx.Rollback(context.Background())
	tag, err := tx.Exec(ctx, "UPDATE lumio_photos SET deleted_at=now() WHERE id=$1 AND deleted_at IS NULL", id)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return core.ErrNotFound
	}
	return tx.Commit(ctx)
}
func (r *Repository) Save(ctx context.Context, p core.Photo, write func(context.Context) error) (core.Photo, bool, error) {
	tx, err := r.beginPhoto(ctx, p.ID)
	if err != nil {
		return core.Photo{}, false, err
	}
	defer tx.Rollback(context.Background())
	existing, err := ScanPhoto(tx.QueryRow(ctx, "SELECT "+Columns+" FROM lumio_photos WHERE id=$1 AND deleted_at IS NULL", p.ID))
	if err == nil {
		return existing, true, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return core.Photo{}, false, err
	}
	if err = write(ctx); err != nil {
		return core.Photo{}, false, err
	}
	p, err = ScanPhoto(tx.QueryRow(ctx, `INSERT INTO lumio_photos (id,name,content_type,size,thumb_size,preview_size,width,height,duration)
 VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9) ON CONFLICT(id) DO UPDATE SET name=excluded.name,content_type=excluded.content_type,size=excluded.size,thumb_size=excluded.thumb_size,preview_size=excluded.preview_size,width=excluded.width,height=excluded.height,duration=excluded.duration,added_at=now(),favorite=false,deleted_at=NULL RETURNING `+Columns, p.ID, p.Name, p.Type, p.Size, p.ThumbSize, p.PreviewSize, p.Width, p.Height, p.Duration))
	if err != nil {
		return core.Photo{}, false, err
	}
	return p, false, tx.Commit(ctx)
}
func (r *Repository) Collect(ctx context.Context, remove func(context.Context, string) error) (bool, error) {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return false, err
	}
	defer tx.Rollback(context.Background())
	if err = Guard(ctx, tx, r.storageURL); err != nil {
		return false, err
	}
	var id string
	err = tx.QueryRow(ctx, "SELECT id FROM lumio_photos WHERE deleted_at IS NOT NULL ORDER BY deleted_at LIMIT 1").Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if err = Lock(ctx, tx, id); err != nil {
		return false, err
	}
	var deleted bool
	err = tx.QueryRow(ctx, "SELECT deleted_at IS NOT NULL FROM lumio_photos WHERE id=$1", id).Scan(&deleted)
	if errors.Is(err, pgx.ErrNoRows) {
		return true, nil
	}
	if err != nil {
		return false, err
	}
	if !deleted {
		return true, nil
	}
	if err = remove(ctx, id); err != nil {
		return false, err
	}
	if _, err = tx.Exec(ctx, "DELETE FROM lumio_photos WHERE id=$1", id); err != nil {
		return false, err
	}
	return true, tx.Commit(ctx)
}
func (r *Repository) Timeline(ctx context.Context, zone, query string, favorite bool) ([]core.Month, error) {
	rows, err := r.db.Query(ctx, `SELECT to_char(added_at AT TIME ZONE $1,'YYYY-MM'),count(*) FROM lumio_photos
 WHERE deleted_at IS NULL AND (NOT $2::boolean OR favorite) AND strpos(lower(name),lower($3))>0 GROUP BY 1 ORDER BY 1 DESC LIMIT 1200`, zone, favorite, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	months := make([]core.Month, 0)
	for rows.Next() {
		var m core.Month
		if err := rows.Scan(&m.Month, &m.Count); err != nil {
			return nil, err
		}
		months = append(months, m)
	}
	return months, rows.Err()
}
func (r *Repository) Stats(ctx context.Context) (s core.Stats, err error) {
	err = r.db.QueryRow(ctx, `SELECT count(*) FILTER(WHERE deleted_at IS NULL),count(*) FILTER(WHERE deleted_at IS NULL AND favorite),
 coalesce(sum(size) FILTER(WHERE deleted_at IS NULL),0)::bigint,coalesce(sum(thumb_size) FILTER(WHERE deleted_at IS NULL),0)::bigint,
 coalesce(sum(preview_size) FILTER(WHERE deleted_at IS NULL),0)::bigint,count(*) FILTER(WHERE deleted_at IS NOT NULL) FROM lumio_photos`).Scan(&s.Photos, &s.Favorites, &s.OriginalBytes, &s.ThumbnailBytes, &s.PreviewBytes, &s.PendingDeletions)
	return
}
func (r *Repository) Select(ctx context.Context, ids []string) ([]core.Photo, error) {
	rows, err := r.db.Query(ctx, "SELECT "+Columns+" FROM lumio_photos WHERE id=ANY($1::text[]) AND deleted_at IS NULL", ids)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	byID := make(map[string]core.Photo, len(ids))
	for rows.Next() {
		p, err := ScanPhoto(rows)
		if err != nil {
			return nil, err
		}
		byID[p.ID] = p
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	photos := make([]core.Photo, 0, len(ids))
	for _, id := range ids {
		p, ok := byID[id]
		if !ok {
			return nil, core.ErrNotFound
		}
		photos = append(photos, p)
	}
	return photos, nil
}
func (r *Repository) CreateArchive(ctx context.Context, id string, ids []string) (expires time.Time, err error) {
	err = r.db.QueryRow(ctx, "INSERT INTO lumio_photo_archives(id,photo_ids) VALUES($1,$2) RETURNING expires_at", id, ids).Scan(&expires)
	return
}
func (r *Repository) ArchiveIDs(ctx context.Context, id string) ([]string, error) {
	var ids []string
	err := r.db.QueryRow(ctx, "SELECT photo_ids FROM lumio_photo_archives WHERE id=$1 AND expires_at>now()", id).Scan(&ids)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, core.ErrArchiveNotFound
	}
	return ids, err
}
func (r *Repository) Attempt(ctx context.Context) (attempts int, err error) {
	err = r.db.QueryRow(ctx, `INSERT INTO lumio_login_limits(minute,attempts) VALUES(date_trunc('minute',now()),1)
 ON CONFLICT(minute) DO UPDATE SET attempts=lumio_login_limits.attempts+1 RETURNING attempts`).Scan(&attempts)
	return
}
func (r *Repository) Valid(ctx context.Context, hash, tag []byte) (exists bool, err error) {
	err = r.db.QueryRow(ctx, "SELECT EXISTS(SELECT 1 FROM lumio_sessions WHERE token_hash=$1 AND password_tag=$2 AND expires_at>now())", hash, tag).Scan(&exists)
	return
}
func (r *Repository) SaveSession(ctx context.Context, hash, tag []byte, expires time.Time) error {
	_, err := r.db.Exec(ctx, "INSERT INTO lumio_sessions(token_hash,password_tag,expires_at) VALUES($1,$2,$3)", hash, tag, expires)
	return err
}
func (r *Repository) Revoke(ctx context.Context, hash []byte) error {
	_, err := r.db.Exec(ctx, "DELETE FROM lumio_sessions WHERE token_hash=$1", hash)
	return err
}
func (r *Repository) Ready(ctx context.Context) error {
	var current string
	if err := r.db.QueryRow(ctx, "SELECT storage_url FROM lumio_settings WHERE id=true").Scan(&current); err != nil {
		return err
	}
	if current != r.storageURL {
		return errors.New("storage configuration outdated")
	}
	return nil
}
func (r *Repository) Prune(ctx context.Context) error {
	for _, sql := range []string{"DELETE FROM lumio_sessions WHERE expires_at<now()", "DELETE FROM lumio_login_limits WHERE minute<now()-interval '5 minutes'", "DELETE FROM lumio_photo_archives WHERE expires_at<now()"} {
		if _, err := r.db.Exec(ctx, sql); err != nil {
			return err
		}
	}
	return nil
}
