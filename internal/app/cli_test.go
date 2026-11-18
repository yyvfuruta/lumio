package app

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/yyvfuruta/lumio/internal/cli"
)

func TestCLIIntegration(t *testing.T) {
	databaseURL := os.Getenv("LUMIO_TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("set LUMIO_TEST_DATABASE_URL")
	}
	cfg := featureConfig(t, databaseURL, (&url.URL{Scheme: "file", Path: t.TempDir()}).String())
	app, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer app.Close()
	// A runtime process must reopen an initialized schema without DDL.
	cfg.SkipMigrations = true
	replica, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer replica.Close()
	server := httptest.NewServer(replica.Handler())
	defer server.Close()
	dir := t.TempDir()
	config := filepath.Join(dir, "cli.json")
	run := func(args ...string) string {
		t.Helper()
		var out bytes.Buffer
		err := cli.Run(context.Background(), append([]string{"--config", config, "--url", server.URL}, args...), strings.NewReader(cfg.Password+"\n"), &out, io.Discard)
		if err != nil {
			t.Fatal(err)
		}
		return out.String()
	}
	run("login", "--password-stdin")
	file := filepath.Join(dir, "phone.png")
	original := testPNG(t)
	if err := os.WriteFile(file, original, 0600); err != nil {
		t.Fatal(err)
	}
	var upload struct {
		Photo     Photo `json:"photo"`
		Duplicate bool  `json:"duplicate"`
	}
	if err := json.Unmarshal([]byte(run("upload", file)), &upload); err != nil {
		t.Fatal(err)
	}
	if !validID(upload.Photo.ID) || upload.Duplicate {
		t.Fatal("invalid upload")
	}
	if !strings.Contains(run("upload", file), `"duplicate":true`) {
		t.Fatal("duplicate not recognized")
	}
	id := upload.Photo.ID
	run("favorite", id)
	if !strings.Contains(run("--json", "list", "--all", "--favorite", "--query", "phone"), id) {
		t.Fatal("list lost uploaded photo")
	}
	downloaded := filepath.Join(dir, "original.png")
	run("download", "--output", downloaded, id)
	data, _ := os.ReadFile(downloaded)
	if !bytes.Equal(data, original) {
		t.Fatal("original changed")
	}
	run("status")
	run("show", id)
	run("delete", "--yes", id)
	run("logout")
	err = cli.Run(context.Background(), []string{"--config", config, "--url", server.URL, "status"}, nil, io.Discard, io.Discard)
	if err == nil || !strings.Contains(err.Error(), "401") {
		t.Fatal("logout left client authenticated", err)
	}
}
