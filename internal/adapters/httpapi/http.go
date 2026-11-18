package httpapi

import (
	"context"
	"embed"
	"encoding/json"
	"errors"
	"io"
	"io/fs"
	"log/slog"
	"mime"
	"net/http"
	"time"

	"github.com/yyvfuruta/lumio/internal/core"
)

//go:embed web/*
var assets embed.FS

func (a *API) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1", apiRoot)
	mux.HandleFunc("GET /api/v1/{$}", apiRoot)
	mux.HandleFunc("GET /livez", func(w http.ResponseWriter, r *http.Request) { w.Write([]byte("ok")) })
	mux.HandleFunc("GET /readyz", func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
		defer cancel()
		if err := a.ready(ctx); err != nil {
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
		w.Header().Add("Link", "</api/v1>; rel=\"index\"")
		if r.Method != "GET" && r.Method != "HEAD" && !safeMutation(r) {
			problem(w, 403, "request not allowed")
			return
		}
		mux.ServeHTTP(w, r)
	}))
}

func (a *API) listPhotos(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	page, err := a.library.List(r.Context(), core.ListInput{Query: q.Get("q"), Favorite: q.Get("favorite") == "true", Cursor: q.Get("cursor"), From: q.Get("from"), Until: q.Get("until")})
	if err != nil {
		coreProblem(w, err)
		return
	}
	if page.NextCursor != "" {
		q.Set("cursor", page.NextCursor)
		w.Header().Add("Link", "</api/v1/photos?"+q.Encode()+">; rel=\"next\"")
	}
	photos := make([]photoRepresentation, 0, len(page.Photos))
	for _, p := range page.Photos {
		photos = append(photos, representPhoto(p))
	}
	links := map[string]resourceLink{"self": {Href: r.URL.RequestURI(), Method: "GET"}, "upload": {Href: "/api/v1/photos", Method: "POST"}}
	if page.NextCursor != "" {
		links["next"] = resourceLink{Href: "/api/v1/photos?" + q.Encode(), Method: "GET"}
	}
	jsonResponse(w, 200, map[string]any{"photos": photos, "nextCursor": page.NextCursor, "links": links})
}

func (a *API) upload(w http.ResponseWriter, r *http.Request) {
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
		if errors.Is(err, core.ErrTooLarge) || errors.As(err, &maxErr) {
			problem(w, 413, "file exceeds the upload limit")
			return
		}
		if errors.Is(err, core.ErrVideoTools) {
			problem(w, 503, "video uploads require FFmpeg and FFprobe on the server")
			return
		}
		if errors.Is(err, core.ErrMedia) {
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
	jsonResponse(w, status, map[string]any{"photo": representPhoto(p), "duplicate": duplicate})
}

func (a *API) getPhoto(w http.ResponseWriter, r *http.Request) {
	p, err := a.library.Get(r.Context(), r.PathValue("id"))
	if err != nil {
		coreProblem(w, err)
		return
	}
	jsonResponse(w, 200, representPhoto(p))
}
func (a *API) favorite(w http.ResponseWriter, r *http.Request) {
	if !validID(r.PathValue("id")) {
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
	p, err := a.library.Favorite(r.Context(), r.PathValue("id"), *input.Favorite)
	if err != nil {
		coreProblem(w, err)
		return
	}
	jsonResponse(w, 200, representPhoto(p))
}
func (a *API) deletePhoto(w http.ResponseWriter, r *http.Request) {
	if err := a.library.Delete(r.Context(), r.PathValue("id")); err != nil {
		coreProblem(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
func (a *API) media(w http.ResponseWriter, r *http.Request) {
	id, kind := r.PathValue("id"), r.PathValue("kind")
	if !validID(id) || (kind != "original" && kind != "thumbnail" && kind != "preview") {
		problem(w, 404, "photo not found")
		return
	}
	p, err := a.library.Get(r.Context(), id)
	if err != nil {
		coreProblem(w, err)
		return
	}
	key, typ, size := "original", p.Type, p.Size
	if kind == "thumbnail" {
		key, typ, size = "thumb.jpg", "image/jpeg", p.ThumbSize
	}
	if kind == "preview" && p.PreviewSize > 0 {
		key, typ, size = "preview.jpg", "image/jpeg", p.PreviewSize
	}
	reader, err := a.library.Open(r.Context(), id+"/"+key, 0)
	if err != nil {
		internal(w, err)
		return
	}
	content := &blobReadSeeker{ctx: r.Context(), objects: a.library, key: id + "/" + key, size: size, reader: reader}
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

func coreProblem(w http.ResponseWriter, err error) {
	var input *core.InputError
	switch {
	case errors.As(err, &input):
		problem(w, 400, err.Error())
	case errors.Is(err, core.ErrNotFound):
		problem(w, 404, "photo not found")
	case errors.Is(err, core.ErrArchiveNotFound):
		problem(w, 404, "archive not found or expired")
	default:
		internal(w, err)
	}
}
