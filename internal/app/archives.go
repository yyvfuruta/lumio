package app

import (
	"archive/zip"
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"mime"
	"net/http"
	"path"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/jackc/pgx/v5"
)

const maxArchivePhotos = 1000

func (a *App) archivePhotos(ctx context.Context, ids []string) ([]Photo, error) {
	rows, err := a.db.Query(ctx, "SELECT "+columns+" FROM lumio_photos WHERE id=ANY($1::text[]) AND deleted_at IS NULL", ids)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	byID := make(map[string]Photo, len(ids))
	for rows.Next() {
		p, err := scanPhoto(rows)
		if err != nil {
			return nil, err
		}
		byID[p.ID] = p
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	photos := make([]Photo, 0, len(ids))
	for _, id := range ids {
		p, ok := byID[id]
		if !ok {
			return nil, pgx.ErrNoRows
		}
		photos = append(photos, p)
	}
	return photos, nil
}

func (a *App) createArchive(w http.ResponseWriter, r *http.Request) {
	var input struct {
		PhotoIDs []string `json:"photoIds"`
	}
	if !decodeJSONLimit(w, r, &input, 128<<10) {
		return
	}
	if len(input.PhotoIDs) == 0 || len(input.PhotoIDs) > maxArchivePhotos {
		problem(w, 400, "select between 1 and 1000 photos")
		return
	}
	ids := make([]string, 0, len(input.PhotoIDs))
	seen := make(map[string]bool, len(input.PhotoIDs))
	for _, id := range input.PhotoIDs {
		if !validID(id) {
			problem(w, 400, "invalid photo ID")
			return
		}
		if !seen[id] {
			ids = append(ids, id)
			seen[id] = true
		}
	}
	if _, err := a.archivePhotos(r.Context(), ids); errors.Is(err, pgx.ErrNoRows) {
		problem(w, 404, "a selected photo no longer exists")
		return
	} else if err != nil {
		internal(w, err)
		return
	}
	var random [16]byte
	if _, err := rand.Read(random[:]); err != nil {
		internal(w, err)
		return
	}
	id := hex.EncodeToString(random[:])
	var expires time.Time
	if err := a.db.QueryRow(r.Context(), "INSERT INTO lumio_photo_archives(id,photo_ids) VALUES($1,$2) RETURNING expires_at", id, ids).Scan(&expires); err != nil {
		internal(w, err)
		return
	}
	url := "/api/v1/photo-archives/" + id
	w.Header().Set("Location", url)
	jsonResponse(w, 201, map[string]any{"id": id, "url": url, "expiresAt": expires.UTC(), "photos": len(ids)})
}

func (a *App) downloadArchive(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if len(id) != 32 {
		problem(w, 404, "archive not found or expired")
		return
	}
	if _, err := hex.DecodeString(id); err != nil {
		problem(w, 404, "archive not found or expired")
		return
	}
	var ids []string
	err := a.db.QueryRow(r.Context(), "SELECT photo_ids FROM lumio_photo_archives WHERE id=$1 AND expires_at>now()", id).Scan(&ids)
	if errors.Is(err, pgx.ErrNoRows) {
		problem(w, 404, "archive not found or expired")
		return
	}
	if err != nil {
		internal(w, err)
		return
	}
	photos, err := a.archivePhotos(r.Context(), ids)
	if errors.Is(err, pgx.ErrNoRows) {
		problem(w, 409, "a selected photo has been deleted; create a new download")
		return
	}
	if err != nil {
		internal(w, err)
		return
	}
	// Large archives stream directly; the ordinary response timeout must not cut
	// off a slow but active download. Request cancellation still stops reads.
	if err := http.NewResponseController(w).SetWriteDeadline(time.Time{}); err != nil && !errors.Is(err, http.ErrNotSupported) {
		internal(w, err)
		return
	}
	w.Header().Set("Content-Type", "application/zip")
	w.Header().Set("Content-Disposition", mime.FormatMediaType("attachment", map[string]string{"filename": "lumio-photos-" + time.Now().UTC().Format("2006-01-02") + ".zip"}))
	if r.Method == http.MethodHead {
		w.WriteHeader(http.StatusOK)
		return
	}
	writer := zip.NewWriter(w)
	used := make(map[string]bool, len(photos))
	for _, p := range photos {
		name := archiveFilename(p, used)
		if err := a.writeArchivePhoto(r.Context(), writer, p, name); err != nil {
			slog.Error("photo archive interrupted", "archive_id", id, "photo_id", p.ID, "error", err)
			// Abort instead of finalizing a misleading, incomplete ZIP file.
			panic(http.ErrAbortHandler)
		}
	}
	if err := writer.Close(); err != nil {
		slog.Error("photo archive interrupted", "archive_id", id, "error", err)
		panic(http.ErrAbortHandler)
	}
}

func (a *App) writeArchivePhoto(ctx context.Context, writer *zip.Writer, p Photo, name string) error {
	r, err := a.bucket.NewReader(ctx, p.ID+"/original", nil)
	if err != nil {
		return err
	}
	defer r.Close()
	header := &zip.FileHeader{Name: name, Method: zip.Store, Modified: p.AddedAt}
	header.SetMode(0600)
	w, err := writer.CreateHeader(header)
	if err != nil {
		return err
	}
	n, err := io.Copy(w, r)
	if err != nil {
		return err
	}
	if n != p.Size {
		return fmt.Errorf("original size mismatch: got %d, expected %d", n, p.Size)
	}
	return nil
}

func archiveFilename(p Photo, used map[string]bool) string {
	name := path.Base(strings.ReplaceAll(p.Name, "\\", "/"))
	if name == "." || name == "/" || name == "" {
		name = p.ID
	}
	if used[strings.ToLower(name)] {
		ext := path.Ext(name)
		stem := strings.TrimSuffix(name, ext)
		if len(ext) > 10 {
			ext = ""
		}
		for len(stem) > 180 {
			_, size := utf8.DecodeLastRuneInString(stem)
			stem = stem[:len(stem)-size]
		}
		name = stem + "-" + p.ID + ext
	}
	used[strings.ToLower(name)] = true
	return name
}
