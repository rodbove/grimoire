package index

import (
	"bytes"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"strings"
	"time"

	"github.com/rodbove/grimoire/internal/db"
	"github.com/rodbove/grimoire/internal/media"
)

func enqueue(tx db.Execer, typ string, fileID, assetID int64, prio int) error {
	_, err := tx.Exec(`INSERT OR IGNORE INTO jobs (type, file_id, asset_id, priority, status, run_after) VALUES (?, ?, ?, ?, 'pending', 0)`,
		typ, fileID, assetID, prio)
	return err
}

type job struct {
	id       int64
	typ      string
	fileID   int64
	assetID  int64
	attempts int
}

func (ix *Indexer) claim(types []string) (*job, error) {
	ph := strings.TrimSuffix(strings.Repeat("?,", len(types)), ",")
	args := []any{db.Now()}
	for _, t := range types {
		args = append(args, t)
	}
	j := &job{}
	err := ix.DB.QueryRow(`UPDATE jobs SET status = 'running', attempts = attempts + 1
		WHERE id = (SELECT id FROM jobs WHERE status = 'pending' AND run_after <= ? AND type IN (`+ph+`) ORDER BY priority DESC, id LIMIT 1)
		RETURNING id, type, file_id, asset_id, attempts`, args...).Scan(&j.id, &j.typ, &j.fileID, &j.assetID, &j.attempts)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	return j, err
}

func (ix *Indexer) startWorkers() {
	general := []string{"index", "extract", "thumb"}
	for i := 0; i < ix.Workers; i++ {
		ix.wg.Add(1)
		go ix.worker(general)
	}
	ix.wg.Add(1)
	go ix.worker([]string{"hash"})
}

func (ix *Indexer) worker(types []string) {
	defer ix.wg.Done()
	idle := time.NewTicker(2 * time.Second)
	defer idle.Stop()
	for {
		select {
		case <-ix.stop:
			return
		default:
		}
		j, err := ix.claim(types)
		if err != nil {
			slog.Warn("claim job", "err", err)
		}
		if j == nil {
			select {
			case <-ix.stop:
				return
			case <-ix.wake:
			case <-idle.C:
			}
			continue
		}
		ix.Wake()
		err = ix.runJob(j)
		if err == nil {
			ix.DB.Exec(`DELETE FROM jobs WHERE id = ?`, j.id)
			continue
		}
		slog.Debug("job failed", "type", j.typ, "file", j.fileID, "asset", j.assetID, "err", err)
		if j.attempts >= 3 {
			ix.DB.Exec(`UPDATE jobs SET status = 'failed', error = ? WHERE id = ?`, err.Error(), j.id)
		} else {
			backoff := time.Duration(j.attempts*j.attempts) * 30 * time.Second
			ix.DB.Exec(`UPDATE jobs SET status = 'pending', error = ?, run_after = ? WHERE id = ?`, err.Error(), time.Now().Add(backoff).UnixMilli(), j.id)
		}
	}
}

func (ix *Indexer) runJob(j *job) error {
	switch j.typ {
	case "index":
		_, err := ix.indexFile(j.fileID, prioExtract)
		return err
	case "extract":
		return ix.extractAsset(j.assetID)
	case "thumb":
		return ix.thumbAsset(j.assetID)
	case "hash":
		return ix.hashAsset(j.assetID)
	}
	return fmt.Errorf("unknown job type %q", j.typ)
}

func (ix *Indexer) indexFile(fileID int64, prio int) (int64, error) {
	loc, status, err := ix.FileLocation(fileID)
	if errors.Is(err, ErrNotFound) {
		return 0, nil
	}
	if err != nil {
		return 0, err
	}
	if !loc.Online || status == "trashed" {
		return 0, nil
	}
	defer ix.lock(fmt.Sprintf("file:%d", fileID))()
	full := loc.Path()
	st, err := os.Stat(full)
	if err != nil {
		ix.DB.Exec(`UPDATE files SET status = 'missing' WHERE id = ?`, fileID)
		return 0, nil
	}
	kind, mime, ok := media.Classify(full)
	if !ok {
		ix.DB.Exec(`DELETE FROM files WHERE id = ?`, fileID)
		return 0, nil
	}
	qh, err := quickHash(full, st.Size())
	if err != nil {
		return 0, err
	}
	tx, err := ix.DB.Begin()
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()
	var current sql.NullInt64
	tx.QueryRow(`SELECT asset_id FROM files WHERE id = ?`, fileID).Scan(&current)
	if current.Valid {
		var cq []byte
		var csize int64
		if tx.QueryRow(`SELECT quick_hash, size FROM assets WHERE id = ?`, current.Int64).Scan(&cq, &csize) == nil && bytes.Equal(cq, qh) && csize == st.Size() {
			if _, err := tx.Exec(`UPDATE files SET status = 'ok', size = ?, mtime = ? WHERE id = ?`, st.Size(), st.ModTime().UnixMilli(), fileID); err != nil {
				return 0, err
			}
			if err := tx.Commit(); err != nil {
				return 0, err
			}
			ix.refreshFTS(current.Int64)
			return current.Int64, nil
		}
	}
	var assetID int64
	err = tx.QueryRow(`SELECT id FROM assets WHERE quick_hash = ? AND size = ? ORDER BY classified_at IS NULL, id LIMIT 1`, qh, st.Size()).Scan(&assetID)
	created := false
	if errors.Is(err, sql.ErrNoRows) {
		now := db.Now()
		res, err := tx.Exec(`INSERT INTO assets (quick_hash, kind, mime, size, sort_at, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?)`,
			qh, kind, mime, st.Size(), st.ModTime().UnixMilli(), now, now)
		if err != nil {
			return 0, err
		}
		assetID, _ = res.LastInsertId()
		created = true
	} else if err != nil {
		return 0, err
	}
	if _, err := tx.Exec(`UPDATE files SET asset_id = ?, status = 'ok', size = ?, mtime = ? WHERE id = ?`, assetID, st.Size(), st.ModTime().UnixMilli(), fileID); err != nil {
		return 0, err
	}
	if _, err := tx.Exec(`DELETE FROM files WHERE asset_id = ? AND status = 'missing'`, assetID); err != nil {
		return 0, err
	}
	if created {
		for _, t := range []struct {
			typ  string
			prio int
		}{{"extract", prio}, {"thumb", prio - 10}, {"hash", prioHash}} {
			if err := enqueue(tx, t.typ, 0, assetID, t.prio); err != nil {
				return 0, err
			}
		}
	}
	if err := tx.Commit(); err != nil {
		return 0, err
	}
	ix.refreshFTS(assetID)
	ix.Wake()
	return assetID, nil
}

func (ix *Indexer) assetKind(assetID int64) (kind, mime string, err error) {
	err = ix.DB.QueryRow(`SELECT kind, mime FROM assets WHERE id = ?`, assetID).Scan(&kind, &mime)
	if errors.Is(err, sql.ErrNoRows) {
		err = ErrNotFound
	}
	return
}

func (ix *Indexer) extractAsset(assetID int64) error {
	kind, mime, err := ix.assetKind(assetID)
	if errors.Is(err, ErrNotFound) {
		return nil
	}
	if err != nil {
		return err
	}
	loc, err := ix.AssetFile(assetID)
	if errors.Is(err, ErrNotFound) || errors.Is(err, ErrOffline) {
		return nil
	}
	if err != nil {
		return err
	}
	m := ix.Tools.Extract(loc.Path(), kind, mime)
	return ix.applyMeta(assetID, kind, m)
}

func nullable[T comparable](v T) any {
	var zero T
	if v == zero {
		return nil
	}
	return v
}

func (ix *Indexer) applyMeta(assetID int64, kind string, m media.Meta) error {
	tx, err := ix.DB.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	_, err = tx.Exec(`UPDATE assets SET
		width = coalesce(?, width), height = coalesce(?, height), duration_ms = coalesce(?, duration_ms), video_codec = coalesce(?, video_codec),
		camera_make = coalesce(?, camera_make), camera_model = coalesce(?, camera_model),
		gps_lat = coalesce(?, gps_lat), gps_lon = coalesce(?, gps_lon), motion = ?,
		taken_at = CASE WHEN taken_at_source = 'user' THEN taken_at ELSE coalesce(?, taken_at) END,
		taken_at_source = CASE WHEN taken_at_source = 'user' THEN taken_at_source WHEN ? IS NULL THEN taken_at_source ELSE ? END,
		live_group_id = coalesce(?, live_group_id),
		updated_at = ?
		WHERE id = ?`,
		nullable(m.Width), nullable(m.Height), nullable(m.DurationMs), nullable(m.Codec),
		nullable(m.Make), nullable(m.Model), m.Lat, m.Lon, m.Motion,
		nullable(m.TakenAt), nullable(m.TakenAt), nullable(m.TakenSource), nullable(m.ContentID), db.Now(), assetID)
	if err != nil {
		return err
	}
	if _, err := tx.Exec(`UPDATE assets SET sort_at = taken_at WHERE id = ? AND taken_at IS NOT NULL`, assetID); err != nil {
		return err
	}
	if m.ContentID != "" {
		if _, err := tx.Exec(`UPDATE assets SET kind = 'live_photo' WHERE live_group_id = ? AND kind = 'photo'
			AND EXISTS (SELECT 1 FROM assets v WHERE v.live_group_id = assets.live_group_id AND v.kind IN ('video', 'live_video'))`, m.ContentID); err != nil {
			return err
		}
		if _, err := tx.Exec(`UPDATE assets SET kind = 'live_video' WHERE live_group_id = ? AND kind = 'video'
			AND EXISTS (SELECT 1 FROM assets p WHERE p.live_group_id = assets.live_group_id AND p.kind IN ('photo', 'live_photo'))`, m.ContentID); err != nil {
			return err
		}
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	ix.refreshFTS(assetID)
	return nil
}

func (ix *Indexer) thumbAsset(assetID int64) error {
	kind, mime, err := ix.assetKind(assetID)
	if errors.Is(err, ErrNotFound) {
		return nil
	}
	if err != nil {
		return err
	}
	loc, err := ix.AssetFile(assetID)
	if errors.Is(err, ErrNotFound) || errors.Is(err, ErrOffline) {
		return nil
	}
	if err != nil {
		return err
	}
	err = ix.Tools.Image(loc.Path(), kind, mime, ix.thumbPath(assetID), 360, true)
	status := "ok"
	if errors.Is(err, media.ErrNoTool) {
		status, err = "none", nil
	} else if err != nil {
		status = "failed"
	}
	ix.DB.Exec(`UPDATE assets SET thumb_status = ?, updated_at = ? WHERE id = ?`, status, db.Now(), assetID)
	return err
}

func (ix *Indexer) hashAsset(assetID int64) error {
	var have []byte
	if err := ix.DB.QueryRow(`SELECT hash FROM assets WHERE id = ?`, assetID).Scan(&have); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil
		}
		return err
	}
	if have != nil {
		return nil
	}
	loc, err := ix.AssetFile(assetID)
	if errors.Is(err, ErrNotFound) || errors.Is(err, ErrOffline) {
		return nil
	}
	if err != nil {
		return err
	}
	h, err := fullHash(loc.Path())
	if err != nil {
		return err
	}
	var other int64
	err = ix.DB.QueryRow(`SELECT id FROM assets WHERE hash = ? AND id != ?`, h, assetID).Scan(&other)
	if errors.Is(err, sql.ErrNoRows) {
		_, err = ix.DB.Exec(`UPDATE assets SET hash = ? WHERE id = ?`, h, assetID)
		return err
	}
	if err != nil {
		return err
	}
	return ix.mergeAssets(other, assetID)
}

func (ix *Indexer) mergeAssets(keep, drop int64) error {
	tx, err := ix.DB.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	stmts := []string{
		`UPDATE assets SET
			title = coalesce(title, (SELECT title FROM assets WHERE id = ?2)),
			description = coalesce(description, (SELECT description FROM assets WHERE id = ?2)),
			location_text = coalesce(location_text, (SELECT location_text FROM assets WHERE id = ?2)),
			rating = coalesce(rating, (SELECT rating FROM assets WHERE id = ?2)),
			classified_at = coalesce(classified_at, (SELECT classified_at FROM assets WHERE id = ?2))
			WHERE id = ?1`,
		`INSERT OR IGNORE INTO asset_tags (asset_id, tag_id) SELECT ?1, tag_id FROM asset_tags WHERE asset_id = ?2`,
		`INSERT OR IGNORE INTO asset_people (asset_id, person_id) SELECT ?1, person_id FROM asset_people WHERE asset_id = ?2`,
		`UPDATE files SET asset_id = ?1 WHERE asset_id = ?2`,
		`DELETE FROM jobs WHERE asset_id = ?2`,
		`DELETE FROM assets WHERE id = ?2`,
		`DELETE FROM assets_fts WHERE rowid = ?2`,
	}
	for _, s := range stmts {
		if _, err := tx.Exec(s, keep, drop); err != nil {
			return err
		}
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	os.Remove(ix.thumbPath(drop))
	os.Remove(ix.PreviewPath(drop))
	ix.refreshFTS(keep)
	return nil
}

func (ix *Indexer) refreshFTS(assetID int64) {
	var title, desc, loc, camera sql.NullString
	err := ix.DB.QueryRow(`SELECT title, description, location_text, trim(coalesce(camera_make, '') || ' ' || coalesce(camera_model, '')) FROM assets WHERE id = ?`, assetID).
		Scan(&title, &desc, &loc, &camera)
	if err != nil {
		return
	}
	join := func(q string) string {
		rows, err := ix.DB.Query(q, assetID)
		if err != nil {
			return ""
		}
		defer rows.Close()
		var parts []string
		for rows.Next() {
			var s string
			if rows.Scan(&s) == nil {
				parts = append(parts, s)
			}
		}
		return strings.Join(parts, " ")
	}
	tags := join(`SELECT t.name FROM asset_tags at JOIN tags t ON t.id = at.tag_id WHERE at.asset_id = ?`)
	people := join(`SELECT p.name FROM asset_people ap JOIN people p ON p.id = ap.person_id WHERE ap.asset_id = ?`)
	names := join(`SELECT rel_path FROM files WHERE asset_id = ? AND status != 'trashed'`)
	names = strings.NewReplacer("/", " ", "_", " ", "-", " ", ".", " ").Replace(names)
	tx, err := ix.DB.Begin()
	if err != nil {
		return
	}
	defer tx.Rollback()
	tx.Exec(`DELETE FROM assets_fts WHERE rowid = ?`, assetID)
	tx.Exec(`INSERT INTO assets_fts (rowid, title, description, location, tags, people, filenames, camera) VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		assetID, title.String, desc.String, loc.String, tags, people, names, camera.String)
	tx.Commit()
}

func (ix *Indexer) RefreshFTS(assetID int64) { ix.refreshFTS(assetID) }
