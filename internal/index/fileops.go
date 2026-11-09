package index

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/rodbove/grimoire/internal/db"
	"github.com/rodbove/grimoire/internal/media"
	"github.com/rodbove/grimoire/internal/platform"
	"lukechampine.com/blake3"
)

const metaDir = ".grimoire"

func exists(p string) bool {
	_, err := os.Lstat(p)
	return err == nil
}

func uniqueRel(root, rel string) string {
	if !exists(platform.Join(root, rel)) {
		return rel
	}
	dir, name := path.Split(rel)
	ext := path.Ext(name)
	base := strings.TrimSuffix(name, ext)
	for i := 1; ; i++ {
		cand := fmt.Sprintf("%s%s (%d)%s", dir, base, i, ext)
		if !exists(platform.Join(root, cand)) {
			return cand
		}
	}
}

func (ix *Indexer) onlineFile(fileID int64) (Location, error) {
	loc, status, err := ix.FileLocation(fileID)
	if err != nil {
		return loc, err
	}
	if !loc.Online {
		return loc, ErrOffline
	}
	if status != "ok" {
		return loc, UserError(ErrConflict, "file is "+status)
	}
	return loc, nil
}

func (ix *Indexer) Rename(fileID int64, newName string) (string, error) {
	newName = strings.TrimSpace(newName)
	if !platform.ValidName(newName) || strings.HasPrefix(newName, ".") {
		return "", UserError(ErrInvalid, "invalid file name")
	}
	if _, _, ok := media.Classify(newName); !ok {
		return "", UserError(ErrInvalid, "unsupported file extension")
	}
	loc, err := ix.onlineFile(fileID)
	if err != nil {
		return "", err
	}
	if !strings.EqualFold(path.Ext(newName), path.Ext(loc.Rel)) {
		return "", UserError(ErrInvalid, "file extension must not change")
	}
	defer ix.lock(fmt.Sprintf("file:%d", fileID))()
	newRel := path.Join(path.Dir(loc.Rel), newName)
	if path.Dir(loc.Rel) == "." {
		newRel = newName
	}
	if newRel == loc.Rel {
		return newRel, nil
	}
	dst := platform.Join(loc.Root, newRel)
	if exists(dst) && !strings.EqualFold(newRel, loc.Rel) {
		return "", UserError(ErrConflict, "a file with this name exists")
	}
	if err := os.Rename(loc.Path(), dst); err != nil {
		return "", err
	}
	if _, err := ix.DB.Exec(`UPDATE files SET rel_path = ? WHERE id = ?`, newRel, fileID); err != nil {
		return "", err
	}
	ix.refreshFileAsset(fileID)
	return newRel, nil
}

func (ix *Indexer) refreshFileAsset(fileID int64) {
	var assetID int64
	if ix.DB.QueryRow(`SELECT asset_id FROM files WHERE id = ?`, fileID).Scan(&assetID) == nil {
		ix.refreshFTS(assetID)
	}
}

func (ix *Indexer) Move(fileID, libID int64, folder string) (string, error) {
	folder, ok := platform.CleanRel(folder)
	if !ok {
		return "", UserError(ErrInvalid, "invalid folder")
	}
	loc, err := ix.onlineFile(fileID)
	if err != nil {
		return "", err
	}
	target, err := ix.Library(libID)
	if err != nil {
		return "", err
	}
	if target.Status != "online" {
		return "", ErrOffline
	}
	defer ix.lock(fmt.Sprintf("file:%d", fileID))()
	name := path.Base(loc.Rel)
	newRel := name
	if folder != "" {
		newRel = folder + "/" + name
	}
	if libID == loc.LibraryID && newRel == loc.Rel {
		return newRel, nil
	}
	newRel = uniqueRel(target.RootPath, newRel)
	dst := platform.Join(target.RootPath, newRel)
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return "", err
	}
	if err := moveFile(loc.Path(), dst); err != nil {
		return "", err
	}
	if _, err := ix.DB.Exec(`UPDATE files SET library_id = ?, rel_path = ? WHERE id = ?`, libID, newRel, fileID); err != nil {
		return "", err
	}
	ix.refreshFileAsset(fileID)
	return newRel, nil
}

func moveFile(src, dst string) error {
	if err := os.Rename(src, dst); err == nil {
		return nil
	}
	if exists(dst) {
		return UserError(ErrConflict, "destination exists")
	}
	tmp := dst + ".grimoire-part"
	sum, err := copyHash(src, tmp)
	if err != nil {
		os.Remove(tmp)
		return err
	}
	check, err := fullHash(tmp)
	if err != nil || !bytes.Equal(sum, check) {
		os.Remove(tmp)
		return errors.New("copy verification failed")
	}
	if st, err := os.Stat(src); err == nil {
		os.Chtimes(tmp, st.ModTime(), st.ModTime())
	}
	if err := os.Rename(tmp, dst); err != nil {
		os.Remove(tmp)
		return err
	}
	return os.Remove(src)
}

func copyHash(src, dst string) ([]byte, error) {
	in, err := os.Open(src)
	if err != nil {
		return nil, err
	}
	defer in.Close()
	out, err := os.OpenFile(dst, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o644)
	if err != nil {
		return nil, err
	}
	h := blake3.New(32, nil)
	buf := make([]byte, 1<<20)
	if _, err := io.CopyBuffer(io.MultiWriter(out, h), in, buf); err != nil {
		out.Close()
		return nil, err
	}
	if err := out.Sync(); err != nil {
		out.Close()
		return nil, err
	}
	if err := out.Close(); err != nil {
		return nil, err
	}
	return h.Sum(nil), nil
}

func (ix *Indexer) Trash(fileID int64) error {
	loc, err := ix.onlineFile(fileID)
	if err != nil {
		return err
	}
	defer ix.lock(fmt.Sprintf("file:%d", fileID))()
	trashRel := fmt.Sprintf("%s/trash/%d_%s", metaDir, fileID, path.Base(loc.Rel))
	dst := platform.Join(loc.Root, trashRel)
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	if err := os.Rename(loc.Path(), dst); err != nil {
		return err
	}
	tx, err := ix.DB.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.Exec(`UPDATE files SET status = 'trashed' WHERE id = ?`, fileID); err != nil {
		return err
	}
	if _, err := tx.Exec(`INSERT INTO trash (file_id, original_rel_path, trash_rel_path, deleted_at) VALUES (?, ?, ?, ?)`, fileID, loc.Rel, trashRel, db.Now()); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	ix.refreshFileAsset(fileID)
	return nil
}

type TrashItem struct {
	ID        int64  `json:"id"`
	FileID    int64  `json:"fileId"`
	AssetID   *int64 `json:"assetId"`
	Library   string `json:"library"`
	Path      string `json:"path"`
	DeletedAt int64  `json:"deletedAt"`
	Kind      string `json:"kind"`
}

func (ix *Indexer) TrashList() ([]TrashItem, error) {
	rows, err := ix.DB.Query(`SELECT t.id, t.file_id, f.asset_id, l.name, t.original_rel_path, t.deleted_at, coalesce(a.kind, '')
		FROM trash t JOIN files f ON f.id = t.file_id JOIN libraries l ON l.id = f.library_id LEFT JOIN assets a ON a.id = f.asset_id
		ORDER BY t.deleted_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []TrashItem{}
	for rows.Next() {
		var t TrashItem
		if err := rows.Scan(&t.ID, &t.FileID, &t.AssetID, &t.Library, &t.Path, &t.DeletedAt, &t.Kind); err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

func (ix *Indexer) trashEntry(id int64) (Location, string, string, error) {
	var fileID int64
	var orig, trashRel string
	if err := ix.DB.QueryRow(`SELECT file_id, original_rel_path, trash_rel_path FROM trash WHERE id = ?`, id).Scan(&fileID, &orig, &trashRel); err != nil {
		return Location{}, "", "", ErrNotFound
	}
	loc, _, err := ix.FileLocation(fileID)
	if err != nil {
		return loc, "", "", err
	}
	if !loc.Online {
		return loc, "", "", ErrOffline
	}
	return loc, orig, trashRel, nil
}

func (ix *Indexer) Restore(id int64) error {
	loc, orig, trashRel, err := ix.trashEntry(id)
	if err != nil {
		return err
	}
	rel := uniqueRel(loc.Root, orig)
	dst := platform.Join(loc.Root, rel)
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	if err := os.Rename(platform.Join(loc.Root, trashRel), dst); err != nil {
		return err
	}
	tx, err := ix.DB.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.Exec(`UPDATE files SET status = 'ok', rel_path = ? WHERE id = ?`, rel, loc.FileID); err != nil {
		return err
	}
	if _, err := tx.Exec(`DELETE FROM trash WHERE id = ?`, id); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	ix.refreshFileAsset(loc.FileID)
	return nil
}

func (ix *Indexer) Purge(id int64) error {
	loc, _, trashRel, err := ix.trashEntry(id)
	if err != nil {
		return err
	}
	if err := os.Remove(platform.Join(loc.Root, trashRel)); err != nil && !os.IsNotExist(err) {
		return err
	}
	var assetID *int64
	ix.DB.QueryRow(`SELECT asset_id FROM files WHERE id = ?`, loc.FileID).Scan(&assetID)
	tx, err := ix.DB.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.Exec(`DELETE FROM files WHERE id = ?`, loc.FileID); err != nil {
		return err
	}
	if assetID != nil {
		if _, err := tx.Exec(`DELETE FROM assets WHERE id = ? AND NOT EXISTS (SELECT 1 FROM files WHERE asset_id = ?)`, *assetID, *assetID); err != nil {
			return err
		}
		if _, err := tx.Exec(`DELETE FROM assets_fts WHERE rowid = ? AND rowid NOT IN (SELECT id FROM assets)`, *assetID); err != nil {
			return err
		}
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	if assetID != nil {
		var n int
		if ix.DB.QueryRow(`SELECT count(*) FROM assets WHERE id = ?`, *assetID).Scan(&n); n == 0 {
			os.Remove(ix.thumbPath(*assetID))
			os.Remove(ix.PreviewPath(*assetID))
			os.Remove(ix.VideoPath(*assetID))
		}
	}
	return nil
}

func (ix *Indexer) PurgeOlderThan(age time.Duration) {
	cutoff := time.Now().Add(-age).UnixMilli()
	rows, err := ix.DB.Query(`SELECT id FROM trash WHERE deleted_at < ?`, cutoff)
	if err != nil {
		return
	}
	var ids []int64
	for rows.Next() {
		var id int64
		if rows.Scan(&id) == nil {
			ids = append(ids, id)
		}
	}
	rows.Close()
	for _, id := range ids {
		ix.Purge(id)
	}
}

func (ix *Indexer) Folders(libID int64, rel string) ([]string, error) {
	rel, ok := platform.CleanRel(rel)
	if !ok {
		return nil, UserError(ErrInvalid, "invalid folder")
	}
	l, err := ix.Library(libID)
	if err != nil {
		return nil, err
	}
	if l.Status != "online" {
		return nil, ErrOffline
	}
	entries, err := os.ReadDir(platform.Join(l.RootPath, rel))
	if err != nil {
		if os.IsNotExist(err) {
			return []string{}, nil
		}
		return nil, err
	}
	out := []string{}
	for _, e := range entries {
		if e.IsDir() && !platform.Skip(e.Name()) {
			out = append(out, e.Name())
		}
	}
	sort.Slice(out, func(i, j int) bool { return strings.ToLower(out[i]) < strings.ToLower(out[j]) })
	return out, nil
}

func (ix *Indexer) UploadTemp(libID int64, id string) (string, error) {
	l, err := ix.Library(libID)
	if err != nil {
		return "", err
	}
	if l.Status != "online" {
		return "", ErrOffline
	}
	dir := filepath.Join(l.RootPath, metaDir, "uploads")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	return filepath.Join(dir, id+".part"), nil
}

type PlaceResult struct {
	Status  string `json:"status"`
	AssetID int64  `json:"assetId"`
	Path    string `json:"path"`
}

func (ix *Indexer) PlaceUpload(libID int64, folder, name, tmp, dup string) (PlaceResult, error) {
	kind, mime, ok := media.Classify(name)
	if !ok {
		os.Remove(tmp)
		return PlaceResult{}, UserError(ErrInvalid, "unsupported file type")
	}
	l, err := ix.Library(libID)
	if err != nil {
		return PlaceResult{}, err
	}
	h, err := fullHash(tmp)
	if err != nil {
		return PlaceResult{}, err
	}
	var existing int64
	if ix.DB.QueryRow(`SELECT id FROM assets WHERE hash = ?`, h).Scan(&existing) == nil && dup != "keep" {
		os.Remove(tmp)
		return PlaceResult{Status: "duplicate", AssetID: existing}, nil
	}
	if folder == "" {
		ts := time.Now()
		m := ix.Tools.Extract(tmp, kind, mime)
		if m.TakenAt > 0 {
			ts = time.UnixMilli(m.TakenAt)
		}
		folder = ts.Format("2006/01")
	}
	rel := uniqueRel(l.RootPath, folder+"/"+name)
	dst := platform.Join(l.RootPath, rel)
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return PlaceResult{}, err
	}
	if err := os.Rename(tmp, dst); err != nil {
		return PlaceResult{}, err
	}
	st, err := os.Stat(dst)
	if err != nil {
		return PlaceResult{}, err
	}
	fileID, err := upsertFile(ix.DB, libID, rel, st.Size(), st.ModTime().UnixMilli())
	if err != nil {
		return PlaceResult{}, err
	}
	assetID, err := ix.indexFile(fileID, prioUpload)
	if err != nil {
		return PlaceResult{}, err
	}
	ix.DB.Exec(`UPDATE assets SET hash = ? WHERE id = ? AND hash IS NULL AND NOT EXISTS (SELECT 1 FROM assets WHERE hash = ?)`, h, assetID, h)
	return PlaceResult{Status: "ok", AssetID: assetID, Path: rel}, nil
}
