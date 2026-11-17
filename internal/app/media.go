package app

import (
	"context"
	"errors"
	"io"

	"gocloud.dev/blob"
)

// A seek opens a new ranged blob reader instead of buffering the original.
// ServeContent can therefore serve video byte ranges for every storage backend.
type blobReadSeeker struct {
	ctx            context.Context
	bucket         *blob.Bucket
	key            string
	size, position int64
	reader         *blob.Reader
}

func (r *blobReadSeeker) Read(p []byte) (int, error) {
	if r.position >= r.size {
		return 0, io.EOF
	}
	if r.reader == nil {
		var err error
		r.reader, err = r.bucket.NewRangeReader(r.ctx, r.key, r.position, -1, nil)
		if err != nil {
			return 0, err
		}
	}
	n, err := r.reader.Read(p)
	r.position += int64(n)
	return n, err
}
func (r *blobReadSeeker) Seek(offset int64, whence int) (int64, error) {
	var base int64
	switch whence {
	case io.SeekStart:
	case io.SeekCurrent:
		base = r.position
	case io.SeekEnd:
		base = r.size
	default:
		return 0, errors.New("invalid seek origin")
	}
	next := base + offset
	if next < 0 || offset > 0 && next < base || offset < 0 && next > base {
		return 0, errors.New("invalid seek offset")
	}
	if next != r.position {
		if err := r.Close(); err != nil {
			return 0, err
		}
		r.position = next
	}
	return r.position, nil
}
func (r *blobReadSeeker) Close() error {
	if r.reader == nil {
		return nil
	}
	err := r.reader.Close()
	r.reader = nil
	return err
}
