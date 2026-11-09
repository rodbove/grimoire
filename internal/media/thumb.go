package media

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

func (t *Tools) vips(src, dst string, size int, square bool) error {
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	tmp := dst + ".tmp.webp"
	args := []string{src, "--size", fmt.Sprintf("%dx%d", size, size)}
	if square {
		args = append(args, "--smartcrop", "attention")
	}
	args = append(args, "-o", tmp+"[Q=78,strip]")
	if _, err := run(context.Background(), 2*time.Minute, t.Vips, args...); err != nil {
		os.Remove(tmp)
		return err
	}
	if st, err := os.Stat(tmp); err != nil || st.Size() == 0 {
		os.Remove(tmp)
		return errors.New("vipsthumbnail wrote no output")
	}
	return os.Rename(tmp, dst)
}

func (t *Tools) Image(src, kind, mime, dst string, size int, square bool) error {
	switch kind {
	case KindPhoto, KindLivePhoto:
		err := t.vips(src, dst, size, square)
		if err == nil || t.Exiftool == "" {
			return err
		}
		prev := t.tempFile(".jpg")
		defer os.Remove(prev)
		if perr := t.exifPreview(src, prev); perr != nil {
			return err
		}
		return t.vips(prev, dst, size, square)
	case KindVideo, KindLiveVideo:
		frame := t.tempFile(".jpg")
		defer os.Remove(frame)
		for _, ss := range []string{"1", "0"} {
			_, err := run(context.Background(), 2*time.Minute, t.FFmpeg, "-y", "-v", "error", "-ss", ss, "-i", src,
				"-frames:v", "1", "-vf", "scale='min(1600,iw)':-2", "-q:v", "3", frame)
			if st, serr := os.Stat(frame); err == nil && serr == nil && st.Size() > 0 {
				return t.vips(frame, dst, size, square)
			}
			if err != nil && ss == "0" {
				return err
			}
		}
		return errors.New("no video frame")
	case KindDocument:
		if mime != "application/pdf" {
			return ErrNoTool
		}
		base := strings.TrimSuffix(t.tempFile(".jpg"), ".jpg")
		os.Remove(base + ".jpg")
		defer os.Remove(base + ".jpg")
		if _, err := run(context.Background(), 2*time.Minute, t.Pdftoppm, "-f", "1", "-l", "1", "-r", "110", "-jpeg", "-singlefile", src, base); err != nil {
			return err
		}
		return t.vips(base+".jpg", dst, size, square)
	}
	return ErrNoTool
}

func (t *Tools) exifPreview(src, dst string) error {
	if t.Exiftool == "" {
		return ErrNoTool
	}
	for _, tag := range []string{"-JpgFromRaw", "-PreviewImage"} {
		out, err := run(context.Background(), time.Minute, t.Exiftool, "-b", tag, src)
		if err == nil && len(out) > 1024 {
			return os.WriteFile(dst, out, 0o644)
		}
	}
	return errors.New("no embedded preview")
}

func (t *Tools) Transcode(src, dst string) error {
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	tmp := dst + ".tmp.mp4"
	enc := []string{"-c:v", "libx264", "-preset", "veryfast", "-crf", "24"}
	if t.HWEncode != "" {
		enc = []string{"-c:v", t.HWEncode, "-b:v", "3M"}
	}
	args := []string{"-y", "-v", "error", "-i", src, "-map", "0:v:0", "-map", "0:a:0?"}
	args = append(args, enc...)
	args = append(args, "-pix_fmt", "yuv420p", "-vf", "scale=-2:'min(720,ih)'", "-c:a", "aac", "-b:a", "128k", "-movflags", "+faststart", tmp)
	if _, err := run(context.Background(), 2*time.Hour, t.FFmpeg, args...); err != nil {
		os.Remove(tmp)
		return err
	}
	return os.Rename(tmp, dst)
}
