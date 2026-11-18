package media

import (
	"bytes"
	"context"
	"errors"
	"image/jpeg"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func videoFixture(t *testing.T, format string) []byte {
	t.Helper()
	for _, tool := range []string{"ffmpeg", "ffprobe"} {
		if _, err := exec.LookPath(tool); err != nil {
			t.Skip("video tests require FFmpeg and FFprobe")
		}
	}
	filename := filepath.Join(t.TempDir(), "sample."+format)
	args := []string{"-nostdin", "-v", "error", "-f", "lavfi", "-i", "testsrc2=size=160x90:rate=10", "-f", "lavfi", "-i", "sine=frequency=440:sample_rate=48000", "-t", "2", "-threads", "1"}
	if format == "mp4" {
		args = append(args, "-c:v", "libx264", "-pix_fmt", "yuv420p", "-c:a", "aac", "-movflags", "+faststart")
	} else {
		args = append(args, "-c:v", "libvpx-vp9", "-c:a", "libopus")
	}
	args = append(args, filename)
	if output, err := exec.Command("ffmpeg", args...).CombinedOutput(); err != nil {
		t.Fatalf("generate %s: %v: %s", format, err, output)
	}
	data, err := os.ReadFile(filename)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func TestPrepareVideo(t *testing.T) {
	for _, format := range []string{"mp4", "webm"} {
		t.Run(format, func(t *testing.T) {
			data := videoFixture(t, format)
			filename := filepath.Join(t.TempDir(), "upload-without-extension")
			if err := os.WriteFile(filename, data, 0600); err != nil {
				t.Fatal(err)
			}
			f, err := os.Open(filename)
			if err != nil {
				t.Fatal(err)
			}
			defer f.Close()
			media, err := prepareMedia(context.Background(), f)
			if err != nil {
				t.Fatal(err)
			}
			if media.typ != "video/"+format || media.width != 160 || media.height != 90 || media.duration < 1.9 || media.duration > 2.1 {
				t.Fatalf("unexpected metadata: %+v", media)
			}
			thumb, err := jpeg.DecodeConfig(bytes.NewReader(media.thumb))
			if err != nil || thumb.Width != 640 || thumb.Height != 360 {
				t.Fatalf("bad thumbnail: %+v %v", thumb, err)
			}
			if len(media.preview) != 0 {
				t.Fatal("video should preserve and play original bytes")
			}
			if err := os.WriteFile(filename, data[:32], 0600); err != nil {
				t.Fatal(err)
			}
			if _, err := prepareVideo(context.Background(), filename, "video/"+format); !errors.Is(err, errImage) {
				t.Fatalf("corrupt video accepted: %v", err)
			}
		})
	}
}

func TestRejectUnsupportedVideoCodec(t *testing.T) {
	videoFixture(t, "mp4")
	filename := filepath.Join(t.TempDir(), "unsupported.mp4")
	output, err := exec.Command("ffmpeg", "-nostdin", "-v", "error", "-f", "lavfi", "-i", "color=size=32x32:rate=1", "-t", "1", "-threads", "1", "-c:v", "mpeg4", filename).CombinedOutput()
	if err != nil {
		t.Fatalf("fixture: %v %s", err, output)
	}
	if _, err := prepareVideo(context.Background(), filename, "video/mp4"); !errors.Is(err, errImage) {
		t.Fatalf("unsupported codec accepted: %v", err)
	}
}

func TestVideoToolLimits(t *testing.T) {
	data := videoFixture(t, "mp4")
	filename := filepath.Join(t.TempDir(), "sample.mp4")
	if err := os.WriteFile(filename, data, 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := videoCommand(context.Background(), "ffprobe", 8, "-v", "error", "-show_format", filename); !errors.Is(err, errImage) {
		t.Fatalf("output limit not enforced: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := prepareVideo(ctx, filename, "video/mp4"); !errors.Is(err, context.Canceled) {
		t.Fatalf("ignored cancellation: %v", err)
	}
	t.Setenv("PATH", t.TempDir())
	if _, err := prepareVideo(context.Background(), filename, "video/mp4"); !errors.Is(err, errVideoTools) {
		t.Fatalf("missing video tools: %v", err)
	}
}
