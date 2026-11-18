package app

import (
	"errors"
	"net/url"
	"os"
	"path/filepath"
	"strconv"

	"github.com/yyvfuruta/lumio/internal/adapters/objectstore"
)

type Config struct {
	SkipMigrations                          bool
	Maintenance                             bool
	Addr, DatabaseURL, StorageURL, Password string
	SecureCookie                            bool
	MaxUploadBytes                          int64
}

func ConfigFromEnv() (Config, error) {
	c := Config{Addr: env("LUMIO_ADDR", ":8080"), DatabaseURL: os.Getenv("DATABASE_URL"), StorageURL: os.Getenv("LUMIO_STORAGE_URL"), Password: os.Getenv("LUMIO_PASSWORD")}
	if c.DatabaseURL == "" {
		return c, errors.New("DATABASE_URL is required (PostgreSQL)")
	}
	if len(c.Password) < 16 {
		return c, errors.New("LUMIO_PASSWORD must contain at least 16 characters")
	}
	if c.StorageURL == "" {
		root, err := filepath.Abs(env("LUMIO_DATA_DIR", "data"))
		if err != nil {
			return c, err
		}
		c.StorageURL = (&url.URL{Scheme: "file", Path: root}).String()
	}
	if err := validateStorageURL(c.StorageURL); err != nil {
		return c, err
	}
	var err error
	c.SecureCookie, err = strconv.ParseBool(env("LUMIO_SECURE_COOKIE", "true"))
	if err != nil {
		return c, errors.New("LUMIO_SECURE_COOKIE must be true or false")
	}
	mb, err := strconv.ParseInt(env("LUMIO_MAX_UPLOAD_MB", "32"), 10, 64)
	if err != nil || mb < 1 || mb > 256 {
		return c, errors.New("LUMIO_MAX_UPLOAD_MB must be between 1 and 256")
	}
	c.Maintenance, err = strconv.ParseBool(env("LUMIO_MAINTENANCE", "true"))
	if err != nil {
		return c, errors.New("LUMIO_MAINTENANCE must be true or false")
	}
	autoMigrate, err := strconv.ParseBool(env("LUMIO_AUTO_MIGRATE", "true"))
	if err != nil {
		return c, errors.New("LUMIO_AUTO_MIGRATE must be true or false")
	}
	c.SkipMigrations = !autoMigrate
	c.MaxUploadBytes = mb << 20
	return c, nil
}

func validateStorageURL(raw string) error { return objectstore.ValidateURL(raw) }

func env(name, fallback string) string {
	if s := os.Getenv(name); s != "" {
		return s
	}
	return fallback
}
