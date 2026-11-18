package app

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/yyvfuruta/lumio/internal/adapters/maintenance"
	"gocloud.dev/blob"
	"gocloud.dev/blob/fileblob"
	"gocloud.dev/blob/memblob"
	"gocloud.dev/gcerrors"
)

// Decode the public status contract independently of the HTTP adapter's DTO.
type libraryStatus struct {
	Photos            int64 `json:"photos"`
	OriginalBytes     int64 `json:"originalBytes"`
	ThumbnailBytes    int64 `json:"thumbnailBytes"`
	PreviewBytes      int64 `json:"previewBytes"`
	StorageAccessible bool  `json:"storageAccessible"`
}

func testPNG(t *testing.T) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, 100, 70))
	for y := 0; y < 70; y++ {
		for x := 0; x < 100; x++ {
			img.Set(x, y, color.RGBA{uint8(x * 2), uint8(y * 3), 90, 255})
		}
	}
	var b bytes.Buffer
	if err := png.Encode(&b, img); err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}

func TestConfig(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://localhost/lumio")
	t.Setenv("LUMIO_PASSWORD", "a-long-test-password")
	t.Setenv("LUMIO_STORAGE_URL", "")
	c, err := ConfigFromEnv()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(c.StorageURL, "file:///") || !c.SecureCookie || c.MaxUploadBytes != 32<<20 {
		t.Fatalf("unexpected defaults: %+v", c)
	}
	for _, raw := range []string{"http://bucket", "mem://", "s3://test?key=everything"} {
		t.Setenv("LUMIO_STORAGE_URL", raw)
		if _, err := ConfigFromEnv(); err == nil {
			t.Fatalf("accepted %s", raw)
		}
	}
	t.Setenv("LUMIO_STORAGE_URL", "gs://photos")
	t.Setenv("LUMIO_MAX_UPLOAD_MB", "9999999999")
	if _, err := ConfigFromEnv(); err == nil {
		t.Fatal("accepted huge limit")
	}
}

// The same storage contract runs against memory, files, and optionally a live bucket.
func TestBucketContract(t *testing.T) {
	file, err := fileblob.OpenBucket(t.TempDir(), nil)
	if err != nil {
		t.Fatal(err)
	}
	buckets := map[string]*blob.Bucket{"memory": memblob.OpenBucket(nil), "file": file}
	if raw := os.Getenv("LUMIO_TEST_STORAGE_URL"); raw != "" {
		b, err := openBucket(context.Background(), raw)
		if err != nil {
			t.Fatal(err)
		}
		buckets["remote"] = b
	}
	for name, b := range buckets {
		t.Run(name, func(t *testing.T) {
			defer b.Close()
			ctx := context.Background()
			key := fmt.Sprintf("contract/%d", time.Now().UnixNano())
			want := []byte("lumio storage contract")
			if err := b.Upload(ctx, key, bytes.NewReader(want), &blob.WriterOptions{ContentType: "text/plain"}); err != nil {
				t.Fatal(err)
			}
			defer b.Delete(ctx, key)
			got, err := b.ReadAll(ctx, key)
			if err != nil || !bytes.Equal(got, want) {
				t.Fatalf("read: %q %v", got, err)
			}
			if err := b.Delete(ctx, key); err != nil {
				t.Fatal(err)
			}
			if _, err := b.ReadAll(ctx, key); gcerrors.Code(err) != gcerrors.NotFound {
				t.Fatalf("expected not found: %v", err)
			}
		})
	}
}

func TestIntegration(t *testing.T) {
	databaseURL := os.Getenv("LUMIO_TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("set LUMIO_TEST_DATABASE_URL to run PostgreSQL integration tests")
	}
	backends := map[string]string{"file": (&url.URL{Scheme: "file", Path: t.TempDir()}).String()}
	if raw := os.Getenv("LUMIO_TEST_STORAGE_URL"); raw != "" {
		backends["remote"] = raw
	}
	for backend, storageURL := range backends {
		t.Run(backend, func(t *testing.T) { testIntegration(t, databaseURL, storageURL) })
	}
}

func testIntegration(t *testing.T, databaseURL, storageURL string) {
	t.Helper()
	ctx := context.Background()
	admin, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	defer admin.Close()
	var random [8]byte
	if _, err := rand.Read(random[:]); err != nil {
		t.Fatal(err)
	}
	schemaName := "lumio_test_" + hex.EncodeToString(random[:])
	if _, err := admin.Exec(ctx, "CREATE SCHEMA "+schemaName); err != nil {
		t.Fatal(err)
	}
	defer admin.Exec(ctx, "DROP SCHEMA "+schemaName+" CASCADE")
	u, err := url.Parse(databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	q := u.Query()
	q.Set("search_path", schemaName)
	u.RawQuery = q.Encode()
	// Isolate cloud objects, too. Never touch unrelated objects in the test bucket.
	su, err := url.Parse(storageURL)
	if err != nil {
		t.Fatal(err)
	}
	sq := su.Query()
	sq.Set("prefix", sq.Get("prefix")+schemaName+"/")
	su.RawQuery = sq.Encode()
	cfg := Config{Addr: ":0", DatabaseURL: u.String(), StorageURL: su.String(), Password: "integration-test-password", MaxUploadBytes: 1 << 20}
	a, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer a.Close()
	b, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer b.Close()
	h1, h2 := a.Handler(), b.Handler()
	request := func(handler http.Handler, method, path string, body io.Reader, cookie *http.Cookie, contentType string) *httptest.ResponseRecorder {
		r := httptest.NewRequest(method, "http://photos.test"+path, body)
		r.Header.Set("X-Lumio-Request", "1")
		if contentType != "" {
			r.Header.Set("Content-Type", contentType)
		}
		if cookie != nil {
			r.AddCookie(cookie)
		}
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, r)
		return w
	}
	assertCode := func(w *httptest.ResponseRecorder, want int) {
		t.Helper()
		if w.Code != want {
			t.Fatalf("got %d want %d: %s", w.Code, want, w.Body.String())
		}
	}
	assertCode(request(h1, "GET", "/api/v1/photos", nil, nil, ""), 401)
	assertCode(request(h1, "POST", "/api/v1/sessions", strings.NewReader(`{"password":"wrong"}`), nil, "application/json"), 401)
	login := request(h1, "POST", "/api/v1/sessions", strings.NewReader(`{"password":"integration-test-password"}`), nil, "application/json")
	assertCode(login, 201)
	cookie := login.Result().Cookies()[0]
	if !cookie.HttpOnly || cookie.SameSite != http.SameSiteStrictMode {
		t.Fatal("unsafe cookie")
	}
	assertCode(request(h2, "GET", "/api/v1/photos", nil, cookie, ""), 200)
	assertCode(request(h1, "GET", "/api/v1/status", nil, nil, ""), 401)
	assertCode(request(h1, "GET", "/api/v1/photos/timeline", nil, nil, ""), 401)
	assertCode(request(h1, "GET", "/api/v1/photos?from=invalid", nil, cookie, ""), 400)
	assertCode(request(h1, "GET", "/api/v1/photos?from=2021-01-01T00:00:00Z&until=2020-01-01T00:00:00Z", nil, cookie, ""), 400)
	assertCode(request(h1, "GET", "/api/v1/photos/timeline?tz=not-a-zone", nil, cookie, ""), 400)
	csrf := httptest.NewRequest("POST", "http://photos.test/api/v1/photos", nil)
	csrf.AddCookie(cookie)
	w := httptest.NewRecorder()
	h1.ServeHTTP(w, csrf)
	assertCode(w, 403)
	pngData := testPNG(t)
	upload := func(handler http.Handler, data []byte) *httptest.ResponseRecorder {
		var body bytes.Buffer
		writer := multipart.NewWriter(&body)
		part, err := writer.CreateFormFile("photo", "memória.png")
		if err != nil {
			t.Fatal(err)
		}
		part.Write(data)
		writer.Close()
		return request(handler, "POST", "/api/v1/photos", &body, cookie, writer.FormDataContentType())
	}
	assertCode(upload(h1, []byte("not an image")), 415)
	assertCode(upload(h1, make([]byte, (1<<20)+1)), 413)
	w = upload(h1, pngData)
	assertCode(w, 201)
	var uploaded struct {
		Photo     Photo `json:"photo"`
		Duplicate bool  `json:"duplicate"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &uploaded); err != nil {
		t.Fatal(err)
	}
	p := uploaded.Photo
	if w.Header().Get("Location") != "/api/v1/photos/"+p.ID {
		t.Fatal("missing photo resource Location")
	}
	assertCode(request(h2, "GET", "/api/v1/photos/"+p.ID, nil, cookie, ""), 200)
	statusResponse := request(h2, "GET", "/api/v1/status", nil, cookie, "")
	assertCode(statusResponse, 200)
	var usage libraryStatus
	if err := json.Unmarshal(statusResponse.Body.Bytes(), &usage); err != nil {
		t.Fatal(err)
	}
	if usage.Photos != 1 || usage.OriginalBytes != int64(len(pngData)) || usage.ThumbnailBytes == 0 || !usage.StorageAccessible {
		t.Fatalf("incorrect library status: %+v", usage)
	}
	if p.Name != "memória.png" || p.Width != 100 || p.Height != 70 {
		t.Fatalf("unexpected metadata: %+v", p)
	}
	// Multiple replicas must deduplicate the same bytes and share the session.
	var wg sync.WaitGroup
	codes := make(chan int, 2)
	for _, handler := range []http.Handler{h1, h2} {
		wg.Add(1)
		go func() { defer wg.Done(); codes <- upload(handler, pngData).Code }()
	}
	wg.Wait()
	close(codes)
	for code := range codes {
		if code != 200 {
			t.Fatalf("concurrent dedup: %d", code)
		}
	}
	w = request(h2, "GET", "/api/v1/photos/"+p.ID+"/original", nil, cookie, "")
	assertCode(w, 200)
	if !bytes.Equal(w.Body.Bytes(), pngData) {
		t.Fatal("original changed")
	}
	assertCode(request(h2, "GET", "/api/v1/photos/"+p.ID+"/thumbnail", nil, nil, ""), 401)
	w = request(h2, "GET", "/api/v1/photos/"+p.ID+"/thumbnail", nil, cookie, "")
	assertCode(w, 200)
	if _, _, err := image.Decode(w.Body); err != nil {
		t.Fatal(err)
	}
	assertCode(request(h2, "PATCH", "/api/v1/photos/"+p.ID, strings.NewReader(`{"favorite":true}`), cookie, "application/json"), 200)
	w = request(h1, "GET", "/api/v1/photos?favorite=true&q=mem", nil, cookie, "")
	assertCode(w, 200)
	if !strings.Contains(w.Body.String(), p.ID) {
		t.Fatal("favorite/search missing")
	}
	// Seed metadata to exercise keyset pagination at equal timestamps.
	for i := 0; i < 65; i++ {
		id := sha256.Sum256([]byte(fmt.Sprint(i)))
		_, err := a.db.Exec(ctx, "INSERT INTO lumio_photos(id,name,content_type,size,thumb_size,width,height,added_at) VALUES($1,'page','image/png',1,1,1,1,'2020-01-01')", hex.EncodeToString(id[:]))
		if err != nil {
			t.Fatal(err)
		}
	}
	w = request(h1, "GET", "/api/v1/photos/timeline?tz=UTC", nil, cookie, "")
	assertCode(w, 200)
	var timeline struct {
		Months []timelineMonth `json:"months"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &timeline); err != nil {
		t.Fatal(err)
	}
	if len(timeline.Months) != 2 || timeline.Months[1].Month != "2020-01" || timeline.Months[1].Count != 65 {
		t.Fatalf("bad timeline: %+v", timeline)
	}
	w = request(h1, "GET", "/api/v1/photos/timeline?tz=America%2FSao_Paulo", nil, cookie, "")
	assertCode(w, 200)
	if err := json.Unmarshal(w.Body.Bytes(), &timeline); err != nil {
		t.Fatal(err)
	}
	if len(timeline.Months) != 2 || timeline.Months[1].Month != "2019-12" || timeline.Months[1].Count != 65 {
		t.Fatalf("timeline ignored local timezone: %+v", timeline)
	}
	w = request(h1, "GET", "/api/v1/photos?from=2020-01-01T00:00:00Z&until=2021-01-01T00:00:00Z", nil, cookie, "")
	assertCode(w, 200)
	var filtered struct {
		Photos []Photo `json:"photos"`
		Next   string  `json:"nextCursor"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &filtered); err != nil {
		t.Fatal(err)
	}
	if len(filtered.Photos) != 60 || filtered.Next == "" {
		t.Fatal("bad filtered page")
	}
	for _, photo := range filtered.Photos {
		if photo.ID == p.ID {
			t.Fatal("date range included an out-of-range photo")
		}
	}
	w = request(h2, "GET", "/api/v1/photos?from=2020-01-01T00:00:00Z&until=2021-01-01T00:00:00Z&cursor="+filtered.Next, nil, cookie, "")
	assertCode(w, 200)
	if err := json.Unmarshal(w.Body.Bytes(), &filtered); err != nil {
		t.Fatal(err)
	}
	if len(filtered.Photos) != 5 || filtered.Next != "" {
		t.Fatal("bad filtered second page")
	}
	var page struct {
		Photos []Photo `json:"photos"`
		Next   string  `json:"nextCursor"`
	}
	w = request(h1, "GET", "/api/v1/photos", nil, cookie, "")
	assertCode(w, 200)
	if err := json.Unmarshal(w.Body.Bytes(), &page); err != nil {
		t.Fatal(err)
	}
	if len(page.Photos) != 60 || page.Next == "" {
		t.Fatal("bad first page")
	}
	seen := map[string]bool{}
	for _, photo := range page.Photos {
		seen[photo.ID] = true
	}
	w = request(h2, "GET", "/api/v1/photos?cursor="+page.Next, nil, cookie, "")
	assertCode(w, 200)
	json.Unmarshal(w.Body.Bytes(), &page)
	if len(page.Photos) != 6 || page.Next != "" {
		t.Fatal("bad second page")
	}
	for _, photo := range page.Photos {
		if seen[photo.ID] {
			t.Fatal("pagination duplicated photo")
		}
	}
	assertCode(request(h1, "DELETE", "/api/v1/photos/"+p.ID, nil, cookie, ""), 204)
	assertCode(request(h2, "GET", "/api/v1/photos/"+p.ID+"/original", nil, cookie, ""), 404)
	if _, err := a.collectOne(ctx); err != nil {
		t.Fatal(err)
	}
	if exists, err := a.bucket.Exists(ctx, p.ID+"/original"); err != nil || exists {
		t.Fatalf("object not collected: %v", err)
	}
	assertCode(upload(h2, pngData), 201)
	// Restarting retains data. Logging out invalidates the session on every replica.
	c, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	assertCode(request(c.Handler(), "GET", "/api/v1/photos/"+p.ID+"/original", nil, cookie, ""), 200)
	c.Close()
	assertCode(request(h1, "DELETE", "/api/v1/sessions/current", nil, cookie, ""), 204)
	assertCode(request(h2, "GET", "/api/v1/photos", nil, cookie, ""), 401)
	// A PVC-to-bucket migration verifies copies, retains originals, and fences old replicas.
	if _, err := a.db.Exec(ctx, "DELETE FROM lumio_photos WHERE name='page'"); err != nil {
		t.Fatal(err)
	}
	targetURL := (&url.URL{Scheme: "file", Path: t.TempDir()}).String()
	if su.Scheme == "file" && os.Getenv("LUMIO_TEST_STORAGE_URL") != "" {
		tu, err := url.Parse(os.Getenv("LUMIO_TEST_STORAGE_URL"))
		if err != nil {
			t.Fatal(err)
		}
		tq := tu.Query()
		tq.Set("prefix", schemaName+"/migrated/")
		tu.RawQuery = tq.Encode()
		targetURL = tu.String()
	}
	if err := MigrateStorage(ctx, cfg.DatabaseURL, targetURL); err != nil {
		t.Fatal(err)
	}
	assertCode(request(h1, "GET", "/readyz", nil, nil, ""), 503)
	if exists, err := a.bucket.Exists(ctx, p.ID+"/original"); err != nil || !exists {
		t.Fatalf("migration removed source: %v", err)
	}
	if _, _, err := a.addPhoto(ctx, "stale.png", bytes.NewReader(pngData)); err == nil {
		t.Fatal("stale replica accepted an upload")
	}
	cfg.StorageURL = targetURL
	migrated, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer migrated.Close()
	assertCode(request(migrated.Handler(), "GET", "/readyz", nil, nil, ""), 200)
	got, err := migrated.bucket.ReadAll(ctx, p.ID+"/original")
	if err != nil || !bytes.Equal(got, pngData) {
		t.Fatalf("migration changed original: %v", err)
	}
	for _, key := range []string{p.ID + "/original", p.ID + "/thumb.jpg"} {
		if err := migrated.bucket.Delete(ctx, key); err != nil {
			t.Fatal(err)
		}
	}
	for _, key := range []string{p.ID + "/original", p.ID + "/thumb.jpg"} {
		if err := a.bucket.Delete(ctx, key); err != nil {
			t.Fatal(err)
		}
	}
}

func TestCopyVerified(t *testing.T) {
	ctx := context.Background()
	source, target := memblob.OpenBucket(nil), memblob.OpenBucket(nil)
	defer source.Close()
	defer target.Close()
	want := []byte("original bytes")
	sum := sha256.Sum256(want)
	if err := source.WriteAll(ctx, "original", want, nil); err != nil {
		t.Fatal(err)
	}
	for range 2 {
		if err := maintenance.CopyVerified(ctx, source, target, "original", "image/png", int64(len(want)), hex.EncodeToString(sum[:])); err != nil {
			t.Fatal(err)
		}
	}
	if err := target.WriteAll(ctx, "original", []byte("corruption"), nil); err != nil {
		t.Fatal(err)
	}
	if err := maintenance.CopyVerified(ctx, source, target, "original", "image/png", int64(len(want)), hex.EncodeToString(sum[:])); err == nil {
		t.Fatal("accepted corrupt destination")
	}
	if err := maintenance.CopyVerified(ctx, source, target, "original", "image/png", 99, ""); err == nil {
		t.Fatal("accepted wrong source size")
	}
}

func TestLocalBucketDoesNotStageOnAnotherVolume(t *testing.T) {
	root := t.TempDir()
	t.Setenv("TMPDIR", root+"/unavailable-temp-directory")
	b, err := openBucket(context.Background(), (&url.URL{Scheme: "file", Path: root}).String())
	if err != nil {
		t.Fatal(err)
	}
	defer b.Close()
	if err := b.Upload(context.Background(), "photo/original", strings.NewReader("original bytes"), &blob.WriterOptions{ContentType: "image/jpeg"}); err != nil {
		t.Fatal(err)
	}
}
