package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestClientWorkflow(t *testing.T) {
	id := strings.Repeat("a", 64)
	var uploads, lists int
	var favored, deleted, loggedOut bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "GET" && r.Header.Get("X-Lumio-Request") != "1" {
			t.Error("missing mutation header")
		}
		if r.URL.Path == "/api/v1/sessions" {
			var body map[string]string
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Error(err)
			}
			if body["password"] != "test-password" {
				t.Error("wrong password")
			}
			http.SetCookie(w, &http.Cookie{Name: "lumio_session", Value: "secret", Path: "/", Expires: time.Now().Add(time.Hour)})
			w.WriteHeader(201)
			fmt.Fprint(w, `{"authenticated":true}`)
			return
		}
		cookie, err := r.Cookie("lumio_session")
		if err != nil || cookie.Value != "secret" {
			http.Error(w, `{"error":"unauthorized"}`, 401)
			return
		}
		switch {
		case r.URL.Path == "/api/v1/photos" && r.Method == "POST":
			reader, err := r.MultipartReader()
			if err != nil {
				t.Error(err)
				return
			}
			part, err := reader.NextPart()
			if err != nil {
				t.Error(err)
				return
			}
			b, _ := io.ReadAll(part)
			if part.FormName() != "photo" || part.FileName() != "test.jpg" || string(b) != "original" {
				t.Error("incorrect multipart upload")
			}
			uploads++
			fmt.Fprintf(w, `{"photo":{"id":%q},"duplicate":false}`, id)
		case r.URL.Path == "/api/v1/photos":
			lists++
			if r.URL.Query().Get("q") != "my photo" || r.URL.Query().Get("favorite") != "true" {
				t.Error("lost filters")
			}
			next := "next"
			if r.URL.Query().Get("cursor") == "next" {
				next = ""
			}
			fmt.Fprintf(w, `{"photos":[{"id":%q,"name":"test.jpg","size":8}],"nextCursor":%q}`, id, next)
		case r.URL.Path == "/api/v1/photos/"+id+"/original":
			fmt.Fprint(w, "original")
		case r.Method == "PATCH":
			var body map[string]bool
			json.NewDecoder(r.Body).Decode(&body)
			favored = body["favorite"]
			fmt.Fprint(w, `{"favorite":true}`)
		case r.Method == "DELETE" && r.URL.Path == "/api/v1/sessions/current":
			loggedOut = true
			w.WriteHeader(204)
		case r.Method == "DELETE":
			deleted = true
			w.WriteHeader(204)
		default:
			fmt.Fprint(w, `{"photos":2}`)
		}
	}))
	defer server.Close()
	dir := t.TempDir()
	config := filepath.Join(dir, "session.json")
	run := func(args ...string) string {
		t.Helper()
		var out bytes.Buffer
		err := Run(context.Background(), append([]string{"--url", server.URL, "--config", config}, args...), strings.NewReader("test-password\n"), &out, io.Discard)
		if err != nil {
			t.Fatal(err)
		}
		return out.String()
	}
	run("login", "--password-stdin")
	data, err := os.ReadFile(config)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(data, []byte("test-password")) {
		t.Fatal("password persisted")
	}
	info, _ := os.Stat(config)
	if info.Mode().Perm()&0077 != 0 {
		t.Fatal("session is readable by other users")
	}
	os.WriteFile(filepath.Join(dir, "test.jpg"), []byte("original"), 0600)
	run("upload", filepath.Join(dir, "test.jpg"))
	output := run("--json", "list", "--all", "--query", "my photo", "--favorite")
	if strings.Count(output, "test.jpg") != 2 || lists != 2 {
		t.Fatal("pagination failed", output)
	}
	run("favorite", id)
	run("status")
	run("show", id)
	download := filepath.Join(dir, "photo.jpg")
	run("download", "--output", download, id)
	data, _ = os.ReadFile(download)
	if string(data) != "original" {
		t.Fatal("download changed bytes")
	}
	err = Run(context.Background(), []string{"--url", server.URL, "--config", config, "download", "--output", download, id}, nil, io.Discard, io.Discard)
	if err == nil {
		t.Fatal("overwrote existing download")
	}
	err = Run(context.Background(), []string{"--url", server.URL, "--config", config, "delete", id}, nil, io.Discard, io.Discard)
	if err == nil || deleted {
		t.Fatal("deletion lacked confirmation")
	}
	run("delete", "--yes", id)
	run("logout")
	if uploads != 1 || !favored || !deleted || !loggedOut {
		t.Fatal("incomplete workflow")
	}
	if _, err = os.Stat(config); !os.IsNotExist(err) {
		t.Fatal("session remains after logout")
	}
}

func TestSessionIsolationAndErrors(t *testing.T) {
	for _, status := range []int{401, 413, 429, 503} {
		t.Run(fmt.Sprint(status), func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Header.Get("Cookie") != "" {
					t.Error("session leaked to a different server")
				}
				w.WriteHeader(status)
				fmt.Fprint(w, `{"error":"rejected"}`)
			}))
			defer server.Close()
			config := filepath.Join(t.TempDir(), "session.json")
			saveSession(config, session{URL: "https://other.example", Cookie: &http.Cookie{Name: "lumio_session", Value: "secret", Path: "/"}})
			err := Run(context.Background(), []string{"--url", server.URL, "--config", config, "status"}, nil, io.Discard, io.Discard)
			if err == nil || !strings.Contains(err.Error(), fmt.Sprint(status)) {
				t.Fatal("missing HTTP error", err)
			}
		})
	}
}

func TestInterruptedDownloadLeavesNoFile(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Length", "100")
		fmt.Fprint(w, "short")
	}))
	defer server.Close()
	dir := t.TempDir()
	output := filepath.Join(dir, "photo.jpg")
	err := Run(context.Background(), []string{"--url", server.URL, "--config", filepath.Join(dir, "session.json"), "download", "--output", output, strings.Repeat("a", 64)}, nil, io.Discard, io.Discard)
	if err == nil {
		t.Fatal("accepted interrupted download")
	}
	entries, _ := os.ReadDir(dir)
	if len(entries) != 0 {
		t.Fatal("partial download left behind")
	}
}

func TestUploadEarlyRejection(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(413)
		fmt.Fprint(w, `{"error":"too large"}`)
	}))
	defer server.Close()
	dir := t.TempDir()
	name := filepath.Join(dir, "large.jpg")
	os.WriteFile(name, bytes.Repeat([]byte("x"), 8<<20), 0600)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	err := Run(ctx, []string{"--url", server.URL, "--config", filepath.Join(dir, "session.json"), "upload", name}, nil, io.Discard, io.Discard)
	if err == nil || !strings.Contains(err.Error(), "413") {
		t.Fatal("missing upload rejection", err)
	}
}
