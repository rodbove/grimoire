package api

import (
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"

	"github.com/rodbove/grimoire/internal/platform"
)

func (s *Server) fsBrowse(w http.ResponseWriter, r *http.Request) {
	p := strings.TrimSpace(r.URL.Query().Get("path"))
	if p == "" {
		if runtime.GOOS == "windows" {
			var drives []string
			for c := 'A'; c <= 'Z'; c++ {
				d := string(c) + `:\`
				if _, err := os.Stat(d); err == nil {
					drives = append(drives, d)
				}
			}
			writeJSON(w, http.StatusOK, map[string]any{"path": "", "parent": "", "dirs": drives})
			return
		}
		p = "/"
	}
	if !filepath.IsAbs(p) {
		writeErr(w, http.StatusBadRequest, "path must be absolute")
		return
	}
	p = filepath.Clean(p)
	entries, err := os.ReadDir(p)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "cannot read folder")
		return
	}
	dirs := []string{}
	for _, e := range entries {
		if !e.IsDir() || platform.Skip(e.Name()) {
			continue
		}
		dirs = append(dirs, filepath.Join(p, e.Name()))
	}
	sort.Slice(dirs, func(i, j int) bool { return strings.ToLower(dirs[i]) < strings.ToLower(dirs[j]) })
	parent := filepath.Dir(p)
	if parent == p {
		parent = ""
		if runtime.GOOS != "windows" {
			parent = "/"
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"path": p, "parent": parent, "dirs": dirs})
}
