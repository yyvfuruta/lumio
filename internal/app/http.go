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
	"strconv"
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
		if err := a.db.Ping(ctx); err != nil {
			problem(w, 503, "banco indisponível")
			return
		}
		w.Write([]byte("ok"))
	})
	mux.HandleFunc("GET /api/session", func(w http.ResponseWriter, r *http.Request) {
		jsonResponse(w, 200, map[string]any{"authenticated": a.authenticated(r), "maxUploadBytes": a.cfg.MaxUploadBytes})
	})
	mux.HandleFunc("POST /api/login", a.login)
	mux.HandleFunc("POST /api/logout", a.logout)
	mux.HandleFunc("GET /api/photos", a.protect(a.listPhotos))
	mux.HandleFunc("POST /api/photos", a.protect(a.upload))
	mux.HandleFunc("PATCH /api/photos/{id}", a.protect(a.favorite))
	mux.HandleFunc("DELETE /api/photos/{id}", a.protect(a.deletePhoto))
	mux.HandleFunc("GET /media/{id}/{kind}", a.protect(a.media))
	web, _ := fs.Sub(assets, "web")
	mux.Handle("GET /", http.FileServer(http.FS(web)))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Referrer-Policy", "same-origin")
		w.Header().Set("Content-Security-Policy", "default-src 'self'; img-src 'self'; style-src 'self'; script-src 'self'; frame-ancestors 'none'; base-uri 'none'; form-action 'self'")
		w.Header().Set("Cache-Control", "no-store")
		if r.Method != "GET" && r.Method != "HEAD" && !safeMutation(r) {
			problem(w, 403, "requisição não permitida")
			return
		}
		mux.ServeHTTP(w, r)
	})
}

func (a *App) listPhotos(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	search := q.Get("q")
	if len(search) > 200 {
		problem(w, 400, "busca muito longa")
		return
	}
	before := time.Now().Add(24 * time.Hour)
	beforeID := strings.Repeat("f", 64)
	if cursor := q.Get("cursor"); cursor != "" {
		raw, err := base64.RawURLEncoding.DecodeString(cursor)
		parts := strings.Split(string(raw), "|")
		if err != nil || len(parts) != 2 || !validID(parts[1]) {
			problem(w, 400, "cursor inválido")
			return
		}
		before, err = time.Parse(time.RFC3339Nano, parts[0])
		if err != nil {
			problem(w, 400, "cursor inválido")
			return
		}
		beforeID = parts[1]
	}
	rows, err := a.db.Query(r.Context(), "SELECT "+columns+` FROM lumio_photos WHERE deleted_at IS NULL AND (NOT $1::boolean OR favorite)
	 AND strpos(lower(name),lower($2))>0 AND (added_at,id)<($3,$4) ORDER BY added_at DESC,id DESC LIMIT 61`, q.Get("favorite") == "true", search, before, beforeID)
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
		problem(w, 503, "outro envio em andamento; tente novamente")
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, a.cfg.MaxUploadBytes+(1<<20))
	reader, err := r.MultipartReader()
	if err != nil {
		problem(w, 400, "envie multipart/form-data com um arquivo no campo photo")
		return
	}
	part, err := reader.NextPart()
	if err != nil || part.FormName() != "photo" || part.FileName() == "" {
		problem(w, 400, "selecione uma foto")
		return
	}
	defer part.Close()
	p, duplicate, err := a.addPhoto(r.Context(), part.FileName(), part)
	if err != nil {
		var maxErr *http.MaxBytesError
		if errors.Is(err, errTooLarge) || errors.As(err, &maxErr) {
			problem(w, 413, "arquivo excede o limite de upload")
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
	jsonResponse(w, status, map[string]any{"photo": p, "duplicate": duplicate})
}

func (a *App) favorite(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if !validID(id) {
		problem(w, 404, "foto não encontrada")
		return
	}
	var input struct {
		Favorite bool `json:"favorite"`
	}
	if !decodeJSON(w, r, &input) {
		return
	}
	p, err := scanPhoto(a.db.QueryRow(r.Context(), "UPDATE lumio_photos SET favorite=$2 WHERE id=$1 AND deleted_at IS NULL RETURNING "+columns, id, input.Favorite))
	if errors.Is(err, pgx.ErrNoRows) {
		problem(w, 404, "foto não encontrada")
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
		problem(w, 404, "foto não encontrada")
		return
	}
	// Use the same advisory lock as uploads and garbage collection.
	tx, err := a.db.Begin(r.Context())
	if err != nil {
		internal(w, err)
		return
	}
	defer tx.Rollback(context.Background())
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
		problem(w, 404, "foto não encontrada")
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
	if !validID(id) || (kind != "original" && kind != "thumb.jpg") {
		problem(w, 404, "foto não encontrada")
		return
	}
	p, err := scanPhoto(a.db.QueryRow(r.Context(), "SELECT "+columns+" FROM lumio_photos WHERE id=$1 AND deleted_at IS NULL", id))
	if errors.Is(err, pgx.ErrNoRows) {
		problem(w, 404, "foto não encontrada")
		return
	}
	if err != nil {
		internal(w, err)
		return
	}
	reader, err := a.bucket.NewReader(r.Context(), id+"/"+kind, nil)
	if err != nil {
		internal(w, err)
		return
	}
	defer reader.Close()
	typ, size := p.Type, p.Size
	if kind == "thumb.jpg" {
		typ, size = "image/jpeg", p.ThumbSize
	}
	w.Header().Set("Content-Type", typ)
	w.Header().Set("Content-Length", strconv.FormatInt(size, 10))
	if r.URL.Query().Get("download") == "1" {
		w.Header().Set("Content-Disposition", mime.FormatMediaType("attachment", map[string]string{"filename": p.Name}))
	}
	if _, err := io.Copy(w, reader); err != nil {
		slog.Error("leitura da foto interrompida", "error", err)
	}
}

func decodeJSON(w http.ResponseWriter, r *http.Request, dst any) bool {
	r.Body = http.MaxBytesReader(w, r.Body, 8192)
	d := json.NewDecoder(r.Body)
	d.DisallowUnknownFields()
	if err := d.Decode(dst); err != nil {
		problem(w, 400, "JSON inválido")
		return false
	}
	if err := d.Decode(&struct{}{}); err != io.EOF {
		problem(w, 400, "JSON inválido")
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
	slog.Error("erro na requisição", "error", err)
	problem(w, 500, "não foi possível concluir; tente novamente")
}
