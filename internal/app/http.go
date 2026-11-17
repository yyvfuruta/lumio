package app

import (
	"context"
	"embed"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"io/fs"
	"log/slog"
	"mime"
	"net/http"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
)

//go:embed web/*
var assets embed.FS

func (a *App) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /livez", func(w http.ResponseWriter, r *http.Request) { w.Write([]byte("ok")) })
	mux.HandleFunc("GET /readyz", func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
		defer cancel()
		var current string
		if err := a.db.QueryRow(ctx, "SELECT storage_url FROM lumio_settings WHERE id=true").Scan(&current); err != nil || current != a.cfg.StorageURL {
			problem(w, 503, "database unavailable or storage configuration outdated")
			return
		}
		w.Write([]byte("ok"))
	})
	mux.HandleFunc("GET /api/v1/sessions/current", func(w http.ResponseWriter, r *http.Request) {
		jsonResponse(w, 200, map[string]any{"authenticated": a.authenticated(r), "maxUploadBytes": a.cfg.MaxUploadBytes})
	})
	mux.HandleFunc("POST /api/v1/sessions", a.login)
	mux.HandleFunc("DELETE /api/v1/sessions/current", a.logout)
	mux.HandleFunc("GET /api/v1/photos", a.protect(a.listPhotos))
	mux.HandleFunc("GET /api/v1/photos/timeline", a.protect(a.timeline))
	mux.HandleFunc("GET /api/v1/status", a.protect(a.status))
	mux.HandleFunc("POST /api/v1/photo-archives", a.protect(a.createArchive))
	mux.HandleFunc("GET /api/v1/photo-archives/{id}", a.protect(a.downloadArchive))
	mux.HandleFunc("POST /api/v1/photos", a.protect(a.upload))
	mux.HandleFunc("GET /api/v1/photos/{id}", a.protect(a.getPhoto))
	mux.HandleFunc("PATCH /api/v1/photos/{id}", a.protect(a.favorite))
	mux.HandleFunc("DELETE /api/v1/photos/{id}", a.protect(a.deletePhoto))
	mux.HandleFunc("GET /api/v1/photos/{id}/{kind}", a.protect(a.media))
	web, _ := fs.Sub(assets, "web")
	mux.Handle("GET /", http.FileServer(http.FS(web)))
	return accessLog(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Referrer-Policy", "same-origin")
		w.Header().Set("Content-Security-Policy", "default-src 'self'; img-src 'self'; style-src 'self'; script-src 'self'; frame-ancestors 'none'; base-uri 'none'; form-action 'self'")
		w.Header().Set("Cache-Control", "no-store")
		if r.Method != "GET" && r.Method != "HEAD" && !safeMutation(r) {
			problem(w, 403, "request not allowed")
			return
		}
		mux.ServeHTTP(w, r)
	}))
}

func (a *App) listPhotos(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	search := q.Get("q")
	if len(search) > 200 {
		problem(w, 400, "search query is too long")
		return
	}
	from, until, err := photoRange(q.Get("from"), q.Get("until"))
	if err != nil {
		problem(w, 400, err.Error())
		return
	}
	before := time.Now().Add(24 * time.Hour)
	beforeID := strings.Repeat("f", 64)
	if cursor := q.Get("cursor"); cursor != "" {
		raw, err := base64.RawURLEncoding.DecodeString(cursor)
		parts := strings.Split(string(raw), "|")
		if err != nil || len(parts) != 2 || !validID(parts[1]) {
			problem(w, 400, "invalid cursor")
			return
		}
		before, err = time.Parse(time.RFC3339Nano, parts[0])
		if err != nil {
			problem(w, 400, "invalid cursor")
			return
		}
		beforeID = parts[1]
	}
	rows, err := a.db.Query(r.Context(), "SELECT "+columns+` FROM lumio_photos WHERE deleted_at IS NULL AND (NOT $1::boolean OR favorite)
	 AND strpos(lower(name),lower($2))>0 AND (added_at,id)<($3,$4)
	 AND ($5::timestamptz IS NULL OR added_at >= $5) AND ($6::timestamptz IS NULL OR added_at < $6)
	 ORDER BY added_at DESC,id DESC LIMIT 61`, q.Get("favorite") == "true", search, before, beforeID, from, until)
	if err != nil {
		internal(w, err)
		return
	}
	defer rows.Close()
	photos := make([]Photo, 0, 61)
	for rows.Next() {
		p, err := scanPhoto(rows)
		if err != nil {
			internal(w, err)
			return
		}
		photos = append(photos, p)
	}
	if err := rows.Err(); err != nil {
		internal(w, err)
		return
	}
	next := ""
	if len(photos) > 60 {
		photos = photos[:60]
		p := photos[59]
		next = base64.RawURLEncoding.EncodeToString([]byte(p.AddedAt.Format(time.RFC3339Nano) + "|" + p.ID))
	}
	jsonResponse(w, 200, map[string]any{"photos": photos, "nextCursor": next})
}

func (a *App) upload(w http.ResponseWriter, r *http.Request) {
	select {
	case a.uploadGate <- struct{}{}:
		defer func() { <-a.uploadGate }()
	default:
		w.Header().Set("Retry-After", "3")
		problem(w, 503, "another upload is in progress; try again")
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, a.cfg.MaxUploadBytes+(1<<20))
	reader, err := r.MultipartReader()
	if err != nil {
		problem(w, 400, "send multipart/form-data with one file in the photo field")
		return
	}
	part, err := reader.NextPart()
	if err != nil || part.FormName() != "photo" || part.FileName() == "" {
		problem(w, 400, "select a photo or video")
		return
	}
	defer part.Close()
	p, duplicate, err := a.addPhoto(r.Context(), part.FileName(), part)
	if err != nil {
		var maxErr *http.MaxBytesError
		if errors.Is(err, errTooLarge) || errors.As(err, &maxErr) {
			problem(w, 413, "file exceeds the upload limit")
			return
		}
		if errors.Is(err, errVideoTools) {
			problem(w, 503, "video uploads require FFmpeg and FFprobe on the server")
			return
		}
		if errors.Is(err, errImage) {
			problem(w, 415, err.Error())
			return
		}
		internal(w, err)
		return
	}
	status := http.StatusCreated
	if duplicate {
		status = http.StatusOK
	}
	w.Header().Set("Location", "/api/v1/photos/"+p.ID)
	jsonResponse(w, status, map[string]any{"photo": p, "duplicate": duplicate})
}

func (a *App) getPhoto(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if !validID(id) {
		problem(w, 404, "photo not found")
		return
	}
	p, err := scanPhoto(a.db.QueryRow(r.Context(), "SELECT "+columns+" FROM lumio_photos WHERE id=$1 AND deleted_at IS NULL", id))
	if errors.Is(err, pgx.ErrNoRows) {
		problem(w, 404, "photo not found")
		return
	}
	if err != nil {
		internal(w, err)
		return
	}
	jsonResponse(w, 200, p)
}

func (a *App) favorite(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if !validID(id) {
		problem(w, 404, "photo not found")
		return
	}
	var input struct {
		Favorite *bool `json:"favorite"`
	}
	if !decodeJSON(w, r, &input) {
		return
	}
	if input.Favorite == nil {
		problem(w, 400, "provide favorite as true or false")
		return
	}
	p, err := scanPhoto(a.db.QueryRow(r.Context(), "UPDATE lumio_photos SET favorite=$2 WHERE id=$1 AND deleted_at IS NULL RETURNING "+columns, id, *input.Favorite))
	if errors.Is(err, pgx.ErrNoRows) {
		problem(w, 404, "photo not found")
		return
	}
	if err != nil {
		internal(w, err)
		return
	}
	jsonResponse(w, 200, p)
}

func (a *App) deletePhoto(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if !validID(id) {
		problem(w, 404, "photo not found")
		return
	}
	// Use the same advisory lock as uploads and garbage collection.
	tx, err := a.db.Begin(r.Context())
	if err != nil {
		internal(w, err)
		return
	}
	defer tx.Rollback(context.Background())
	if err := a.storageGuard(r.Context(), tx); err != nil {
		internal(w, err)
		return
	}
	if err := photoLock(r.Context(), tx, id); err != nil {
		internal(w, err)
		return
	}
	tag, err := tx.Exec(r.Context(), "UPDATE lumio_photos SET deleted_at=now() WHERE id=$1 AND deleted_at IS NULL", id)
	if err != nil {
		internal(w, err)
		return
	}
	if tag.RowsAffected() == 0 {
		problem(w, 404, "photo not found")
		return
	}
	if err := tx.Commit(r.Context()); err != nil {
		internal(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (a *App) media(w http.ResponseWriter, r *http.Request) {
	id, kind := r.PathValue("id"), r.PathValue("kind")
	if !validID(id) || (kind != "original" && kind != "thumbnail" && kind != "preview") {
		problem(w, 404, "photo not found")
		return
	}
	p, err := scanPhoto(a.db.QueryRow(r.Context(), "SELECT "+columns+" FROM lumio_photos WHERE id=$1 AND deleted_at IS NULL", id))
	if errors.Is(err, pgx.ErrNoRows) {
		problem(w, 404, "photo not found")
		return
	}
	if err != nil {
		internal(w, err)
		return
	}
	key, typ, size := "original", p.Type, p.Size
	if kind == "thumbnail" {
		key, typ, size = "thumb.jpg", "image/jpeg", p.ThumbSize
	}
	if kind == "preview" && p.PreviewSize > 0 {
		key, typ, size = "preview.jpg", "image/jpeg", p.PreviewSize
	}
	reader, err := a.bucket.NewReader(r.Context(), id+"/"+key, nil)
	if err != nil {
		internal(w, err)
		return
	}
	content := &blobReadSeeker{ctx: r.Context(), bucket: a.bucket, key: id + "/" + key, size: size, reader: reader}
	defer content.Close()
	w.Header().Set("Content-Type", typ)
	if r.URL.Query().Get("download") == "1" {
		w.Header().Set("Content-Disposition", mime.FormatMediaType("attachment", map[string]string{"filename": p.Name}))
	}
	http.ServeContent(w, r, p.Name, p.AddedAt, content)
}

func decodeJSON(w http.ResponseWriter, r *http.Request, dst any) bool {
	return decodeJSONLimit(w, r, dst, 8192)
}

func decodeJSONLimit(w http.ResponseWriter, r *http.Request, dst any, limit int64) bool {
	typ, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil || typ != "application/json" {
		problem(w, 415, "use Content-Type: application/json")
		return false
	}
	r.Body = http.MaxBytesReader(w, r.Body, limit)
	d := json.NewDecoder(r.Body)
	d.DisallowUnknownFields()
	if err := d.Decode(dst); err != nil {
		problem(w, 400, "invalid JSON")
		return false
	}
	if err := d.Decode(&struct{}{}); err != io.EOF {
		problem(w, 400, "invalid JSON")
		return false
	}
	return true
}

func jsonResponse(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(value)
}
func problem(w http.ResponseWriter, status int, message string) {
	jsonResponse(w, status, map[string]string{"error": message})
}
func internal(w http.ResponseWriter, err error) {
	slog.Error("request failed", "error", err)
	problem(w, 500, "could not complete the request; try again")
}
