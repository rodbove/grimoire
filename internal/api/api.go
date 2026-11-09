package api

import (
	"encoding/json"
	"errors"
	"io/fs"
	"log/slog"
	"net/http"
	"path"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/rodbove/grimoire/internal/auth"
	"github.com/rodbove/grimoire/internal/index"
)

type Server struct {
	ix     *index.Indexer
	auth   *auth.Auth
	static fs.FS
}

func New(ix *index.Indexer, a *auth.Auth, static fs.FS) http.Handler {
	s := &Server{ix: ix, auth: a, static: static}
	r := chi.NewRouter()
	r.Use(middleware.Recoverer)
	r.Route("/api", func(r chi.Router) {
		r.Use(s.csrf)
		r.Get("/auth/state", s.authState)
		r.Post("/auth/setup", s.authSetup)
		r.Post("/auth/login", s.authLogin)
		r.Post("/auth/logout", s.authLogout)
		r.Group(func(r chi.Router) {
			r.Use(s.requireAuth)
			r.Post("/auth/password", s.changePassword)
			r.Get("/status", s.status)
			r.Get("/events", s.events)
			r.Get("/system", s.system)

			r.Get("/libraries", s.libraries)
			r.Post("/libraries", s.addLibrary)
			r.Patch("/libraries/{id}", s.updateLibrary)
			r.Delete("/libraries/{id}", s.removeLibrary)
			r.Post("/libraries/{id}/scan", s.scanLibrary)
			r.Get("/libraries/{id}/folders", s.folders)
			r.Get("/browse", s.browse)
			r.Get("/fs", s.fsBrowse)

			r.Get("/assets", s.assets)
			r.Get("/assets/ids", s.assetIDs)
			r.Post("/assets/bulk", s.bulk)
			r.Get("/assets/{id}", s.asset)
			r.Patch("/assets/{id}", s.patchAsset)
			r.Post("/assets/{id}/regenerate", s.regenerate)
			r.Get("/assets/{id}/thumb", s.thumb)
			r.Get("/assets/{id}/preview", s.preview)
			r.Get("/assets/{id}/original", s.original)
			r.Get("/assets/{id}/video", s.video)
			r.Get("/assets/{id}/motion", s.motion)
			r.Get("/download", s.download)

			r.Get("/classify/next", s.classifyNext)

			r.Get("/tags", s.suggest("tags"))
			r.Get("/people", s.suggest("people"))
			r.Get("/cameras", s.cameras)

			r.Post("/files/{id}/rename", s.rename)
			r.Post("/files/{id}/move", s.move)
			r.Delete("/files/{id}", s.trashFile)
			r.Get("/trash", s.trash)
			r.Post("/trash/{id}/restore", s.restore)
			r.Delete("/trash/{id}", s.purge)

			r.Get("/jobs/failed", s.failedJobs)
			r.Post("/jobs/retry", s.retryJobs)

			r.Post("/uploads", s.uploadCreate)
			r.Head("/uploads/{id}", s.uploadHead)
			r.Patch("/uploads/{id}", s.uploadPatch)
			r.Get("/uploads/{id}/result", s.uploadResult)
		})
	})
	r.NotFound(s.spa)
	return r
}

func (s *Server) csrf(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet, http.MethodHead, http.MethodOptions:
		default:
			if r.Header.Get("X-Grimoire") != "1" {
				writeErr(w, http.StatusForbidden, "missing request header")
				return
			}
		}
		next.ServeHTTP(w, r)
	})
}

func (s *Server) requireAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !s.auth.Valid(r) {
			writeErr(w, http.StatusUnauthorized, "login required")
			return
		}
		next.ServeHTTP(w, r)
	})
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(v)
}

func writeErr(w http.ResponseWriter, code int, msg string) {
	writeJSON(w, code, map[string]string{"error": msg})
}

func fail(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, index.ErrNotFound):
		writeErr(w, http.StatusNotFound, err.Error())
	case errors.Is(err, index.ErrInvalid):
		writeErr(w, http.StatusBadRequest, err.Error())
	case errors.Is(err, index.ErrConflict):
		writeErr(w, http.StatusConflict, err.Error())
	case errors.Is(err, index.ErrOffline):
		writeErr(w, http.StatusServiceUnavailable, err.Error())
	default:
		slog.Error("request failed", "err", err)
		writeErr(w, http.StatusInternalServerError, err.Error())
	}
}

func decode(w http.ResponseWriter, r *http.Request, v any) bool {
	r.Body = http.MaxBytesReader(w, r.Body, 4<<20)
	if err := json.NewDecoder(r.Body).Decode(v); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid JSON")
		return false
	}
	return true
}

func idParam(r *http.Request) int64 {
	id, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	return id
}

func (s *Server) spa(w http.ResponseWriter, r *http.Request) {
	if strings.HasPrefix(r.URL.Path, "/api/") || s.static == nil {
		http.NotFound(w, r)
		return
	}
	p := strings.TrimPrefix(path.Clean(r.URL.Path), "/")
	if p == "" {
		p = "index.html"
	}
	if st, err := fs.Stat(s.static, p); err != nil || st.IsDir() {
		p = "index.html"
	}
	if strings.HasPrefix(p, "_app/immutable/") {
		w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
	} else {
		w.Header().Set("Cache-Control", "no-cache")
	}
	f, err := s.static.Open(p)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	defer f.Close()
	st, _ := f.Stat()
	rs, ok := f.(interface {
		Read([]byte) (int, error)
		Seek(int64, int) (int64, error)
	})
	if !ok {
		http.NotFound(w, r)
		return
	}
	http.ServeContent(w, r, p, st.ModTime(), rs)
}

func (s *Server) authState(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]bool{"setUp": s.auth.IsSetUp(), "authenticated": s.auth.Valid(r)})
}

type passwordBody struct {
	Password    string `json:"password"`
	NewPassword string `json:"newPassword"`
}

func (s *Server) authSetup(w http.ResponseWriter, r *http.Request) {
	var b passwordBody
	if !decode(w, r, &b) {
		return
	}
	if err := s.auth.Setup(w, b.Password); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (s *Server) authLogin(w http.ResponseWriter, r *http.Request) {
	var b passwordBody
	if !decode(w, r, &b) {
		return
	}
	if err := s.auth.Login(w, b.Password); err != nil {
		writeErr(w, http.StatusUnauthorized, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (s *Server) authLogout(w http.ResponseWriter, r *http.Request) {
	s.auth.Logout(w, r)
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (s *Server) changePassword(w http.ResponseWriter, r *http.Request) {
	var b passwordBody
	if !decode(w, r, &b) {
		return
	}
	if !s.auth.Check(b.Password) {
		writeErr(w, http.StatusUnauthorized, "wrong password")
		return
	}
	if err := s.auth.SetPassword(b.NewPassword); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	s.auth.Login(w, b.NewPassword)
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (s *Server) status(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, s.ix.Status())
}

func (s *Server) system(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"tools": s.ix.Tools.Report(), "dataDir": s.ix.DataDir, "workers": s.ix.Workers})
}

func (s *Server) events(w http.ResponseWriter, r *http.Request) {
	fl, ok := w.(http.Flusher)
	if !ok {
		writeErr(w, http.StatusInternalServerError, "streaming unsupported")
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Accel-Buffering", "no")
	ch, cancel := s.ix.Hub.Subscribe()
	defer cancel()
	b, _ := json.Marshal(s.ix.Status())
	w.Write([]byte("data: " + string(b) + "\n\n"))
	fl.Flush()
	ping := time.NewTicker(25 * time.Second)
	defer ping.Stop()
	for {
		select {
		case <-r.Context().Done():
			return
		case msg := <-ch:
			w.Write([]byte("data: " + string(msg) + "\n\n"))
			fl.Flush()
		case <-ping.C:
			w.Write([]byte(": ping\n\n"))
			fl.Flush()
		}
	}
}
