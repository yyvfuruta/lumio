package objectstore

import (
	"errors"
	"net/url"
	"path/filepath"
)

func ValidateURL(raw string) error {
	u, err := url.Parse(raw)
	if err != nil {
		return errors.New("invalid storage URL")
	}
	if u.Scheme != "file" && u.Scheme != "s3" && u.Scheme != "gs" && u.Scheme != "azblob" {
		return errors.New("storage must use file://, s3://, gs://, or azblob://")
	}
	if u.Query().Get("key") != "" {
		return errors.New("single-key storage is not supported")
	}
	if u.Scheme == "file" && (!filepath.IsAbs(u.Path) || u.Host != "") {
		return errors.New("local storage requires an absolute path: file:///directory")
	}
	if u.Scheme != "file" && u.Host == "" {
		return errors.New("a bucket name is required")
	}
	return nil
}
