package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/kardianos/service"
	"github.com/rodbove/grimoire/internal/api"
	"github.com/rodbove/grimoire/internal/auth"
	"github.com/rodbove/grimoire/internal/db"
	"github.com/rodbove/grimoire/internal/index"
	"github.com/rodbove/grimoire/internal/media"
	"github.com/rodbove/grimoire/internal/platform"
	"github.com/rodbove/grimoire/web"
)

type program struct {
	addr    string
	dataDir string
	binDir  string
	workers int
	hwenc   string

	srv  *http.Server
	ix   *index.Indexer
	done chan struct{}
}

func (p *program) Start(s service.Service) error {
	if err := os.MkdirAll(p.dataDir, 0o755); err != nil {
		return err
	}
	for _, d := range []string{"thumbs", "previews", "video", "tmp", "logs", "backups"} {
		os.MkdirAll(filepath.Join(p.dataDir, d), 0o755)
	}
	logFile, err := os.OpenFile(filepath.Join(p.dataDir, "logs", "grimoire.log"), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	var out io.Writer = os.Stderr
	if err == nil {
		if service.Interactive() {
			out = io.MultiWriter(os.Stderr, logFile)
		} else {
			out = logFile
		}
	}
	slog.SetDefault(slog.New(slog.NewTextHandler(out, &slog.HandlerOptions{Level: slog.LevelInfo})))

	d, err := db.Open(filepath.Join(p.dataDir, "grimoire.db"))
	if err != nil {
		return fmt.Errorf("open database: %w", err)
	}
	tmp := filepath.Join(p.dataDir, "tmp")
	entries, _ := os.ReadDir(tmp)
	for _, e := range entries {
		os.Remove(filepath.Join(tmp, e.Name()))
	}
	tools := media.FindTools(p.binDir, tmp)
	tools.HWEncode = p.hwenc
	slog.Info("tools", "found", tools.Report())
	p.ix = index.New(d, tools, p.dataDir)
	if p.workers > 0 {
		p.ix.Workers = p.workers
	}
	if err := p.ix.Start(); err != nil {
		return err
	}
	ln, err := net.Listen("tcp", p.addr)
	if err != nil {
		return fmt.Errorf("listen %s: %w", p.addr, err)
	}
	p.srv = &http.Server{
		Handler:           api.New(p.ix, &auth.Auth{DB: d}, web.FS()),
		ReadHeaderTimeout: 15 * time.Second,
		IdleTimeout:       2 * time.Minute,
	}
	p.done = make(chan struct{})
	go func() {
		defer close(p.done)
		slog.Info("grimoire listening", "addr", p.addr, "data", p.dataDir)
		if err := p.srv.Serve(ln); err != nil && !errors.Is(err, http.ErrServerClosed) {
			slog.Error("server", "err", err)
		}
	}()
	return nil
}

func (p *program) Stop(s service.Service) error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if p.srv != nil {
		p.srv.Shutdown(ctx)
		<-p.done
	}
	if p.ix != nil {
		p.ix.Stop()
		p.ix.Tools.Close()
		p.ix.DB.Close()
	}
	return nil
}

func main() {
	exe, _ := os.Executable()
	defaultBin := filepath.Join(filepath.Dir(exe), "bin")
	p := &program{}
	fs := flag.NewFlagSet("grimoire", flag.ExitOnError)
	fs.StringVar(&p.addr, "addr", ":7777", "listen address")
	fs.StringVar(&p.dataDir, "data", platform.DefaultDataDir(), "app data directory")
	fs.StringVar(&p.binDir, "bin", defaultBin, "directory with bundled tools")
	fs.IntVar(&p.workers, "workers", 0, "media workers (0 = CPU count - 1)")
	fs.StringVar(&p.hwenc, "hwenc", "", "ffmpeg hardware H.264 encoder, for example h264_v4l2m2m")

	args := os.Args[1:]
	cmd := "run"
	if len(args) > 0 && len(args[0]) > 0 && args[0][0] != '-' {
		cmd, args = args[0], args[1:]
	}
	fs.Parse(args)
	p.dataDir, _ = filepath.Abs(p.dataDir)

	cfg := &service.Config{
		Name:        "grimoire",
		DisplayName: "Grimoire",
		Description: "Grimoire media library server",
		Arguments:   append([]string{"run"}, args...),
	}
	svc, err := service.New(p, cfg)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	switch cmd {
	case "run":
		if err := svc.Run(); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
	case "install", "uninstall", "start", "stop", "restart":
		if err := service.Control(svc, cmd); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		fmt.Println("service:", cmd, "ok")
	default:
		fmt.Fprintf(os.Stderr, "usage: grimoire [run|install|uninstall|start|stop|restart] [flags]\n")
		fs.PrintDefaults()
		os.Exit(2)
	}
}
