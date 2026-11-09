package index

import (
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"

	"github.com/rodbove/grimoire/internal/db"
	"github.com/rodbove/grimoire/internal/media"
	"github.com/rodbove/grimoire/internal/platform"
)

type Indexer struct {
	DB      *sql.DB
	Tools   *media.Tools
	DataDir string
	Hub     *Hub

	scanning sync.Map
	wake     chan struct{}
	watcher  *watcher
	locks    sync.Map
	stop     chan struct{}
	wg       sync.WaitGroup
	Workers  int
}

func New(d *sql.DB, tools *media.Tools, dataDir string) *Indexer {
	ix := &Indexer{
		DB:      d,
		Tools:   tools,
		DataDir: dataDir,
		wake:    make(chan struct{}, 1),
		stop:    make(chan struct{}),
		Workers: max(1, runtime.NumCPU()-1),
	}
	ix.Hub = newHub(ix)
	return ix
}

func (ix *Indexer) Start() error {
	if _, err := ix.DB.Exec(`UPDATE jobs SET status='pending' WHERE status='running'`); err != nil {
		return err
	}
	ix.resolveLibraries()
	w, err := newWatcher(ix)
	if err != nil {
		slog.Warn("file watcher unavailable", "err", err)
	} else {
		ix.watcher = w
	}
	libs, _ := ix.Libraries()
	for _, l := range libs {
		if l.Status == "online" {
			ix.watchLibrary(l)
			go ix.Scan(l.ID)
		}
	}
	ix.startWorkers()
	go ix.Hub.run(ix.stop)
	go ix.maintenance()
	return nil
}

func (ix *Indexer) Stop() {
	close(ix.stop)
	if ix.watcher != nil {
		ix.watcher.close()
	}
	ix.wg.Wait()
}

func (ix *Indexer) Wake() {
	select {
	case ix.wake <- struct{}{}:
	default:
	}
}

func (ix *Indexer) lock(key string) func() {
	m, _ := ix.locks.LoadOrStore(key, &sync.Mutex{})
	mu := m.(*sync.Mutex)
	mu.Lock()
	return mu.Unlock
}

type Library struct {
	ID             int64  `json:"id"`
	Name           string `json:"name"`
	RootPath       string `json:"rootPath"`
	VolumeID       string `json:"volumeId"`
	IsUploadTarget bool   `json:"isUploadTarget"`
	Status         string `json:"status"`
	LastScanAt     *int64 `json:"lastScanAt"`
	Scanning       bool   `json:"scanning"`
	FileCount      int64  `json:"fileCount"`
	volumeRel      string
}

func (ix *Indexer) Libraries() ([]Library, error) {
	rows, err := ix.DB.Query(`SELECT l.id, l.name, l.root_path, l.volume_id, l.volume_rel, l.is_upload_target, l.status, l.last_scan_at,
		(SELECT count(*) FROM files f WHERE f.library_id = l.id AND f.status = 'ok')
		FROM libraries l ORDER BY l.name COLLATE NOCASE`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Library{}
	for rows.Next() {
		var l Library
		if err := rows.Scan(&l.ID, &l.Name, &l.RootPath, &l.VolumeID, &l.volumeRel, &l.IsUploadTarget, &l.Status, &l.LastScanAt, &l.FileCount); err != nil {
			return nil, err
		}
		_, l.Scanning = ix.scanning.Load(l.ID)
		out = append(out, l)
	}
	return out, rows.Err()
}

func (ix *Indexer) Library(id int64) (Library, error) {
	var l Library
	err := ix.DB.QueryRow(`SELECT id, name, root_path, volume_id, volume_rel, is_upload_target, status, last_scan_at FROM libraries WHERE id = ?`, id).
		Scan(&l.ID, &l.Name, &l.RootPath, &l.VolumeID, &l.volumeRel, &l.IsUploadTarget, &l.Status, &l.LastScanAt)
	if errors.Is(err, sql.ErrNoRows) {
		return l, ErrNotFound
	}
	_, l.Scanning = ix.scanning.Load(l.ID)
	return l, err
}

var (
	ErrNotFound = errors.New("not found")
	ErrInvalid  = errors.New("invalid input")
	ErrConflict = errors.New("conflict")
	ErrOffline  = errors.New("library offline")
)

func UserError(err error, msg string) error {
	return fmt.Errorf("%w: %s", err, msg)
}

func checkWritable(dir string) error {
	f, err := os.CreateTemp(dir, ".grimoire-write-test-*")
	if err != nil {
		return err
	}
	name := f.Name()
	f.Close()
	return os.Remove(name)
}

func (ix *Indexer) AddLibrary(name, root string, uploadTarget bool) (Library, error) {
	root = strings.TrimSpace(root)
	if root == "" || !filepath.IsAbs(root) {
		return Library{}, UserError(ErrInvalid, "path must be absolute")
	}
	root = filepath.Clean(root)
	st, err := os.Stat(root)
	if err != nil || !st.IsDir() {
		return Library{}, UserError(ErrInvalid, "path is not a directory")
	}
	if err := checkWritable(root); err != nil {
		return Library{}, UserError(ErrInvalid, "path is not writable")
	}
	if platform.Contains(ix.DataDir, root) || platform.Contains(root, ix.DataDir) {
		return Library{}, UserError(ErrInvalid, "path overlaps the app data folder")
	}
	libs, err := ix.Libraries()
	if err != nil {
		return Library{}, err
	}
	for _, l := range libs {
		if platform.Contains(l.RootPath, root) || platform.Contains(root, l.RootPath) {
			return Library{}, UserError(ErrConflict, "path overlaps library "+l.Name)
		}
	}
	if strings.TrimSpace(name) == "" {
		name = filepath.Base(root)
	}
	volID, volRel := platform.Volume(root)
	res, err := ix.DB.Exec(`INSERT INTO libraries (name, root_path, volume_id, volume_rel, is_upload_target, status, created_at) VALUES (?, ?, ?, ?, ?, 'online', ?)`,
		strings.TrimSpace(name), root, volID, volRel, uploadTarget, db.Now())
	if err != nil {
		return Library{}, err
	}
	id, _ := res.LastInsertId()
	l, err := ix.Library(id)
	if err != nil {
		return l, err
	}
	ix.watchLibrary(l)
	go ix.Scan(id)
	return l, nil
}

func (ix *Indexer) UpdateLibrary(id int64, name *string, uploadTarget *bool, root *string) (Library, error) {
	l, err := ix.Library(id)
	if err != nil {
		return l, err
	}
	if name != nil && strings.TrimSpace(*name) != "" {
		if _, err := ix.DB.Exec(`UPDATE libraries SET name = ? WHERE id = ?`, strings.TrimSpace(*name), id); err != nil {
			return l, err
		}
	}
	if uploadTarget != nil {
		if _, err := ix.DB.Exec(`UPDATE libraries SET is_upload_target = ? WHERE id = ?`, *uploadTarget, id); err != nil {
			return l, err
		}
	}
	if root != nil && *root != l.RootPath {
		p := filepath.Clean(strings.TrimSpace(*root))
		st, err := os.Stat(p)
		if !filepath.IsAbs(p) || err != nil || !st.IsDir() {
			return l, UserError(ErrInvalid, "path is not a directory")
		}
		volID, volRel := platform.Volume(p)
		if _, err := ix.DB.Exec(`UPDATE libraries SET root_path = ?, volume_id = ?, volume_rel = ?, status = 'online' WHERE id = ?`, p, volID, volRel, id); err != nil {
			return l, err
		}
		if ix.watcher != nil {
			ix.watcher.unwatch(l.RootPath)
		}
		nl, _ := ix.Library(id)
		ix.watchLibrary(nl)
		go ix.Scan(id)
	}
	return ix.Library(id)
}

func (ix *Indexer) RemoveLibrary(id int64) error {
	l, err := ix.Library(id)
	if err != nil {
		return err
	}
	if ix.watcher != nil {
		ix.watcher.unwatch(l.RootPath)
	}
	tx, err := ix.DB.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.Exec(`DELETE FROM jobs WHERE file_id IN (SELECT id FROM files WHERE library_id = ?)`, id); err != nil {
		return err
	}
	if _, err := tx.Exec(`DELETE FROM libraries WHERE id = ?`, id); err != nil {
		return err
	}
	if err := deleteOrphans(tx); err != nil {
		return err
	}
	return tx.Commit()
}

func deleteOrphans(tx db.Execer) error {
	_, err := tx.Exec(`DELETE FROM assets WHERE classified_at IS NULL AND title IS NULL AND description IS NULL
		AND NOT EXISTS (SELECT 1 FROM files f WHERE f.asset_id = assets.id)
		AND NOT EXISTS (SELECT 1 FROM asset_tags t WHERE t.asset_id = assets.id)`)
	if err != nil {
		return err
	}
	_, err = tx.Exec(`DELETE FROM assets_fts WHERE rowid NOT IN (SELECT id FROM assets)`)
	return err
}

func (ix *Indexer) resolveLibraries() {
	libs, err := ix.Libraries()
	if err != nil {
		return
	}
	for _, l := range libs {
		status := "online"
		if st, err := os.Stat(l.RootPath); err != nil || !st.IsDir() {
			status = "offline"
			if l.VolumeID != "" {
				if mount := platform.Locate(l.VolumeID); mount != "" {
					cand := platform.Join(mount, l.volumeRel)
					if st, err := os.Stat(cand); err == nil && st.IsDir() {
						slog.Info("library relocated", "library", l.Name, "from", l.RootPath, "to", cand)
						ix.DB.Exec(`UPDATE libraries SET root_path = ? WHERE id = ?`, cand, l.ID)
						status = "online"
					}
				}
			}
		} else if l.VolumeID == "" {
			if volID, volRel := platform.Volume(l.RootPath); volID != "" {
				ix.DB.Exec(`UPDATE libraries SET volume_id = ?, volume_rel = ? WHERE id = ?`, volID, volRel, l.ID)
			}
		}
		if status != l.Status {
			ix.DB.Exec(`UPDATE libraries SET status = ? WHERE id = ?`, status, l.ID)
		}
	}
}

func (ix *Indexer) CheckLibraries() {
	before, _ := ix.Libraries()
	ix.resolveLibraries()
	after, _ := ix.Libraries()
	prev := map[int64]string{}
	for _, l := range before {
		prev[l.ID] = l.Status
	}
	for _, l := range after {
		if l.Status == "online" && prev[l.ID] != "online" {
			ix.watchLibrary(l)
			go ix.Scan(l.ID)
		}
	}
}

func (ix *Indexer) thumbPath(assetID int64) string {
	return filepath.Join(ix.DataDir, "thumbs", fmt.Sprintf("%02x", assetID%256), fmt.Sprintf("%d.webp", assetID))
}

func (ix *Indexer) ThumbPath(assetID int64) string { return ix.thumbPath(assetID) }

func (ix *Indexer) PreviewPath(assetID int64) string {
	return filepath.Join(ix.DataDir, "previews", fmt.Sprintf("%02x", assetID%256), fmt.Sprintf("%d.webp", assetID))
}

func (ix *Indexer) VideoPath(assetID int64) string {
	return filepath.Join(ix.DataDir, "video", fmt.Sprintf("%02x", assetID%256), fmt.Sprintf("%d.mp4", assetID))
}

type Location struct {
	FileID    int64
	LibraryID int64
	Root      string
	Rel       string
	Online    bool
}

func (l Location) Path() string { return platform.Join(l.Root, l.Rel) }

func (ix *Indexer) AssetFile(assetID int64) (Location, error) {
	var loc Location
	var status string
	err := ix.DB.QueryRow(`SELECT f.id, f.library_id, l.root_path, f.rel_path, l.status FROM files f JOIN libraries l ON l.id = f.library_id
		WHERE f.asset_id = ? AND f.status = 'ok' ORDER BY l.status = 'online' DESC, f.id LIMIT 1`, assetID).
		Scan(&loc.FileID, &loc.LibraryID, &loc.Root, &loc.Rel, &status)
	if errors.Is(err, sql.ErrNoRows) {
		return loc, ErrNotFound
	}
	if err != nil {
		return loc, err
	}
	loc.Online = status == "online"
	if !loc.Online {
		return loc, ErrOffline
	}
	return loc, nil
}

func (ix *Indexer) FileLocation(fileID int64) (Location, string, error) {
	var loc Location
	var status, fstatus string
	err := ix.DB.QueryRow(`SELECT f.id, f.library_id, l.root_path, f.rel_path, l.status, f.status FROM files f JOIN libraries l ON l.id = f.library_id WHERE f.id = ?`, fileID).
		Scan(&loc.FileID, &loc.LibraryID, &loc.Root, &loc.Rel, &status, &fstatus)
	if errors.Is(err, sql.ErrNoRows) {
		return loc, "", ErrNotFound
	}
	loc.Online = status == "online"
	return loc, fstatus, err
}
