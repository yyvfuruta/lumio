package app

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"image"
	"image/color"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/gen2brain/h265/heic"
	"github.com/jackc/pgx/v5/pgxpool"
	"gocloud.dev/gcerrors"
)

func featureConfig(t *testing.T, databaseURL, storageURL string) Config {
	t.Helper()
	ctx := context.Background()
	db, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	var random [8]byte
	if _, err := rand.Read(random[:]); err != nil {
		t.Fatal(err)
	}
	schema := "lumio_features_" + hex.EncodeToString(random[:])
	if _, err := db.Exec(ctx, "CREATE SCHEMA "+schema); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Exec(ctx, "DROP SCHEMA "+schema+" CASCADE"); db.Close() })
	u, err := url.Parse(databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	q := u.Query()
	q.Set("search_path", schema)
	u.RawQuery = q.Encode()
	su, err := url.Parse(storageURL)
	if err != nil {
		t.Fatal(err)
	}
	sq := su.Query()
	sq.Set("prefix", sq.Get("prefix")+schema+"/")
	su.RawQuery = sq.Encode()
	return Config{DatabaseURL: u.String(), StorageURL: su.String(), Password: "feature-test-password", MaxUploadBytes: 2 << 20}
}

func encodedHEIC(t *testing.T, width, height, bits int) []byte {
	t.Helper()
	img := image.NewNRGBA(image.Rect(0, 0, width, height))
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			img.SetNRGBA(x, y, color.NRGBA{uint8(x), uint8(y * 3), 90, 255})
		}
	}
	var out bytes.Buffer
	if err := heic.Encode(&out, img, heic.EncodeOptions{Lossless: true, BitDepth: bits}); err != nil {
		t.Fatal(err)
	}
	return out.Bytes()
}

func TestHEICAndArchivesIntegration(t *testing.T) {
	databaseURL := env("LUMIO_TEST_DATABASE_URL", "")
	if databaseURL == "" {
		t.Skip("set LUMIO_TEST_DATABASE_URL for integration tests")
	}
	backends := map[string]string{"file": (&url.URL{Scheme: "file", Path: t.TempDir()}).String()}
	if raw := env("LUMIO_TEST_STORAGE_URL", ""); raw != "" {
		backends["remote"] = raw
	}
	for backend, storageURL := range backends {
		t.Run(backend, func(t *testing.T) { testHEICAndArchives(t, featureConfig(t, databaseURL, storageURL)) })
	}
}

func testHEICAndArchives(t *testing.T, cfg Config) {
	ctx := context.Background()
	var apps []*App
	var photos []Photo
	t.Cleanup(func() {
		for _, a := range apps {
			for _, p := range photos {
				for _, key := range []string{p.ID + "/original", p.ID + "/thumb.jpg", p.ID + "/preview.jpg"} {
					if err := a.bucket.Delete(ctx, key); err != nil && gcerrors.Code(err) != gcerrors.NotFound {
						t.Errorf("cleanup: %v", err)
					}
				}
			}
			a.Close()
		}
	})
	start := func(cfg Config) *App {
		t.Helper()
		a, err := New(cfg)
		if err != nil {
			t.Fatal(err)
		}
		apps = append(apps, a)
		return a
	}
	a, b := start(cfg), start(cfg)
	var cookie *http.Cookie
	request := func(handler http.Handler, method, path string, body io.Reader, typ string) *httptest.ResponseRecorder {
		r := httptest.NewRequest(method, "http://photos.test"+path, body)
		r.Header.Set("X-Lumio-Request", "1")
		if typ != "" {
			r.Header.Set("Content-Type", typ)
		}
		if cookie != nil {
			r.AddCookie(cookie)
		}
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, r)
		return w
	}
	check := func(w *httptest.ResponseRecorder, code int) {
		t.Helper()
		if w.Code != code {
			t.Fatalf("got %d want %d: %s", w.Code, code, w.Body.String())
		}
	}
	check(request(a.Handler(), "POST", "/api/v1/photo-archives", strings.NewReader(`{"photoIds":[]}`), "application/json"), 401)
	check(request(a.Handler(), "GET", "/api/v1/photo-archives/"+strings.Repeat("a", 32), nil, ""), 401)
	login := request(a.Handler(), "POST", "/api/v1/sessions", strings.NewReader(`{"password":"feature-test-password"}`), "application/json")
	check(login, 201)
	cookie = login.Result().Cookies()[0]
	upload := func(name string, data []byte, code int) Photo {
		t.Helper()
		var body bytes.Buffer
		writer := multipart.NewWriter(&body)
		part, err := writer.CreateFormFile("photo", name)
		if err != nil {
			t.Fatal(err)
		}
		part.Write(data)
		writer.Close()
		w := request(a.Handler(), "POST", "/api/v1/photos", &body, writer.FormDataContentType())
		check(w, code)
		if code != 201 && code != 200 {
			return Photo{}
		}
		var result struct {
			Photo Photo `json:"photo"`
		}
		if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
			t.Fatal(err)
		}
		return result.Photo
	}
	originals := [][]byte{encodedHEIC(t, 2304, 64, 8), encodedHEIC(t, 64, 48, 10), testPNG(t)}
	photos = append(photos, upload("CAMERA.HEIC", originals[0], 201), upload("CAMERA.HEIC", originals[1], 201), upload("color.png", originals[2], 201))
	upload("duplicate.heic", originals[0], 200)
	upload("broken.heic", []byte("\x00\x00\x00\x18ftypheic\x00\x00\x00\x00"), 415)
	for i, p := range photos {
		w := request(b.Handler(), "GET", "/api/v1/photos/"+p.ID+"/original?download=1", nil, "")
		check(w, 200)
		if !bytes.Equal(w.Body.Bytes(), originals[i]) {
			t.Fatal("original bytes changed")
		}
		w = request(b.Handler(), "GET", "/api/v1/photos/"+p.ID+"/preview", nil, "")
		check(w, 200)
		if i < 2 {
			if p.Type != "image/heic" || w.Header().Get("Content-Type") != "image/jpeg" {
				t.Fatal("HEIC did not produce a JPEG preview")
			}
			img, _, err := image.Decode(w.Body)
			if err != nil {
				t.Fatal(err)
			}
			if i == 0 && (p.Width != 2304 || img.Bounds().Dx() != 2048) {
				t.Fatalf("bad full-size metadata/preview: %+v %v", p, img.Bounds())
			}
		} else if !bytes.Equal(w.Body.Bytes(), originals[i]) {
			t.Fatal("PNG preview must use the original")
		}
	}
	stats := request(a.Handler(), "GET", "/api/v1/status", nil, "")
	check(stats, 200)
	var usage libraryStatus
	if err := json.Unmarshal(stats.Body.Bytes(), &usage); err != nil {
		t.Fatal(err)
	}
	if usage.Photos != 3 || usage.PreviewBytes <= 0 {
		t.Fatalf("bad HEIC storage accounting: %+v", usage)
	}
	for _, body := range []string{`{"photoIds":[]}`, `{"photoIds":["bad"]}`} {
		check(request(a.Handler(), "POST", "/api/v1/photo-archives", strings.NewReader(body), "application/json"), 400)
	}
	check(request(a.Handler(), "POST", "/api/v1/photo-archives", strings.NewReader(`{"photoIds":["`+strings.Repeat("a", 64)+`"]}`), "application/json"), 404)
	tooMany := make([]string, 1001)
	for i := range tooMany {
		tooMany[i] = photos[0].ID
	}
	raw, _ := json.Marshal(map[string]any{"photoIds": tooMany})
	check(request(a.Handler(), "POST", "/api/v1/photo-archives", bytes.NewReader(raw), "application/json"), 400)
	create := func() string {
		t.Helper()
		raw, _ := json.Marshal(map[string]any{"photoIds": []string{photos[0].ID, photos[1].ID, photos[2].ID, photos[0].ID}})
		w := request(a.Handler(), "POST", "/api/v1/photo-archives", bytes.NewReader(raw), "application/json")
		check(w, 201)
		var result struct {
			URL    string `json:"url"`
			Photos int    `json:"photos"`
		}
		if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
			t.Fatal(err)
		}
		if result.Photos != 3 || w.Header().Get("Location") != result.URL {
			t.Fatal("archive did not deduplicate its selection")
		}
		return result.URL
	}
	verifyZIP := func(handler http.Handler, path string) {
		t.Helper()
		w := request(handler, "GET", path, nil, "")
		check(w, 200)
		if w.Header().Get("Content-Type") != "application/zip" {
			t.Fatal("wrong archive content type")
		}
		zr, err := zip.NewReader(bytes.NewReader(w.Body.Bytes()), int64(w.Body.Len()))
		if err != nil {
			t.Fatal(err)
		}
		if len(zr.File) != 3 {
			t.Fatal("wrong archive file count")
		}
		names, contents := map[string]bool{}, map[string]bool{}
		for _, entry := range zr.File {
			if names[entry.Name] || strings.ContainsAny(entry.Name, "/\\") {
				t.Fatalf("unsafe/duplicate ZIP name: %s", entry.Name)
			}
			names[entry.Name] = true
			r, err := entry.Open()
			if err != nil {
				t.Fatal(err)
			}
			data, err := io.ReadAll(r)
			r.Close()
			if err != nil {
				t.Fatal(err)
			}
			sum := sha256.Sum256(data)
			contents[hex.EncodeToString(sum[:])] = true
		}
		for _, p := range photos {
			if !contents[p.ID] {
				t.Fatal("ZIP did not preserve an original")
			}
		}
	}
	archive := create()
	verifyZIP(b.Handler(), archive)
	id := strings.TrimPrefix(archive, "/api/v1/photo-archives/")
	if _, err := a.db.Exec(ctx, "UPDATE lumio_photo_archives SET expires_at=now()-interval '1 minute' WHERE id=$1", id); err != nil {
		t.Fatal(err)
	}
	check(request(b.Handler(), "GET", archive, nil, ""), 404)
	archive = create()
	target := (&url.URL{Scheme: "file", Path: t.TempDir()}).String()
	if err := MigrateStorage(ctx, cfg.DatabaseURL, target); err != nil {
		t.Fatal(err)
	}
	cfg.StorageURL = target
	c := start(cfg)
	verifyZIP(c.Handler(), archive)
	w := request(c.Handler(), "GET", "/api/v1/photos/"+photos[0].ID+"/preview", nil, "")
	check(w, 200)
	if _, _, err := image.Decode(w.Body); err != nil {
		t.Fatal(err)
	}
	check(request(c.Handler(), "DELETE", "/api/v1/photos/"+photos[0].ID, nil, ""), 204)
	if _, err := c.collectOne(ctx); err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{photos[0].ID + "/original", photos[0].ID + "/thumb.jpg", photos[0].ID + "/preview.jpg"} {
		exists, err := c.bucket.Exists(ctx, key)
		if err != nil || exists {
			t.Fatalf("HEIC object was not deleted: %s %v", key, err)
		}
	}
	check(request(c.Handler(), "GET", archive, nil, ""), 409)
}
