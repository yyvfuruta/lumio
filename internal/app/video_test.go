package app

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"image/jpeg"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func videoFixture(t *testing.T, format string) []byte {
	t.Helper()
	for _, tool := range []string{"ffmpeg", "ffprobe"} {
		if _, err := exec.LookPath(tool); err != nil {
			t.Skip("video tests require FFmpeg and FFprobe")
		}
	}
	filename := filepath.Join(t.TempDir(), "sample."+format)
	args := []string{"-nostdin", "-v", "error", "-f", "lavfi", "-i", "testsrc2=size=160x90:rate=10", "-f", "lavfi", "-i", "sine=frequency=440:sample_rate=48000", "-t", "2", "-threads", "1"}
	if format == "mp4" {
		args = append(args, "-c:v", "libx264", "-pix_fmt", "yuv420p", "-c:a", "aac", "-movflags", "+faststart")
	} else {
		args = append(args, "-c:v", "libvpx-vp9", "-c:a", "libopus")
	}
	args = append(args, filename)
	if output, err := exec.Command("ffmpeg", args...).CombinedOutput(); err != nil {
		t.Fatalf("generate %s: %v: %s", format, err, output)
	}
	data, err := os.ReadFile(filename)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func TestPrepareVideo(t *testing.T) {
	for _, format := range []string{"mp4", "webm"} {
		t.Run(format, func(t *testing.T) {
			data := videoFixture(t, format)
			filename := filepath.Join(t.TempDir(), "upload-without-extension")
			if err := os.WriteFile(filename, data, 0600); err != nil {
				t.Fatal(err)
			}
			f, err := os.Open(filename)
			if err != nil {
				t.Fatal(err)
			}
			defer f.Close()
			media, err := prepareMedia(context.Background(), f)
			if err != nil {
				t.Fatal(err)
			}
			if media.typ != "video/"+format || media.width != 160 || media.height != 90 || media.duration < 1.9 || media.duration > 2.1 {
				t.Fatalf("unexpected metadata: %+v", media)
			}
			thumb, err := jpeg.DecodeConfig(bytes.NewReader(media.thumb))
			if err != nil || thumb.Width != 640 || thumb.Height != 360 {
				t.Fatalf("bad thumbnail: %+v %v", thumb, err)
			}
			if len(media.preview) != 0 {
				t.Fatal("video should preserve and play original bytes")
			}
			if err := os.WriteFile(filename, data[:32], 0600); err != nil {
				t.Fatal(err)
			}
			if _, err := prepareVideo(context.Background(), filename, "video/"+format); !errors.Is(err, errImage) {
				t.Fatalf("corrupt video accepted: %v", err)
			}
		})
	}
}

func TestRejectUnsupportedVideoCodec(t *testing.T) {
	videoFixture(t, "mp4")
	filename := filepath.Join(t.TempDir(), "unsupported.mp4")
	output, err := exec.Command("ffmpeg", "-nostdin", "-v", "error", "-f", "lavfi", "-i", "color=size=32x32:rate=1", "-t", "1", "-threads", "1", "-c:v", "mpeg4", filename).CombinedOutput()
	if err != nil {
		t.Fatalf("fixture: %v %s", err, output)
	}
	if _, err := prepareVideo(context.Background(), filename, "video/mp4"); !errors.Is(err, errImage) {
		t.Fatalf("unsupported codec accepted: %v", err)
	}
}

func TestVideoIntegration(t *testing.T) {
	databaseURL := env("LUMIO_TEST_DATABASE_URL", "")
	if databaseURL == "" {
		t.Skip("set LUMIO_TEST_DATABASE_URL for integration tests")
	}
	backends := map[string]string{"file": (&url.URL{Scheme: "file", Path: t.TempDir()}).String()}
	if raw := env("LUMIO_TEST_STORAGE_URL", ""); raw != "" {
		backends["remote"] = raw
	}
	for backend, storageURL := range backends {
		t.Run(backend, func(t *testing.T) {
			cfg := featureConfig(t, databaseURL, storageURL)
			a, err := New(cfg)
			if err != nil {
				t.Fatal(err)
			}
			defer a.Close()
			var cookie *http.Cookie
			request := func(method, path string, body io.Reader, headers map[string]string) *httptest.ResponseRecorder {
				r := httptest.NewRequest(method, "http://photos.test"+path, body)
				r.Header.Set("X-Lumio-Request", "1")
				for key, value := range headers {
					r.Header.Set(key, value)
				}
				if cookie != nil {
					r.AddCookie(cookie)
				}
				w := httptest.NewRecorder()
				a.Handler().ServeHTTP(w, r)
				return w
			}
			check := func(w *httptest.ResponseRecorder, code int) {
				t.Helper()
				if w.Code != code {
					t.Fatalf("got %d want %d: %s", w.Code, code, w.Body.String())
				}
			}
			login := request("POST", "/api/v1/sessions", strings.NewReader(`{"password":"feature-test-password"}`), map[string]string{"Content-Type": "application/json"})
			check(login, 201)
			cookie = login.Result().Cookies()[0]
			for _, format := range []string{"mp4", "webm"} {
				data := videoFixture(t, format)
				upload := func() *httptest.ResponseRecorder {
					var body bytes.Buffer
					writer := multipart.NewWriter(&body)
					part, err := writer.CreateFormFile("photo", "sample."+format)
					if err != nil {
						t.Fatal(err)
					}
					part.Write(data)
					writer.Close()
					return request("POST", "/api/v1/photos", &body, map[string]string{"Content-Type": writer.FormDataContentType()})
				}
				w := upload()
				check(w, 201)
				var result struct {
					Photo     Photo `json:"photo"`
					Duplicate bool  `json:"duplicate"`
				}
				if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
					t.Fatal(err)
				}
				p := result.Photo
				if p.Type != "video/"+format || p.Duration <= 0 {
					t.Fatalf("missing video metadata: %+v", p)
				}
				duplicate := upload()
				check(duplicate, 200)
				if !strings.Contains(duplicate.Body.String(), `"duplicate":true`) {
					t.Fatal("video was not deduplicated")
				}
				path := "/api/v1/photos/" + p.ID
				w = request("GET", path+"/thumbnail", nil, nil)
				check(w, 200)
				if _, err := jpeg.DecodeConfig(w.Body); err != nil {
					t.Fatal(err)
				}
				for _, kind := range []string{"original", "preview"} {
					w = request("GET", path+"/"+kind, nil, nil)
					check(w, 200)
					if !bytes.Equal(w.Body.Bytes(), data) || w.Header().Get("Content-Type") != p.Type {
						t.Fatal("original bytes or content type changed")
					}
					for _, tc := range []struct {
						header     string
						start, end int
					}{{"bytes=16-79", 16, 80}, {"bytes=-32", len(data) - 32, len(data)}, {"bytes=32-", 32, len(data)}} {
						w = request("GET", path+"/"+kind, nil, map[string]string{"Range": tc.header})
						check(w, 206)
						if !bytes.Equal(w.Body.Bytes(), data[tc.start:tc.end]) || w.Header().Get("Content-Range") == "" {
							t.Fatalf("wrong range: %s", tc.header)
						}
					}
					w = request("HEAD", path+"/"+kind, nil, map[string]string{"Range": "bytes=0-15"})
					check(w, 206)
					if w.Body.Len() != 0 || w.Header().Get("Content-Length") != "16" {
						t.Fatal("HEAD returned content or wrong length")
					}
					check(request("GET", path+"/"+kind, nil, map[string]string{"Range": "bytes=999999999-"}), 416)
				}
				w = request("PATCH", path, strings.NewReader(`{"favorite":true}`), map[string]string{"Content-Type": "application/json"})
				check(w, 200)
				w = request("GET", "/api/v1/photos?favorite=true", nil, nil)
				check(w, 200)
				if !strings.Contains(w.Body.String(), p.ID) {
					t.Fatal("video missing from favorites")
				}
				archive := request("POST", "/api/v1/photo-archives", strings.NewReader(`{"photoIds":["`+p.ID+`"]}`), map[string]string{"Content-Type": "application/json"})
				check(archive, 201)
				var location struct {
					URL string `json:"url"`
				}
				json.Unmarshal(archive.Body.Bytes(), &location)
				w = request("GET", location.URL, nil, nil)
				check(w, 200)
				zr, err := zip.NewReader(bytes.NewReader(w.Body.Bytes()), int64(w.Body.Len()))
				if err != nil {
					t.Fatal(err)
				}
				item, err := zr.File[0].Open()
				if err != nil {
					t.Fatal(err)
				}
				archived, err := io.ReadAll(item)
				item.Close()
				if err != nil || !bytes.Equal(archived, data) {
					t.Fatal("archive changed video")
				}
				check(request("DELETE", path, nil, nil), 204)
				check(request("GET", path+"/original", nil, nil), 404)
			}
		})
	}
}

func TestVideoToolLimits(t *testing.T) {
	data := videoFixture(t, "mp4")
	filename := filepath.Join(t.TempDir(), "sample.mp4")
	if err := os.WriteFile(filename, data, 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := videoCommand(context.Background(), "ffprobe", 8, "-v", "error", "-show_format", filename); !errors.Is(err, errImage) {
		t.Fatalf("output limit not enforced: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := prepareVideo(ctx, filename, "video/mp4"); !errors.Is(err, context.Canceled) {
		t.Fatalf("ignored cancellation: %v", err)
	}
	t.Setenv("PATH", t.TempDir())
	if _, err := prepareVideo(context.Background(), filename, "video/mp4"); !errors.Is(err, errVideoTools) {
		t.Fatalf("missing video tools: %v", err)
	}
}
