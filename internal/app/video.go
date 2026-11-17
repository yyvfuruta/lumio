package app

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"image/jpeg"
	"math"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

var errVideoTools = errors.New("FFmpeg and FFprobe are required for video uploads")

// Bound the subprocess output as well as runtime; only local, supported containers
// are permitted, so an uploaded file cannot fetch external playlists or URLs.
type cappedOutput struct {
	buffer bytes.Buffer
	limit  int
}

func (b *cappedOutput) Write(p []byte) (int, error) {
	if len(p) > b.limit-b.buffer.Len() {
		return 0, errors.New("media tool output exceeds limit")
	}
	return b.buffer.Write(p)
}
func videoCommand(ctx context.Context, tool string, limit int, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, tool, args...)
	out := &cappedOutput{limit: limit}
	cmd.Stdout = out
	if err := cmd.Run(); err != nil {
		if errors.Is(err, exec.ErrNotFound) {
			return nil, errVideoTools
		}
		if ctx.Err() != nil {
			return nil, fmt.Errorf("video processing stopped: %w", ctx.Err())
		}
		return nil, errImage
	}
	return out.buffer.Bytes(), nil
}
func prepareVideo(ctx context.Context, filename, typ string) (preparedMedia, error) {
	ctx, cancel := context.WithTimeout(ctx, 45*time.Second)
	defer cancel()
	probe, err := videoCommand(ctx, "ffprobe", 64<<10,
		"-v", "error", "-protocol_whitelist", "file,pipe", "-format_whitelist", "mov,matroska,webm",
		"-show_entries", "stream=index,codec_type,codec_name,width,height:stream_disposition=attached_pic:format=duration,format_name", "-of", "json", filename)
	if err != nil {
		return preparedMedia{}, err
	}
	var info struct {
		Streams []struct {
			Index       int    `json:"index"`
			Type        string `json:"codec_type"`
			Codec       string `json:"codec_name"`
			Width       int    `json:"width"`
			Height      int    `json:"height"`
			Disposition struct {
				AttachedPic int `json:"attached_pic"`
			} `json:"disposition"`
		} `json:"streams"`
		Format struct {
			Duration string `json:"duration"`
			Name     string `json:"format_name"`
		} `json:"format"`
	}
	if err := json.Unmarshal(probe, &info); err != nil {
		return preparedMedia{}, errImage
	}
	duration, err := strconv.ParseFloat(info.Format.Duration, 64)
	if err != nil || math.IsNaN(duration) || math.IsInf(duration, 0) || duration <= 0 {
		return preparedMedia{}, errImage
	}
	if typ == "video/mp4" && !strings.Contains(info.Format.Name, "mp4") || typ == "video/webm" && !strings.Contains(info.Format.Name, "webm") {
		return preparedMedia{}, errImage
	}
	videoIndex := -1
	for _, stream := range info.Streams {
		if stream.Type == "audio" {
			if typ == "video/mp4" && stream.Codec != "aac" || typ == "video/webm" && stream.Codec != "opus" && stream.Codec != "vorbis" {
				return preparedMedia{}, errImage
			}
		}
		if stream.Type != "video" || stream.Disposition.AttachedPic != 0 {
			continue
		}
		if videoIndex != -1 {
			return preparedMedia{}, errImage
		}
		if typ == "video/mp4" && stream.Codec != "h264" || typ == "video/webm" && stream.Codec != "vp8" && stream.Codec != "vp9" && stream.Codec != "av1" {
			return preparedMedia{}, errImage
		}
		if stream.Width <= 0 || stream.Height <= 0 || stream.Width > maxImagePixels/stream.Height {
			return preparedMedia{}, errImage
		}
		videoIndex = stream.Index
	}
	if videoIndex < 0 {
		return preparedMedia{}, errImage
	}
	thumb, err := videoCommand(ctx, "ffmpeg", 4<<20,
		"-nostdin", "-v", "error", "-threads", "1", "-protocol_whitelist", "file,pipe", "-format_whitelist", "mov,matroska,webm",
		"-i", filename, "-map", fmt.Sprintf("0:%d", videoIndex), "-frames:v", "1", "-an", "-sn", "-dn",
		"-vf", "scale=640:640:force_original_aspect_ratio=decrease", "-threads", "1", "-f", "image2pipe", "-c:v", "mjpeg", "pipe:1")
	if err != nil {
		return preparedMedia{}, err
	}
	cfg, err := jpeg.DecodeConfig(bytes.NewReader(thumb))
	if err != nil {
		return preparedMedia{}, errImage
	}
	// FFmpeg applies rotation to the thumbnail; report the same orientation.
	var width, height int
	for _, stream := range info.Streams {
		if stream.Index == videoIndex {
			width, height = stream.Width, stream.Height
		}
	}
	if (width > height) != (cfg.Width > cfg.Height) {
		width, height = height, width
	}
	return preparedMedia{typ: typ, width: width, height: height, duration: duration, thumb: thumb}, nil
}
