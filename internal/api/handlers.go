package api

import (
	"archive/zip"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/rodbove/grimoire/internal/index"
	"github.com/rodbove/grimoire/internal/media"
)

func (s *Server) libraries(w http.ResponseWriter, r *http.Request) {
	libs, err := s.ix.Libraries()
	if err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, libs)
}

type libraryBody struct {
	Name           *string `json:"name"`
	RootPath       *string `json:"rootPath"`
	IsUploadTarget *bool   `json:"isUploadTarget"`
}

func (s *Server) addLibrary(w http.ResponseWriter, r *http.Request) {
	var b libraryBody
	if !decode(w, r, &b) {
		return
	}
	if b.RootPath == nil {
		writeErr(w, http.StatusBadRequest, "rootPath is required")
		return
	}
	name := ""
	if b.Name != nil {
		name = *b.Name
	}
	l, err := s.ix.AddLibrary(name, *b.RootPath, b.IsUploadTarget != nil && *b.IsUploadTarget)
	if err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, l)
}

func (s *Server) updateLibrary(w http.ResponseWriter, r *http.Request) {
	var b libraryBody
	if !decode(w, r, &b) {
		return
	}
	l, err := s.ix.UpdateLibrary(idParam(r), b.Name, b.IsUploadTarget, b.RootPath)
	if err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, l)
}

func (s *Server) removeLibrary(w http.ResponseWriter, r *http.Request) {
	if err := s.ix.RemoveLibrary(idParam(r)); err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (s *Server) scanLibrary(w http.ResponseWriter, r *http.Request) {
	id := idParam(r)
	if _, err := s.ix.Library(id); err != nil {
		fail(w, err)
		return
	}
	s.ix.CheckLibraries()
	go s.ix.Scan(id)
	writeJSON(w, http.StatusAccepted, map[string]bool{"ok": true})
}

func (s *Server) folders(w http.ResponseWriter, r *http.Request) {
	out, err := s.ix.Folders(idParam(r), r.URL.Query().Get("path"))
	if err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) browse(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	lib, _ := strconv.ParseInt(q.Get("library"), 10, 64)
	out, err := s.ix.Folders(lib, q.Get("path"))
	if err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"folders": out})
}

func splitList(v string) []string {
	var out []string
	for _, p := range strings.Split(v, ",") {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}

func parseFilter(q url.Values) index.Filter {
	f := index.Filter{
		Q:          q.Get("q"),
		Kinds:      splitList(q.Get("kind")),
		Folder:     q.Get("folder"),
		Direct:     q.Get("direct") == "1",
		Tags:       splitList(q.Get("tags")),
		AnyTags:    q.Get("tagMode") == "any",
		People:     splitList(q.Get("people")),
		Camera:     q.Get("camera"),
		Classified: q.Get("classified"),
		Duplicates: q.Get("duplicates") == "1",
		Offline:    q.Get("offline") == "1",
	}
	f.Library, _ = strconv.ParseInt(q.Get("library"), 10, 64)
	f.From, _ = strconv.ParseInt(q.Get("from"), 10, 64)
	f.To, _ = strconv.ParseInt(q.Get("to"), 10, 64)
	f.MinRating, _ = strconv.Atoi(q.Get("rating"))
	for _, s := range splitList(q.Get("exclude")) {
		if id, err := strconv.ParseInt(s, 10, 64); err == nil && len(f.Exclude) < 500 {
			f.Exclude = append(f.Exclude, id)
		}
	}
	return f
}

func (s *Server) assets(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	limit, _ := strconv.Atoi(q.Get("limit"))
	page, err := s.ix.Query(parseFilter(q), q.Get("cursor"), limit, q.Get("order") == "asc")
	if err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, page)
}

func (s *Server) assetIDs(w http.ResponseWriter, r *http.Request) {
	ids, err := s.ix.QueryIDs(parseFilter(r.URL.Query()), 50000)
	if err != nil {
		fail(w, err)
		return
	}
	if ids == nil {
		ids = []int64{}
	}
	writeJSON(w, http.StatusOK, ids)
}

func (s *Server) asset(w http.ResponseWriter, r *http.Request) {
	a, err := s.ix.Asset(idParam(r))
	if err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, a)
}

func (s *Server) patchAsset(w http.ResponseWriter, r *http.Request) {
	var p index.Patch
	if !decode(w, r, &p) {
		return
	}
	id := idParam(r)
	if err := s.ix.ApplyPatch([]int64{id}, p); err != nil {
		fail(w, err)
		return
	}
	a, err := s.ix.Asset(id)
	if err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, a)
}

func (s *Server) bulk(w http.ResponseWriter, r *http.Request) {
	var b struct {
		IDs   []int64     `json:"ids"`
		Patch index.Patch `json:"patch"`
	}
	if !decode(w, r, &b) {
		return
	}
	if len(b.IDs) > 50000 {
		writeErr(w, http.StatusBadRequest, "too many assets")
		return
	}
	b.Patch.Tags, b.Patch.People, b.Patch.TakenAt = nil, nil, nil
	if err := s.ix.ApplyPatch(b.IDs, b.Patch); err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]int{"updated": len(b.IDs)})
}

func (s *Server) regenerate(w http.ResponseWriter, r *http.Request) {
	id := idParam(r)
	os.Remove(s.ix.PreviewPath(id))
	if err := s.ix.Regenerate(id); err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func serveFile(w http.ResponseWriter, r *http.Request, p, ctype string, cache string) bool {
	f, err := os.Open(p)
	if err != nil {
		return false
	}
	defer f.Close()
	st, err := f.Stat()
	if err != nil {
		return false
	}
	if ctype != "" {
		w.Header().Set("Content-Type", ctype)
	}
	w.Header().Set("Cache-Control", cache)
	http.ServeContent(w, r, "", st.ModTime(), f)
	return true
}

func (s *Server) thumb(w http.ResponseWriter, r *http.Request) {
	if !serveFile(w, r, s.ix.ThumbPath(idParam(r)), "image/webp", "private, max-age=604800") {
		http.NotFound(w, r)
	}
}

var genLocks sync.Map

func lockKey(k string) func() {
	m, _ := genLocks.LoadOrStore(k, &sync.Mutex{})
	mu := m.(*sync.Mutex)
	mu.Lock()
	return mu.Unlock
}

func (s *Server) preview(w http.ResponseWriter, r *http.Request) {
	id := idParam(r)
	p := s.ix.PreviewPath(id)
	unlock := lockKey("p" + p)
	if _, err := os.Stat(p); err != nil {
		a, err := s.ix.Asset(id)
		if err != nil {
			unlock()
			fail(w, err)
			return
		}
		loc, err := s.ix.AssetFile(id)
		if err != nil {
			unlock()
			fail(w, err)
			return
		}
		if err := s.ix.Tools.Image(loc.Path(), a.Kind, a.Mime, p, 2048, false); err != nil {
			unlock()
			if errors.Is(err, media.ErrNoTool) {
				http.NotFound(w, r)
				return
			}
			fail(w, err)
			return
		}
	} else {
		now := time.Now()
		os.Chtimes(p, now, now)
	}
	unlock()
	if !serveFile(w, r, p, "image/webp", "private, max-age=604800") {
		http.NotFound(w, r)
	}
}

func (s *Server) original(w http.ResponseWriter, r *http.Request) {
	id := idParam(r)
	a, err := s.ix.Asset(id)
	if err != nil {
		fail(w, err)
		return
	}
	loc, err := s.ix.AssetFile(id)
	if err != nil {
		fail(w, err)
		return
	}
	name := path.Base(loc.Rel)
	disp := "inline"
	if r.URL.Query().Get("download") == "1" {
		disp = "attachment"
	}
	w.Header().Set("Content-Disposition", fmt.Sprintf("%s; filename*=UTF-8''%s", disp, url.PathEscape(name)))
	if !serveFile(w, r, loc.Path(), a.Mime, "private, max-age=3600") {
		http.NotFound(w, r)
	}
}

func (s *Server) video(w http.ResponseWriter, r *http.Request) {
	id := idParam(r)
	p := s.ix.VideoPath(id)
	unlock := lockKey("v" + p)
	if _, err := os.Stat(p); err != nil {
		loc, err := s.ix.AssetFile(id)
		if err != nil {
			unlock()
			fail(w, err)
			return
		}
		if err := s.ix.Tools.Transcode(loc.Path(), p); err != nil {
			unlock()
			fail(w, err)
			return
		}
	} else {
		now := time.Now()
		os.Chtimes(p, now, now)
	}
	unlock()
	if !serveFile(w, r, p, "video/mp4", "private, max-age=604800") {
		http.NotFound(w, r)
	}
}

func (s *Server) motion(w http.ResponseWriter, r *http.Request) {
	loc, err := s.ix.AssetFile(idParam(r))
	if err != nil {
		fail(w, err)
		return
	}
	off := media.MotionOffset(loc.Path())
	if off <= 0 {
		http.NotFound(w, r)
		return
	}
	f, err := os.Open(loc.Path())
	if err != nil {
		fail(w, err)
		return
	}
	defer f.Close()
	st, _ := f.Stat()
	w.Header().Set("Content-Type", "video/mp4")
	w.Header().Set("Cache-Control", "private, max-age=3600")
	http.ServeContent(w, r, "", st.ModTime(), io.NewSectionReader(f, off, st.Size()-off))
}

func (s *Server) download(w http.ResponseWriter, r *http.Request) {
	var ids []int64
	for _, p := range splitList(r.URL.Query().Get("ids")) {
		if id, err := strconv.ParseInt(p, 10, 64); err == nil {
			ids = append(ids, id)
		}
	}
	if len(ids) == 0 {
		writeErr(w, http.StatusBadRequest, "no ids")
		return
	}
	w.Header().Set("Content-Type", "application/zip")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="grimoire-%s.zip"`, time.Now().Format("20060102-150405")))
	zw := zip.NewWriter(w)
	defer zw.Close()
	used := map[string]int{}
	for _, id := range ids {
		loc, err := s.ix.AssetFile(id)
		if err != nil {
			continue
		}
		f, err := os.Open(loc.Path())
		if err != nil {
			continue
		}
		st, _ := f.Stat()
		name := path.Base(loc.Rel)
		if n := used[strings.ToLower(name)]; n > 0 {
			ext := path.Ext(name)
			name = fmt.Sprintf("%s (%d)%s", strings.TrimSuffix(name, ext), n, ext)
		}
		used[strings.ToLower(path.Base(loc.Rel))]++
		hw, err := zw.CreateHeader(&zip.FileHeader{Name: name, Method: zip.Store, Modified: st.ModTime()})
		if err == nil {
			io.Copy(hw, f)
		}
		f.Close()
	}
}

func (s *Server) classifyNext(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	a, remaining, err := s.ix.NextUnclassified(parseFilter(q), q.Get("order"))
	if err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"asset": a, "remaining": remaining})
}

func (s *Server) suggest(kind string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
		out, err := s.ix.Suggest(kind, r.URL.Query().Get("prefix"), limit)
		if err != nil {
			fail(w, err)
			return
		}
		writeJSON(w, http.StatusOK, out)
	}
}

func (s *Server) cameras(w http.ResponseWriter, r *http.Request) {
	out, err := s.ix.Cameras()
	if err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) rename(w http.ResponseWriter, r *http.Request) {
	var b struct {
		Name string `json:"name"`
	}
	if !decode(w, r, &b) {
		return
	}
	rel, err := s.ix.Rename(idParam(r), b.Name)
	if err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"path": rel})
}

func (s *Server) move(w http.ResponseWriter, r *http.Request) {
	var b struct {
		LibraryID int64  `json:"libraryId"`
		Folder    string `json:"folder"`
	}
	if !decode(w, r, &b) {
		return
	}
	rel, err := s.ix.Move(idParam(r), b.LibraryID, b.Folder)
	if err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"path": rel})
}

func (s *Server) trashFile(w http.ResponseWriter, r *http.Request) {
	if err := s.ix.Trash(idParam(r)); err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (s *Server) trash(w http.ResponseWriter, r *http.Request) {
	out, err := s.ix.TrashList()
	if err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) restore(w http.ResponseWriter, r *http.Request) {
	if err := s.ix.Restore(idParam(r)); err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (s *Server) purge(w http.ResponseWriter, r *http.Request) {
	if err := s.ix.Purge(idParam(r)); err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (s *Server) failedJobs(w http.ResponseWriter, r *http.Request) {
	out, err := s.ix.FailedJobs()
	if err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) retryJobs(w http.ResponseWriter, r *http.Request) {
	if err := s.ix.RetryFailed(); err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}
