package index

import (
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"
	"github.com/rodbove/grimoire/internal/media"
	"github.com/rodbove/grimoire/internal/platform"
)

type watcher struct {
	ix      *Indexer
	w       *fsnotify.Watcher
	mu      sync.Mutex
	pending map[string]*time.Timer
	roots   map[string]int64
}

func newWatcher(ix *Indexer) (*watcher, error) {
	fw, err := fsnotify.NewBufferedWatcher(4096)
	if err != nil {
		return nil, err
	}
	w := &watcher{ix: ix, w: fw, pending: map[string]*time.Timer{}, roots: map[string]int64{}}
	go w.loop()
	return w, nil
}

func (w *watcher) close() { w.w.Close() }

func (ix *Indexer) watchLibrary(l Library) {
	if ix.watcher == nil || l.Status != "online" {
		return
	}
	w := ix.watcher
	w.mu.Lock()
	w.roots[l.RootPath] = l.ID
	w.mu.Unlock()
	go w.addTree(l.RootPath)
}

func (w *watcher) addTree(root string) {
	filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if !d.IsDir() {
			return nil
		}
		if p != root && platform.Skip(d.Name()) {
			return filepath.SkipDir
		}
		if err := w.w.Add(p); err != nil {
			slog.Debug("watch add", "path", p, "err", err)
		}
		return nil
	})
}

func (w *watcher) unwatch(root string) {
	w.mu.Lock()
	delete(w.roots, root)
	w.mu.Unlock()
	for _, p := range w.w.WatchList() {
		if platform.Contains(root, p) {
			w.w.Remove(p)
		}
	}
}

func (w *watcher) library(p string) (int64, string, bool) {
	w.mu.Lock()
	defer w.mu.Unlock()
	for root, id := range w.roots {
		if platform.Contains(root, p) {
			rel, err := filepath.Rel(root, p)
			if err != nil {
				return 0, "", false
			}
			if rel == "." {
				rel = ""
			}
			return id, filepath.ToSlash(rel), true
		}
	}
	return 0, "", false
}

func (w *watcher) loop() {
	for {
		select {
		case ev, ok := <-w.w.Events:
			if !ok {
				return
			}
			w.debounce(ev.Name)
		case err, ok := <-w.w.Errors:
			if !ok {
				return
			}
			slog.Warn("watcher error, rescanning", "err", err)
			w.mu.Lock()
			ids := make([]int64, 0, len(w.roots))
			for _, id := range w.roots {
				ids = append(ids, id)
			}
			w.mu.Unlock()
			for _, id := range ids {
				go w.ix.Scan(id)
			}
		}
	}
}

func (w *watcher) debounce(p string) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if t, ok := w.pending[p]; ok {
		t.Reset(2 * time.Second)
		return
	}
	w.pending[p] = time.AfterFunc(2*time.Second, func() {
		w.mu.Lock()
		delete(w.pending, p)
		w.mu.Unlock()
		w.handle(p)
	})
}

func (w *watcher) handle(p string) {
	libID, rel, ok := w.library(p)
	if !ok || rel == "" {
		return
	}
	for _, part := range strings.Split(rel, "/") {
		if platform.Skip(part) {
			return
		}
	}
	ix := w.ix
	st, err := os.Stat(p)
	if err != nil {
		ix.DB.Exec(`UPDATE files SET status = 'missing' WHERE library_id = ? AND status IN ('ok', 'pending') AND (rel_path = ? OR rel_path LIKE ? ESCAPE '\')`,
			libID, rel, likeEscape(rel)+"/%")
		return
	}
	if st.IsDir() {
		w.addTree(p)
		if err := ix.scanTree(libID, rel); err != nil {
			slog.Warn("subtree scan", "path", p, "err", err)
		}
		return
	}
	if _, _, ok := media.Classify(p); !ok || platform.IsPlaceholder(st) {
		return
	}
	var size, mtime int64
	var status string
	err = ix.DB.QueryRow(`SELECT size, mtime, status FROM files WHERE library_id = ? AND rel_path = ?`, libID, rel).Scan(&size, &mtime, &status)
	if err == nil && status == "ok" && size == st.Size() && mtime == st.ModTime().UnixMilli() {
		return
	}
	tx, err := ix.DB.Begin()
	if err != nil {
		return
	}
	defer tx.Rollback()
	id, err := upsertFile(tx, libID, rel, st.Size(), st.ModTime().UnixMilli())
	if err != nil {
		return
	}
	if enqueue(tx, "index", id, 0, prioIndex+5) != nil {
		return
	}
	if tx.Commit() == nil {
		ix.Wake()
	}
}
