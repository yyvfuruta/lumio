package httpapi

import (
	"context"
	"log/slog"
	"net/http"
	"net/url"
	"time"

	"github.com/yyvfuruta/lumio/internal/core"
)

type DiskUsage struct {
	Total     uint64 `json:"total"`
	Used      uint64 `json:"used"`
	Available uint64 `json:"available"`
}

type libraryStatus struct {
	core.Stats
	Storage           string     `json:"storage"`
	StorageAccessible bool       `json:"storageAccessible"`
	Database          string     `json:"database"`
	Disk              *DiskUsage `json:"disk"`
	CheckedAt         time.Time  `json:"checkedAt"`
}

func (a *API) status(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()
	result := libraryStatus{Database: "available", CheckedAt: time.Now().UTC()}
	var err error
	result.Stats, err = a.library.Stats(ctx)
	if err != nil {
		internal(w, err)
		return
	}

	u, _ := url.Parse(a.cfg.StorageURL)
	result.Storage = map[string]string{"file": "local volume", "s3": "S3-compatible bucket", "gs": "Google Cloud Storage", "azblob": "Azure Blob Storage"}[u.Scheme]
	result.StorageAccessible, err = a.library.Accessible(ctx)
	if err != nil {
		slog.Warn("storage status check failed", "error", err)
	}
	if u.Scheme == "file" {
		result.Disk, err = diskUsage(u.Path)
		if err != nil {
			slog.Warn("disk usage check failed", "error", err)
		}
	}
	jsonResponse(w, 200, result)
}
