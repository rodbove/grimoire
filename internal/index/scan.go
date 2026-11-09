package index

import (
	"database/sql"
	"io"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/rodbove/grimoire/internal/db"
	"github.com/rodbove/grimoire/internal/media"
	"github.com/rodbove/grimoire/internal/platform"
	"lukechampine.com/blake3"
)

const (
	prioUpload  = 100
	prioIndex   = 30
	prioExtract = 20
	prioThumb   = 10
	prioHash    = 0
)

type known struct {
	id     int64
	size   int64
	mtime  int64
	status string
	seen   bool
}

func (ix *Indexer) Scan(libID int64) {
	if err := ix.scanTree(libID, ""); err != nil {
		slog.Error("scan failed", "library", libID, "err", err)
	}
}

func likeEscape(s string) string {
	r := strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`)
	return r.Replace(s)
}

func (ix *Indexer) scanTree(libID int64, sub string) error {
	if sub == "" {
		if _, busy := ix.scanning.LoadOrStore(libID, true); busy {
			return nil
		}
		defer ix.scanning.Delete(libID)
	}
	lib, err := ix.Library(libID)
	if err != nil {
		return err
	}
	if st, err := os.Stat(lib.RootPath); err != nil || !st.IsDir() {
		ix.DB.Exec(`UPDATE libraries SET status = 'offline' WHERE id = ?`, libID)
		return nil
	}
	started := time.Now()
	q := `SELECT id, rel_path, size, mtime, status FROM files WHERE library_id = ? AND status IN ('ok', 'pending', 'missing')`
	args := []any{libID}
	if sub != "" {
		q += ` AND (rel_path = ? OR rel_path LIKE ? ESCAPE '\')`
		args = append(args, sub, likeEscape(sub)+"/%")
	}
	rows, err := ix.DB.Query(q, args...)
	if err != nil {
		return err
	}
	existing := map[string]*known{}
	for rows.Next() {
		var rel string
		k := &known{}
		if err := rows.Scan(&k.id, &rel, &k.size, &k.mtime, &k.status); err != nil {
			rows.Close()
			return err
		}
		existing[rel] = k
	}
	rows.Close()

	b := newBatch(ix.DB)
	defer b.close()
	added := 0
	walkRoot := platform.Join(lib.RootPath, sub)
	err = filepath.WalkDir(walkRoot, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			if path == walkRoot {
				return err
			}
			return nil
		}
		if path != walkRoot && platform.Skip(d.Name()) {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if d.IsDir() {
			if path != walkRoot {
				if fi, err := d.Info(); err == nil && platform.IsHiddenSystem(fi) {
					return filepath.SkipDir
				}
			}
			return nil
		}
		if !d.Type().IsRegular() {
			return nil
		}
		if _, _, ok := media.Classify(d.Name()); !ok {
			return nil
		}
		fi, err := d.Info()
		if err != nil || platform.IsPlaceholder(fi) {
			return nil
		}
		rel, err := filepath.Rel(lib.RootPath, path)
		if err != nil {
			return nil
		}
		rel = filepath.ToSlash(rel)
		size, mtime := fi.Size(), fi.ModTime().UnixMilli()
		if k, ok := existing[rel]; ok {
			k.seen = true
			if k.size == size && k.mtime == mtime && k.status == "ok" {
				return nil
			}
		}
		if err := b.upsertFile(ix, libID, rel, size, mtime, prioIndex); err != nil {
			return err
		}
		added++
		if added%200 == 0 {
			ix.Wake()
		}
		return nil
	})
	if err != nil {
		return err
	}
	for _, k := range existing {
		if !k.seen && k.status != "missing" {
			if _, err := b.exec(`UPDATE files SET status = 'missing' WHERE id = ?`, k.id); err != nil {
				return err
			}
		}
	}
	if err := b.commit(); err != nil {
		return err
	}
	ix.cleanupMoved()
	if sub == "" {
		ix.DB.Exec(`UPDATE libraries SET last_scan_at = ?, status = 'online' WHERE id = ?`, db.Now(), libID)
		slog.Info("scan done", "library", lib.Name, "changed", added, "took", time.Since(started).Round(time.Millisecond))
	}
	ix.Wake()
	return nil
}

func (ix *Indexer) cleanupMoved() {
	ix.DB.Exec(`DELETE FROM files WHERE status = 'missing' AND asset_id IN (SELECT asset_id FROM files WHERE status = 'ok' AND asset_id IS NOT NULL)`)
}

type batch struct {
	d  *sql.DB
	tx *sql.Tx
	n  int
}

func newBatch(d *sql.DB) *batch { return &batch{d: d} }

func (b *batch) begin() error {
	if b.tx != nil {
		return nil
	}
	tx, err := b.d.Begin()
	if err != nil {
		return err
	}
	b.tx, b.n = tx, 0
	return nil
}

func (b *batch) exec(q string, args ...any) (sql.Result, error) {
	if err := b.begin(); err != nil {
		return nil, err
	}
	res, err := b.tx.Exec(q, args...)
	if err != nil {
		return nil, err
	}
	b.n++
	if b.n >= 500 {
		if err := b.commit(); err != nil {
			return nil, err
		}
	}
	return res, nil
}

func (b *batch) commit() error {
	if b.tx == nil {
		return nil
	}
	err := b.tx.Commit()
	b.tx = nil
	return err
}

func (b *batch) close() {
	if b.tx != nil {
		b.tx.Rollback()
		b.tx = nil
	}
}

func (b *batch) upsertFile(ix *Indexer, libID int64, rel string, size, mtime int64, prio int) error {
	if err := b.begin(); err != nil {
		return err
	}
	id, err := upsertFile(b.tx, libID, rel, size, mtime)
	if err != nil {
		return err
	}
	if err := enqueue(b.tx, "index", id, 0, prio); err != nil {
		return err
	}
	b.n += 2
	if b.n >= 500 {
		return b.commit()
	}
	return nil
}

func upsertFile(tx db.Execer, libID int64, rel string, size, mtime int64) (int64, error) {
	now := db.Now()
	var id int64
	err := tx.QueryRow(`INSERT INTO files (library_id, rel_path, size, mtime, status, first_seen_at, last_seen_at) VALUES (?, ?, ?, ?, 'pending', ?, ?)
		ON CONFLICT (library_id, rel_path) DO UPDATE SET size = excluded.size, mtime = excluded.mtime, status = 'pending', last_seen_at = excluded.last_seen_at
		RETURNING id`, libID, rel, size, mtime, now, now).Scan(&id)
	return id, err
}

func quickHash(path string, size int64) ([]byte, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	h := blake3.New(32, nil)
	var sz [8]byte
	for i := 0; i < 8; i++ {
		sz[i] = byte(size >> (8 * i))
	}
	h.Write(sz[:])
	const chunk = 1 << 20
	if size <= 2*chunk {
		if _, err := io.Copy(h, f); err != nil {
			return nil, err
		}
		return h.Sum(nil), nil
	}
	if _, err := io.CopyN(h, f, chunk); err != nil {
		return nil, err
	}
	if _, err := f.Seek(size-chunk, io.SeekStart); err != nil {
		return nil, err
	}
	if _, err := io.CopyN(h, f, chunk); err != nil {
		return nil, err
	}
	return h.Sum(nil), nil
}

func fullHash(path string) ([]byte, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	h := blake3.New(32, nil)
	buf := make([]byte, 1<<20)
	if _, err := io.CopyBuffer(h, f, buf); err != nil {
		return nil, err
	}
	return h.Sum(nil), nil
}
