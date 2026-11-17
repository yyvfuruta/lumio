package app

import (
	"context"
	"log/slog"
	"net/http"
	"net/url"
	"time"
)

type DiskUsage struct {
	Total     uint64 `json:"total"`
	Used      uint64 `json:"used"`
	Available uint64 `json:"available"`
}

type libraryStatus struct {
	Photos            int64      `json:"photos"`
	Favorites         int64      `json:"favorites"`
	OriginalBytes     int64      `json:"originalBytes"`
	ThumbnailBytes    int64      `json:"thumbnailBytes"`
	PreviewBytes      int64      `json:"previewBytes"`
	PendingDeletions  int64      `json:"pendingDeletions"`
	Storage           string     `json:"storage"`
	StorageAccessible bool       `json:"storageAccessible"`
	Database          string     `json:"database"`
	Disk              *DiskUsage `json:"disk"`
	CheckedAt         time.Time  `json:"checkedAt"`
}

func (a *App) status(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()
	result := libraryStatus{Database: "available", CheckedAt: time.Now().UTC()}
	err := a.db.QueryRow(ctx, `SELECT count(*) FILTER(WHERE deleted_at IS NULL),
	 count(*) FILTER(WHERE deleted_at IS NULL AND favorite),
	 coalesce(sum(size) FILTER(WHERE deleted_at IS NULL),0)::bigint,
	 coalesce(sum(thumb_size) FILTER(WHERE deleted_at IS NULL),0)::bigint,
	 coalesce(sum(preview_size) FILTER(WHERE deleted_at IS NULL),0)::bigint,
	 count(*) FILTER(WHERE deleted_at IS NOT NULL) FROM lumio_photos`).Scan(&result.Photos, &result.Favorites, &result.OriginalBytes, &result.ThumbnailBytes, &result.PreviewBytes, &result.PendingDeletions)
	if err != nil {
		internal(w, err)
		return
	}
	u, _ := url.Parse(a.cfg.StorageURL)
	result.Storage = map[string]string{"file": "local volume", "s3": "S3-compatible bucket", "gs": "Google Cloud Storage", "azblob": "Azure Blob Storage"}[u.Scheme]
	result.StorageAccessible, err = a.bucket.IsAccessible(ctx)
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
