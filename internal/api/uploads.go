package api

import (
	"crypto/rand"
	"database/sql"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/rodbove/grimoire/internal/db"
	"github.com/rodbove/grimoire/internal/index"
	"github.com/rodbove/grimoire/internal/media"
	"github.com/rodbove/grimoire/internal/platform"
)

const maxUpload = 64 << 30

func tusHeaders(w http.ResponseWriter) {
	w.Header().Set("Tus-Resumable", "1.0.0")
	w.Header().Set("Cache-Control", "no-store")
}

func parseMeta(h string) map[string]string {
	out := map[string]string{}
	for _, pair := range strings.Split(h, ",") {
		k, v, _ := strings.Cut(strings.TrimSpace(pair), " ")
		if k == "" {
			continue
		}
		b, err := base64.StdEncoding.DecodeString(v)
		if err == nil {
			out[k] = string(b)
		}
	}
	return out
}

type upload struct {
	id      string
	libID   int64
	folder  string
	name    string
	size    int64
	offset  int64
	dup     string
	result  sql.NullString
}

func (s *Server) loadUpload(id string) (*upload, error) {
	u := &upload{id: id}
	err := s.ix.DB.QueryRow(`SELECT library_id, folder, name, size, offset, dup, result FROM uploads WHERE id = ?`, id).
		Scan(&u.libID, &u.folder, &u.name, &u.size, &u.offset, &u.dup, &u.result)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, index.ErrNotFound
	}
	return u, err
}

func (s *Server) uploadCreate(w http.ResponseWriter, r *http.Request) {
	tusHeaders(w)
	size, err := strconv.ParseInt(r.Header.Get("Upload-Length"), 10, 64)
	if err != nil || size < 0 || size > maxUpload {
		writeErr(w, http.StatusBadRequest, "invalid Upload-Length")
		return
	}
	meta := parseMeta(r.Header.Get("Upload-Metadata"))
	name := strings.TrimSpace(meta["filename"])
	if !platform.ValidName(name) || strings.HasPrefix(name, ".") {
		writeErr(w, http.StatusBadRequest, "invalid file name")
		return
	}
	if _, _, ok := media.Classify(name); !ok {
		writeErr(w, http.StatusBadRequest, "unsupported file type")
		return
	}
	folder, ok := platform.CleanRel(meta["folder"])
	if !ok {
		writeErr(w, http.StatusBadRequest, "invalid folder")
		return
	}
	libID, _ := strconv.ParseInt(meta["library"], 10, 64)
	lib, err := s.ix.Library(libID)
	if err != nil {
		fail(w, err)
		return
	}
	if !lib.IsUploadTarget {
		writeErr(w, http.StatusBadRequest, "library is not an upload target")
		return
	}
	dup := meta["dup"]
	if dup != "keep" {
		dup = "skip"
	}
	b := make([]byte, 16)
	rand.Read(b)
	id := hex.EncodeToString(b)
	tmp, err := s.ix.UploadTemp(libID, id)
	if err != nil {
		fail(w, err)
		return
	}
	f, err := os.Create(tmp)
	if err != nil {
		fail(w, err)
		return
	}
	f.Close()
	if _, err := s.ix.DB.Exec(`INSERT INTO uploads (id, library_id, folder, name, size, offset, dup, created_at) VALUES (?, ?, ?, ?, ?, 0, ?, ?)`,
		id, libID, folder, name, size, dup, db.Now()); err != nil {
		os.Remove(tmp)
		fail(w, err)
		return
	}
	w.Header().Set("Location", "/api/uploads/"+id)
	w.WriteHeader(http.StatusCreated)
	if size == 0 {
		s.finishUpload(id)
	}
}

func (s *Server) uploadHead(w http.ResponseWriter, r *http.Request) {
	tusHeaders(w)
	u, err := s.loadUpload(chi.URLParam(r, "id"))
	if err != nil {
		w.WriteHeader(http.StatusNotFound)
		return
	}
	w.Header().Set("Upload-Offset", strconv.FormatInt(u.offset, 10))
	w.Header().Set("Upload-Length", strconv.FormatInt(u.size, 10))
	w.WriteHeader(http.StatusOK)
}

func (s *Server) uploadPatch(w http.ResponseWriter, r *http.Request) {
	tusHeaders(w)
	id := chi.URLParam(r, "id")
	defer lockKey("u" + id)()
	u, err := s.loadUpload(id)
	if err != nil {
		w.WriteHeader(http.StatusNotFound)
		return
	}
	if r.Header.Get("Content-Type") != "application/offset+octet-stream" {
		w.WriteHeader(http.StatusUnsupportedMediaType)
		return
	}
	off, err := strconv.ParseInt(r.Header.Get("Upload-Offset"), 10, 64)
	if err != nil || off != u.offset {
		w.WriteHeader(http.StatusConflict)
		return
	}
	if u.result.Valid {
		w.Header().Set("Upload-Offset", strconv.FormatInt(u.offset, 10))
		w.WriteHeader(http.StatusNoContent)
		return
	}
	tmp, err := s.ix.UploadTemp(u.libID, id)
	if err != nil {
		fail(w, err)
		return
	}
	f, err := os.OpenFile(tmp, os.O_WRONLY, 0o644)
	if err != nil {
		w.WriteHeader(http.StatusNotFound)
		return
	}
	if _, err := f.Seek(off, io.SeekStart); err != nil {
		f.Close()
		fail(w, err)
		return
	}
	n, copyErr := io.Copy(f, io.LimitReader(r.Body, u.size-off))
	f.Close()
	newOff := off + n
	s.ix.DB.Exec(`UPDATE uploads SET offset = ? WHERE id = ?`, newOff, id)
	if copyErr != nil && n == 0 {
		writeErr(w, http.StatusBadRequest, copyErr.Error())
		return
	}
	if newOff == u.size {
		if err := s.finishUpload(id); err != nil {
			fail(w, err)
			return
		}
	}
	w.Header().Set("Upload-Offset", strconv.FormatInt(newOff, 10))
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) finishUpload(id string) error {
	u, err := s.loadUpload(id)
	if err != nil {
		return err
	}
	tmp, err := s.ix.UploadTemp(u.libID, id)
	if err != nil {
		return err
	}
	res, err := s.ix.PlaceUpload(u.libID, u.folder, u.name, tmp, u.dup)
	if err != nil {
		return err
	}
	b, _ := json.Marshal(res)
	_, err = s.ix.DB.Exec(`UPDATE uploads SET result = ? WHERE id = ?`, string(b), id)
	return err
}

func (s *Server) uploadResult(w http.ResponseWriter, r *http.Request) {
	u, err := s.loadUpload(chi.URLParam(r, "id"))
	if err != nil {
		fail(w, err)
		return
	}
	if !u.result.Valid {
		writeJSON(w, http.StatusOK, map[string]any{"status": "pending", "offset": u.offset, "size": u.size})
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.Write([]byte(u.result.String))
}
