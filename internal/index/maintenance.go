package index

import (
	"fmt"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/rodbove/grimoire/internal/db"
)

const (
	trashRetention = 30 * 24 * time.Hour
	previewCap     = 5 << 30
	videoCap       = 10 << 30
)

func (ix *Indexer) maintenance() {
	libCheck := time.NewTicker(time.Minute)
	defer libCheck.Stop()
	first := time.NewTimer(2 * time.Minute)
	daily := time.NewTicker(24 * time.Hour)
	defer daily.Stop()
	for {
		select {
		case <-ix.stop:
			return
		case <-libCheck.C:
			ix.CheckLibraries()
		case <-first.C:
			ix.nightly(false)
		case <-daily.C:
			ix.nightly(true)
		}
	}
}

func (ix *Indexer) nightly(rescan bool) {
	if err := ix.BackupDB(); err != nil {
		slog.Warn("db backup", "err", err)
	}
	ix.PurgeOlderThan(trashRetention)
	pruneCache(filepath.Join(ix.DataDir, "previews"), previewCap)
	pruneCache(filepath.Join(ix.DataDir, "video"), videoCap)
	ix.DB.Exec(`DELETE FROM sessions WHERE expires_at < ?`, db.Now())
	ix.DB.Exec(`DELETE FROM uploads WHERE created_at < ?`, time.Now().Add(-7*24*time.Hour).UnixMilli())
	ix.DB.Exec(`DELETE FROM tags WHERE id NOT IN (SELECT tag_id FROM asset_tags)`)
	ix.DB.Exec(`DELETE FROM people WHERE id NOT IN (SELECT person_id FROM asset_people)`)
	if rescan {
		libs, _ := ix.Libraries()
		for _, l := range libs {
			if l.Status == "online" {
				ix.Scan(l.ID)
			}
		}
	}
}

func (ix *Indexer) BackupDB() error {
	libs, err := ix.Libraries()
	if err != nil {
		return err
	}
	now := time.Now()
	name := fmt.Sprintf("grimoire-%s.db", now.Format("20060102"))
	local := filepath.Join(ix.DataDir, "backups", name)
	if err := os.MkdirAll(filepath.Dir(local), 0o755); err != nil {
		return err
	}
	os.Remove(local)
	if _, err := ix.DB.Exec(`VACUUM INTO ?`, local); err != nil {
		return err
	}
	rotate(filepath.Dir(local))
	data, err := os.ReadFile(local)
	if err != nil {
		return err
	}
	for _, l := range libs {
		if l.Status != "online" {
			continue
		}
		dir := filepath.Join(l.RootPath, metaDir, "db-backup")
		if err := os.MkdirAll(dir, 0o755); err != nil {
			continue
		}
		tmp := filepath.Join(dir, name+".tmp")
		if os.WriteFile(tmp, data, 0o644) == nil {
			os.Rename(tmp, filepath.Join(dir, name))
		}
		rotate(dir)
	}
	return nil
}

func rotate(dir string) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}
	var names []string
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), "grimoire-") && strings.HasSuffix(e.Name(), ".db") {
			names = append(names, e.Name())
		}
	}
	sort.Sort(sort.Reverse(sort.StringSlice(names)))
	weekly := 0
	for i, n := range names {
		if i < 7 {
			continue
		}
		t, err := time.Parse("20060102", strings.TrimSuffix(strings.TrimPrefix(n, "grimoire-"), ".db"))
		if err == nil && t.Weekday() == time.Sunday && weekly < 4 {
			weekly++
			continue
		}
		os.Remove(filepath.Join(dir, n))
	}
}

func pruneCache(dir string, limit int64) {
	type entry struct {
		path string
		size int64
		at   time.Time
	}
	var all []entry
	var total int64
	filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		if fi, err := d.Info(); err == nil {
			all = append(all, entry{p, fi.Size(), fi.ModTime()})
			total += fi.Size()
		}
		return nil
	})
	if total <= limit {
		return
	}
	sort.Slice(all, func(i, j int) bool { return all[i].at.Before(all[j].at) })
	for _, e := range all {
		if total <= limit*8/10 {
			break
		}
		if os.Remove(e.path) == nil {
			total -= e.size
		}
	}
}
