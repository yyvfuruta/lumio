package httpapi

import (
	"archive/zip"
	"context"
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

	"github.com/yyvfuruta/lumio/internal/core"
)

const maxArchivePhotos = 1000

func (a *API) archivePhotos(ctx context.Context, ids []string) ([]Photo, error) {
	return a.library.Select(ctx, ids)
}

func (a *API) createArchive(w http.ResponseWriter, r *http.Request) {
	var input struct {
		PhotoIDs []string `json:"photoIds"`
	}
	if !decodeJSONLimit(w, r, &input, 128<<10) {
		return
	}
	archive, err := a.library.CreateArchive(r.Context(), input.PhotoIDs)
	if errors.Is(err, core.ErrNotFound) {
		problem(w, 404, "a selected photo no longer exists")
		return
	}
	if err != nil {
		coreProblem(w, err)
		return
	}
	url := "/api/v1/photo-archives/" + archive.ID
	w.Header().Set("Location", url)
	jsonResponse(w, 201, map[string]any{"id": archive.ID, "url": url, "expiresAt": archive.ExpiresAt, "photos": archive.Photos})
}

func (a *API) downloadArchive(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	photos, err := a.library.ArchivePhotos(r.Context(), id)
	if errors.Is(err, core.ErrNotFound) {
		problem(w, 409, "a selected photo has been deleted; create a new download")
		return
	}
	if err != nil {
		coreProblem(w, err)
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

func (a *API) writeArchivePhoto(ctx context.Context, writer *zip.Writer, p Photo, name string) error {
	r, err := a.library.Open(ctx, p.ID+"/original", 0)
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
