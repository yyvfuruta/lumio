package core

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
)

var ErrMedia = errors.New("use a valid JPEG, PNG, GIF, or HEIC image up to 40 megapixels, or an MP4 (H.264/AAC) or WebM (VP8/VP9/AV1 with Opus/Vorbis) video")
var ErrTooLarge = errors.New("file exceeds the upload limit")
var ErrVideoTools = errors.New("FFmpeg and FFprobe are required for video uploads")

type Prepared struct {
	Type               string
	Width, Height      int
	Duration           float64
	Thumbnail, Preview []byte
}

// Processor receives the path of a private, bounded, temporary upload spool.
// Files exist only for the duration of this use case and never across requests.
type Processor interface {
	Prepare(context.Context, string) (Prepared, error)
}

func (l *Library) Upload(ctx context.Context, name string, src io.Reader, maxBytes int64) (Photo, bool, error) {
	f, err := os.CreateTemp("", "lumio-upload-*")
	if err != nil {
		return Photo{}, false, err
	}
	defer os.Remove(f.Name())
	defer f.Close()
	hash := sha256.New()
	n, err := io.Copy(io.MultiWriter(f, hash), io.LimitReader(src, maxBytes+1))
	if err != nil {
		return Photo{}, false, err
	}
	if n > maxBytes {
		return Photo{}, false, ErrTooLarge
	}
	if _, err := f.Seek(0, 0); err != nil {
		return Photo{}, false, err
	}
	media, err := l.processor.Prepare(ctx, f.Name())
	if err != nil {
		return Photo{}, false, err
	}
	thumb, preview := *bytes.NewBuffer(media.Thumbnail), *bytes.NewBuffer(media.Preview)
	p := Photo{ID: hex.EncodeToString(hash.Sum(nil)), Name: filepath.Base(strings.ReplaceAll(name, "\\", "/")), Type: media.Type, Size: n, ThumbSize: int64(thumb.Len()), PreviewSize: int64(preview.Len()), Width: media.Width, Height: media.Height, Duration: media.Duration}
	if len(p.Name) > 255 {
		p.Name = "photo"
	}
	if _, err := f.Seek(0, 0); err != nil {
		return Photo{}, false, err
	}
	return l.Import(ctx, p, f, &thumb, &preview)
}
