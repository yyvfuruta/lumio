package app

import (
	"context"
	"net/url"
	"os"

	"gocloud.dev/blob"
	_ "gocloud.dev/blob/azureblob"
	_ "gocloud.dev/blob/fileblob"
	_ "gocloud.dev/blob/gcsblob"
	_ "gocloud.dev/blob/s3blob"
)

func openBucket(ctx context.Context, rawURL string) (*blob.Bucket, error) {
	u, err := url.Parse(rawURL)
	if err != nil {
		return nil, err
	}
	if u.Scheme == "file" {
		if err := os.MkdirAll(u.Path, 0700); err != nil {
			return nil, err
		}
		// Atomic rename requires temporary and final objects on the same mount.
		q := u.Query()
		q.Set("no_tmp_dir", "true")
		u.RawQuery = q.Encode()
		rawURL = u.String()
	}
	return blob.OpenBucket(ctx, rawURL)
}
