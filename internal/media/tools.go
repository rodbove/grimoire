package media

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"
)

type Tools struct {
	Vips       string
	VipsHeader string
	FFmpeg     string
	FFprobe    string
	Pdftoppm   string
	Exiftool   string

	exifMu   sync.Mutex
	exifCmd  *exec.Cmd
	exifIn   io.WriteCloser
	exifOut  *bufio.Reader
	TempDir  string
	HWEncode string
}

func find(binDir, name string) string {
	exe := name
	if runtime.GOOS == "windows" {
		exe += ".exe"
	}
	if binDir != "" {
		for _, p := range []string{filepath.Join(binDir, exe), filepath.Join(binDir, name, exe)} {
			if st, err := os.Stat(p); err == nil && !st.IsDir() {
				return p
			}
		}
	}
	if p, err := exec.LookPath(name); err == nil {
		return p
	}
	return ""
}

func FindTools(binDir, tempDir string) *Tools {
	return &Tools{
		Vips:       find(binDir, "vipsthumbnail"),
		VipsHeader: find(binDir, "vipsheader"),
		FFmpeg:     find(binDir, "ffmpeg"),
		FFprobe:    find(binDir, "ffprobe"),
		Pdftoppm:   find(binDir, "pdftoppm"),
		Exiftool:   find(binDir, "exiftool"),
		TempDir:    tempDir,
	}
}

func (t *Tools) Report() map[string]bool {
	return map[string]bool{
		"vipsthumbnail": t.Vips != "",
		"vipsheader":    t.VipsHeader != "",
		"ffmpeg":        t.FFmpeg != "",
		"ffprobe":       t.FFprobe != "",
		"pdftoppm":      t.Pdftoppm != "",
		"exiftool":      t.Exiftool != "",
	}
}

var ErrNoTool = errors.New("tool not available")

func run(ctx context.Context, timeout time.Duration, name string, args ...string) ([]byte, error) {
	if name == "" {
		return nil, ErrNoTool
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, name, args...)
	hideWindow(cmd)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		msg := strings.TrimSpace(stderr.String())
		if len(msg) > 400 {
			msg = msg[:400]
		}
		return out, fmt.Errorf("%s: %w: %s", filepath.Base(name), err, msg)
	}
	return out, nil
}

func (t *Tools) exif(path string, tags []string) ([]byte, error) {
	if t.Exiftool == "" {
		return nil, ErrNoTool
	}
	t.exifMu.Lock()
	defer t.exifMu.Unlock()
	if t.exifCmd == nil {
		cmd := exec.Command(t.Exiftool, "-stay_open", "True", "-@", "-")
		hideWindow(cmd)
		in, err := cmd.StdinPipe()
		if err != nil {
			return nil, err
		}
		out, err := cmd.StdoutPipe()
		if err != nil {
			return nil, err
		}
		if err := cmd.Start(); err != nil {
			return nil, err
		}
		t.exifCmd, t.exifIn, t.exifOut = cmd, in, bufio.NewReaderSize(out, 64*1024)
	}
	var b strings.Builder
	b.WriteString("-json\n-n\n-charset\nfilename=utf8\n-fast\n")
	for _, tag := range tags {
		b.WriteString("-" + tag + "\n")
	}
	b.WriteString(path + "\n-execute\n")
	if _, err := io.WriteString(t.exifIn, b.String()); err != nil {
		t.killExif()
		return nil, err
	}
	var buf bytes.Buffer
	done := make(chan error, 1)
	go func() {
		for {
			line, err := t.exifOut.ReadString('\n')
			if err != nil {
				done <- err
				return
			}
			if strings.TrimSpace(line) == "{ready}" {
				done <- nil
				return
			}
			buf.WriteString(line)
		}
	}()
	select {
	case err := <-done:
		if err != nil {
			t.killExif()
			return nil, err
		}
	case <-time.After(60 * time.Second):
		t.killExif()
		return nil, errors.New("exiftool timeout")
	}
	return buf.Bytes(), nil
}

func (t *Tools) killExif() {
	if t.exifCmd != nil && t.exifCmd.Process != nil {
		t.exifCmd.Process.Kill()
		t.exifCmd.Wait()
	}
	t.exifCmd, t.exifIn, t.exifOut = nil, nil, nil
}

func (t *Tools) Close() {
	t.exifMu.Lock()
	defer t.exifMu.Unlock()
	if t.exifIn != nil {
		io.WriteString(t.exifIn, "-stay_open\nFalse\n")
		t.exifIn.Close()
		done := make(chan struct{})
		go func() { t.exifCmd.Wait(); close(done) }()
		select {
		case <-done:
		case <-time.After(3 * time.Second):
			t.exifCmd.Process.Kill()
		}
	}
	t.exifCmd = nil
}

func (t *Tools) tempFile(ext string) string {
	f, err := os.CreateTemp(t.TempDir, "g-*"+ext)
	if err != nil {
		return filepath.Join(os.TempDir(), fmt.Sprintf("g-%d%s", time.Now().UnixNano(), ext))
	}
	name := f.Name()
	f.Close()
	return name
}
