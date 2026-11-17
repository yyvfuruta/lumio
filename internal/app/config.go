package app

import (
	"errors"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
)

type Config struct {
	Addr, DatabaseURL, StorageURL, Password string
	SecureCookie                            bool
	MaxUploadBytes                          int64
}

func ConfigFromEnv() (Config, error) {
	c := Config{Addr: env("LUMIO_ADDR", ":8080"), DatabaseURL: os.Getenv("DATABASE_URL"), StorageURL: os.Getenv("LUMIO_STORAGE_URL"), Password: os.Getenv("LUMIO_PASSWORD")}
	if c.DatabaseURL == "" {
		return c, errors.New("DATABASE_URL é obrigatória (PostgreSQL)")
	}
	if len(c.Password) < 16 {
		return c, errors.New("LUMIO_PASSWORD deve ter ao menos 16 caracteres")
	}
	if c.StorageURL == "" {
		root, err := filepath.Abs(env("LUMIO_DATA_DIR", "data"))
		if err != nil {
			return c, err
		}
		c.StorageURL = (&url.URL{Scheme: "file", Path: root}).String()
	}
	u, err := url.Parse(c.StorageURL)
	if err != nil {
		return c, errors.New("LUMIO_STORAGE_URL inválida")
	}
	if u.Scheme != "file" && u.Scheme != "s3" && u.Scheme != "gs" && u.Scheme != "azblob" {
		return c, errors.New("use file://, s3://, gs:// ou azblob:// em LUMIO_STORAGE_URL")
	}
	if u.Query().Get("key") != "" {
		return c, errors.New("storage com chave única não é suportado")
	}
	c.SecureCookie, err = strconv.ParseBool(env("LUMIO_SECURE_COOKIE", "true"))
	if err != nil {
		return c, errors.New("LUMIO_SECURE_COOKIE deve ser true ou false")
	}
	mb, err := strconv.ParseInt(env("LUMIO_MAX_UPLOAD_MB", "32"), 10, 64)
	if err != nil || mb < 1 || mb > 256 {
		return c, errors.New("LUMIO_MAX_UPLOAD_MB deve estar entre 1 e 256")
	}
	c.MaxUploadBytes = mb << 20
	return c, nil
}

func env(name, fallback string) string {
	if s := os.Getenv(name); s != "" {
		return s
	}
	return fallback
}
