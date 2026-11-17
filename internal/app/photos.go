package app

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"image"
	"image/color"
	_ "image/gif"
	"image/jpeg"
	_ "image/png"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"gocloud.dev/blob"
	"gocloud.dev/gcerrors"
)

var errImage = errors.New("use a valid JPEG, PNG, or GIF image up to 40 megapixels")
var errTooLarge = errors.New("file exceeds the upload limit")

type Photo struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	Type      string    `json:"type"`
	Size      int64     `json:"size"`
	ThumbSize int64     `json:"-"`
	Width     int       `json:"width"`
	Height    int       `json:"height"`
	AddedAt   time.Time `json:"addedAt"`
	Favorite  bool      `json:"favorite"`
}

const columns = "id,name,content_type,size,thumb_size,width,height,added_at,favorite"

func scanPhoto(row pgx.Row) (p Photo, err error) {
	err = row.Scan(&p.ID, &p.Name, &p.Type, &p.Size, &p.ThumbSize, &p.Width, &p.Height, &p.AddedAt, &p.Favorite)
	return
}

func validID(id string) bool {
	if len(id) != 64 {
		return false
	}
	_, err := hex.DecodeString(id)
	return err == nil && id == strings.ToLower(id)
}

func photoLock(ctx context.Context, tx pgx.Tx, id string) error {
	hash := sha256.Sum256([]byte(id))
	_, err := tx.Exec(ctx, "SELECT pg_advisory_xact_lock($1)", int64(binary.BigEndian.Uint64(hash[:8])))
	return err
}

func (a *App) addPhoto(ctx context.Context, name string, src io.Reader) (Photo, bool, error) {
	f, err := os.CreateTemp("", "lumio-upload-*")
	if err != nil {
		return Photo{}, false, err
	}
	defer os.Remove(f.Name())
	defer f.Close()
	hash := sha256.New()
	n, err := io.Copy(io.MultiWriter(f, hash), io.LimitReader(src, a.cfg.MaxUploadBytes+1))
	if err != nil {
		return Photo{}, false, err
	}
	if n > a.cfg.MaxUploadBytes {
		return Photo{}, false, errTooLarge
	}
	if _, err := f.Seek(0, 0); err != nil {
		return Photo{}, false, err
	}
	cfg, format, err := image.DecodeConfig(f)
	if err != nil || cfg.Width <= 0 || cfg.Height <= 0 || int64(cfg.Width)*int64(cfg.Height) > 40_000_000 {
		return Photo{}, false, errImage
	}
	mime := map[string]string{"jpeg": "image/jpeg", "png": "image/png", "gif": "image/gif"}[format]
	if mime == "" {
		return Photo{}, false, errImage
	}
	if _, err := f.Seek(0, 0); err != nil {
		return Photo{}, false, err
	}
	img, _, err := image.Decode(f)
	if err != nil {
		return Photo{}, false, errImage
	}
	var thumb bytes.Buffer
	if err := jpeg.Encode(&thumb, thumbnail(img), &jpeg.Options{Quality: 82}); err != nil {
		return Photo{}, false, err
	}
	p := Photo{ID: hex.EncodeToString(hash.Sum(nil)), Name: filepath.Base(strings.ReplaceAll(name, "\\", "/")), Type: mime, Size: n, ThumbSize: int64(thumb.Len()), Width: cfg.Width, Height: cfg.Height}
	if len(p.Name) > 255 {
		p.Name = "photo"
	}
	tx, err := a.db.Begin(ctx)
	if err != nil {
		return Photo{}, false, err
	}
	defer tx.Rollback(context.Background())
	if err := a.storageGuard(ctx, tx); err != nil {
		return Photo{}, false, err
	}
	if err := photoLock(ctx, tx, p.ID); err != nil {
		return Photo{}, false, err
	}
	existing, err := scanPhoto(tx.QueryRow(ctx, "SELECT "+columns+" FROM lumio_photos WHERE id=$1 AND deleted_at IS NULL", p.ID))
	if err == nil {
		return existing, true, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return Photo{}, false, err
	}
	// Lock spans both storage writes and metadata commit, coordinating all replicas.
	if _, err := f.Seek(0, 0); err != nil {
		return Photo{}, false, err
	}
	if err := a.bucket.Upload(ctx, p.ID+"/original", f, &blob.WriterOptions{ContentType: p.Type}); err != nil {
		return Photo{}, false, err
	}
	if err := a.bucket.Upload(ctx, p.ID+"/thumb.jpg", &thumb, &blob.WriterOptions{ContentType: "image/jpeg"}); err != nil {
		return Photo{}, false, err
	}
	p, err = scanPhoto(tx.QueryRow(ctx, `INSERT INTO lumio_photos (id,name,content_type,size,thumb_size,width,height)
	 VALUES ($1,$2,$3,$4,$5,$6,$7) ON CONFLICT(id) DO UPDATE SET name=excluded.name,content_type=excluded.content_type,size=excluded.size,thumb_size=excluded.thumb_size,width=excluded.width,height=excluded.height,added_at=now(),favorite=false,deleted_at=NULL RETURNING `+columns, p.ID, p.Name, p.Type, p.Size, p.ThumbSize, p.Width, p.Height))
	if err != nil {
		return Photo{}, false, err
	}
	return p, false, tx.Commit(ctx)
}

// Tombstones hide photos immediately; cleanup is idempotent and retried on failure.
func (a *App) collectOne(ctx context.Context) (bool, error) {
	tx, err := a.db.Begin(ctx)
	if err != nil {
		return false, err
	}
	defer tx.Rollback(context.Background())
	if err := a.storageGuard(ctx, tx); err != nil {
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
	if err := photoLock(ctx, tx, id); err != nil {
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
	for _, key := range []string{id + "/original", id + "/thumb.jpg"} {
		if err := a.bucket.Delete(ctx, key); err != nil && gcerrors.Code(err) != gcerrors.NotFound {
			return false, err
		}
	}
	if _, err := tx.Exec(ctx, "DELETE FROM lumio_photos WHERE id=$1", id); err != nil {
		return false, err
	}
	return true, tx.Commit(ctx)
}

func thumbnail(src image.Image) image.Image {
	b := src.Bounds()
	w, h := b.Dx(), b.Dy()
	tw, th := w, h
	if w > 640 || h > 640 {
		if w >= h {
			tw, th = 640, max(1, h*640/w)
		} else {
			th, tw = 640, max(1, w*640/h)
		}
	}
	dst := image.NewRGBA(image.Rect(0, 0, tw, th))
	for y := 0; y < th; y++ {
		for x := 0; x < tw; x++ {
			x0, x1, y0, y1 := x*w/tw, (x+1)*w/tw, y*h/th, (y+1)*h/th
			var r, g, bl uint64
			for sy := y0; sy < y1; sy++ {
				for sx := x0; sx < x1; sx++ {
					cr, cg, cb, ca := src.At(b.Min.X+sx, b.Min.Y+sy).RGBA()
					r += uint64(cr + 65535 - ca)
					g += uint64(cg + 65535 - ca)
					bl += uint64(cb + 65535 - ca)
				}
			}
			n := uint64((x1 - x0) * (y1 - y0))
			dst.SetRGBA(x, y, color.RGBA{uint8(r / n >> 8), uint8(g / n >> 8), uint8(bl / n >> 8), 255})
		}
	}
	return dst
}
