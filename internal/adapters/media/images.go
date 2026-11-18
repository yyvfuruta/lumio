// Package media implements the core media processor using image decoders and FFmpeg.
package media

import (
	"bytes"
	"context"
	"image"
	"image/color"
	_ "image/gif"
	"image/jpeg"
	_ "image/png"
	"io"
	"net/http"
	"os"

	"github.com/gen2brain/h265/heic"
	"github.com/yyvfuruta/lumio/internal/core"
)

var errImage = core.ErrMedia

const maxImagePixels = 40_000_000

// Processor validates media and prepares derived images without changing originals.
type Processor struct{}

var _ core.Processor = Processor{}

func (Processor) Prepare(ctx context.Context, path string) (core.Prepared, error) {
	f, err := os.Open(path)
	if err != nil {
		return core.Prepared{}, err
	}
	defer f.Close()
	m, err := prepareMedia(ctx, f)
	if err != nil {
		return core.Prepared{}, err
	}
	return core.Prepared{Type: m.typ, Width: m.width, Height: m.height, Duration: m.duration, Thumbnail: m.thumb, Preview: m.preview}, nil
}
func thumbnail(src image.Image) image.Image {
	return resizeImage(src, 640)
}

func resizeImage(src image.Image, limit int) image.Image {
	b := src.Bounds()
	w, h := b.Dx(), b.Dy()
	tw, th := w, h
	if w > limit || h > limit {
		if w >= h {
			tw, th = limit, max(1, h*limit/w)
		} else {
			th, tw = limit, max(1, w*limit/h)
		}
	}
	dst := image.NewRGBA(image.Rect(0, 0, tw, th))
	for y := 0; y < th; y++ {
		for x := 0; x < tw; x++ {
			x0, x1, y0, y1 := x*w/tw, (x+1)*w/tw, y*h/th, (y+1)*h/th
			var r, g, bl uint64
			for sy := y0; sy < y1; sy++ {
				for sx := x0; sx < x1; sx++ {
					cr, cg, cb, ca := src.At(b.Min.X+sx, b.Min.Y+sy).RGBA()
					r += uint64(cr + 65535 - ca)
					g += uint64(cg + 65535 - ca)
					bl += uint64(cb + 65535 - ca)
				}
			}
			n := uint64((x1 - x0) * (y1 - y0))
			dst.SetRGBA(x, y, color.RGBA{uint8(r / n >> 8), uint8(g / n >> 8), uint8(bl / n >> 8), 255})
		}
	}
	return dst
}

type preparedMedia struct {
	typ            string
	width, height  int
	duration       float64
	thumb, preview []byte
}

func prepareMedia(ctx context.Context, f *os.File) (preparedMedia, error) {
	var header [512]byte
	n, err := f.ReadAt(header[:], 0)
	if err != nil && err != io.EOF {
		return preparedMedia{}, err
	}
	typ := http.DetectContentType(header[:n])
	if typ == "video/mp4" || typ == "video/webm" {
		return prepareVideo(ctx, f.Name(), typ)
	}
	cfg, format, err := image.DecodeConfig(f)
	if err != nil || cfg.Width <= 0 || cfg.Height <= 0 || cfg.Width > maxImagePixels/cfg.Height {
		return preparedMedia{}, errImage
	}
	mime := map[string]string{"jpeg": "image/jpeg", "png": "image/png", "gif": "image/gif", "heic": "image/heic"}[format]
	if mime == "" {
		return preparedMedia{}, errImage
	}
	if _, err := f.Seek(0, 0); err != nil {
		return preparedMedia{}, err
	}
	var img image.Image
	if format == "heic" {
		img, err = heic.Decode(f, heic.Options{AutoRotate: true, FrameSizeLimit: maxImagePixels, Threads: 2})
	} else {
		img, _, err = image.Decode(f)
	}
	if err != nil {
		return preparedMedia{}, errImage
	}
	cfg.Width, cfg.Height = img.Bounds().Dx(), img.Bounds().Dy()
	if cfg.Width <= 0 || cfg.Height <= 0 || cfg.Width > maxImagePixels/cfg.Height {
		return preparedMedia{}, errImage
	}
	var preview bytes.Buffer
	if format == "heic" {
		img = resizeImage(img, 2048)
		if err := jpeg.Encode(&preview, img, &jpeg.Options{Quality: 90}); err != nil {
			return preparedMedia{}, err
		}
	}
	var thumb bytes.Buffer
	if err := jpeg.Encode(&thumb, thumbnail(img), &jpeg.Options{Quality: 82}); err != nil {
		return preparedMedia{}, err
	}
	return preparedMedia{typ: mime, width: cfg.Width, height: cfg.Height, thumb: thumb.Bytes(), preview: preview.Bytes()}, nil
}
