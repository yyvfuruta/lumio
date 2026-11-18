package media

import (
	"image"
	"testing"
)

func TestThumbnail(t *testing.T) {
	src := image.NewNRGBA(image.Rect(4, 5, 2004, 1005))
	thumb := thumbnail(src)
	if thumb.Bounds().Dx() != 640 || thumb.Bounds().Dy() != 320 {
		t.Fatal(thumb.Bounds())
	}
	r, g, b, _ := thumb.At(0, 0).RGBA()
	if r != 65535 || g != 65535 || b != 65535 {
		t.Fatal("transparent pixels should be white")
	}
}
