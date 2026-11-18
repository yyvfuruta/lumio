// Package objectstore adapts Go Cloud buckets to the core object port.
package objectstore

import (
	"context"
	"io"

	"github.com/yyvfuruta/lumio/internal/core"
	"gocloud.dev/blob"
	"gocloud.dev/gcerrors"
)

type Store struct{ bucket *blob.Bucket }

func New(bucket *blob.Bucket) *Store { return &Store{bucket: bucket} }

var _ core.Objects = (*Store)(nil)

func (s *Store) Open(ctx context.Context, key string, offset int64) (io.ReadCloser, error) {
	return s.bucket.NewRangeReader(ctx, key, offset, -1, nil)
}
func (s *Store) Put(ctx context.Context, key, typ string, src io.Reader) error {
	return s.bucket.Upload(ctx, key, src, &blob.WriterOptions{ContentType: typ})
}
func (s *Store) Delete(ctx context.Context, key string) error {
	err := s.bucket.Delete(ctx, key)
	if gcerrors.Code(err) == gcerrors.NotFound {
		return nil
	}
	return err
}
func (s *Store) Accessible(ctx context.Context) (bool, error) { return s.bucket.IsAccessible(ctx) }
