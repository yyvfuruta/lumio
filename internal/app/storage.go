package app

import (
	"context"

	"github.com/yyvfuruta/lumio/internal/adapters/objectstore"
	"gocloud.dev/blob"
)

func openBucket(ctx context.Context, raw string) (*blob.Bucket, error) {
	return objectstore.Open(ctx, raw)
}
