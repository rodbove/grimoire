package index

import (
	"database/sql"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"unicode"

	"github.com/rodbove/grimoire/internal/db"
	"github.com/rodbove/grimoire/internal/platform"
)

type Filter struct {
	Q          string
	Kinds      []string
	Library    int64
	Folder     string
	Direct     bool
	From, To   int64
	Tags       []string
	AnyTags    bool
	People     []string
	Camera     string
	Classified string
	MinRating  int
	Duplicates bool
	Offline    bool
	Exclude    []int64
}

func ftsQuery(q string) string {
	var terms []string
	for _, f := range strings.FieldsFunc(q, func(r rune) bool { return !unicode.IsLetter(r) && !unicode.IsDigit(r) }) {
		terms = append(terms, `"`+f+`"*`)
	}
	return strings.Join(terms, " AND ")
}

func placeholders(n int) string {
	return strings.TrimSuffix(strings.Repeat("?,", n), ",")
}

func (f Filter) where() (string, []any) {
	conds := []string{"a.kind != 'live_video'"}
	var args []any
	fileConds := []string{"f.asset_id = a.id", "f.status = 'ok'"}
	if f.Library > 0 {
		fileConds = append(fileConds, "f.library_id = ?")
		args = append(args, f.Library)
	}
	if folder, ok := platform.CleanRel(f.Folder); ok && folder != "" {
		fileConds = append(fileConds, `f.rel_path LIKE ? ESCAPE '\'`)
		args = append(args, likeEscape(folder)+"/%")
		if f.Direct {
			fileConds = append(fileConds, `f.rel_path NOT LIKE ? ESCAPE '\'`)
			args = append(args, likeEscape(folder)+"/%/%")
		}
	} else if ok && f.Direct {
		fileConds = append(fileConds, `f.rel_path NOT LIKE '%/%'`)
	}
	if f.Offline {
		fileConds = append(fileConds, "l.status = 'offline'")
	}
	conds = append(conds, "EXISTS (SELECT 1 FROM files f JOIN libraries l ON l.id = f.library_id WHERE "+strings.Join(fileConds, " AND ")+")")
	if q := ftsQuery(f.Q); q != "" {
		conds = append(conds, "a.id IN (SELECT rowid FROM assets_fts WHERE assets_fts MATCH ?)")
		args = append(args, q)
	}
	if len(f.Kinds) > 0 {
		var kc []string
		for _, k := range f.Kinds {
			switch k {
			case "live":
				kc = append(kc, "(a.kind = 'live_photo' OR a.motion = 1)")
			case "photo":
				kc = append(kc, "a.kind IN ('photo', 'live_photo')")
			case "video", "document":
				kc = append(kc, "a.kind = ?")
				args = append(args, k)
			}
		}
		if len(kc) > 0 {
			conds = append(conds, "("+strings.Join(kc, " OR ")+")")
		}
	}
	if f.From > 0 {
		conds = append(conds, "a.sort_at >= ?")
		args = append(args, f.From)
	}
	if f.To > 0 {
		conds = append(conds, "a.sort_at < ?")
		args = append(args, f.To)
	}
	if len(f.Tags) > 0 {
		if f.AnyTags {
			conds = append(conds, "EXISTS (SELECT 1 FROM asset_tags at JOIN tags t ON t.id = at.tag_id WHERE at.asset_id = a.id AND t.name IN ("+placeholders(len(f.Tags))+"))")
			for _, t := range f.Tags {
				args = append(args, t)
			}
		} else {
			for _, t := range f.Tags {
				conds = append(conds, "EXISTS (SELECT 1 FROM asset_tags at JOIN tags t ON t.id = at.tag_id WHERE at.asset_id = a.id AND t.name = ?)")
				args = append(args, t)
			}
		}
	}
	for _, p := range f.People {
		conds = append(conds, "EXISTS (SELECT 1 FROM asset_people ap JOIN people p ON p.id = ap.person_id WHERE ap.asset_id = a.id AND p.name = ?)")
		args = append(args, p)
	}
	if f.Camera != "" {
		conds = append(conds, "a.camera_model = ?")
		args = append(args, f.Camera)
	}
	switch f.Classified {
	case "yes":
		conds = append(conds, "a.classified_at IS NOT NULL")
	case "no":
		conds = append(conds, "a.classified_at IS NULL")
	}
	if f.MinRating > 0 {
		conds = append(conds, "a.rating >= ?")
		args = append(args, f.MinRating)
	}
	if f.Duplicates {
		conds = append(conds, "(SELECT count(*) FROM files f2 WHERE f2.asset_id = a.id AND f2.status = 'ok') > 1")
	}
	if len(f.Exclude) > 0 {
		conds = append(conds, "a.id NOT IN ("+placeholders(len(f.Exclude))+")")
		for _, id := range f.Exclude {
			args = append(args, id)
		}
	}
	return strings.Join(conds, " AND "), args
}

type AssetSummary struct {
	ID         int64   `json:"id"`
	Kind       string  `json:"kind"`
	Mime       string  `json:"mime"`
	SortAt     int64   `json:"sortAt"`
	TakenAt    *int64  `json:"takenAt"`
	Width      *int    `json:"width"`
	Height     *int    `json:"height"`
	DurationMs *int64  `json:"durationMs"`
	Title      *string `json:"title"`
	Rating     *int    `json:"rating"`
	Classified bool    `json:"classified"`
	Motion     bool    `json:"motion"`
	Thumb      string  `json:"thumb"`
	Name       string  `json:"name"`
	Version    int64   `json:"v"`
}

const summaryCols = `a.id, a.kind, a.mime, a.sort_at, a.taken_at, a.width, a.height, a.duration_ms, a.title, a.rating, a.classified_at IS NOT NULL, a.motion, a.thumb_status,
	coalesce((SELECT f.rel_path FROM files f WHERE f.asset_id = a.id AND f.status = 'ok' ORDER BY f.id LIMIT 1), ''), a.updated_at`

func scanSummary(rows interface{ Scan(...any) error }) (AssetSummary, error) {
	var s AssetSummary
	err := rows.Scan(&s.ID, &s.Kind, &s.Mime, &s.SortAt, &s.TakenAt, &s.Width, &s.Height, &s.DurationMs, &s.Title, &s.Rating, &s.Classified, &s.Motion, &s.Thumb, &s.Name, &s.Version)
	if i := strings.LastIndex(s.Name, "/"); i >= 0 {
		s.Name = s.Name[i+1:]
	}
	return s, err
}

type Page struct {
	Items  []AssetSummary `json:"items"`
	Next   string         `json:"next"`
	Total  *int64         `json:"total,omitempty"`
}

func (ix *Indexer) Query(f Filter, cursor string, limit int, asc bool) (Page, error) {
	if limit <= 0 || limit > 500 {
		limit = 200
	}
	where, args := f.where()
	page := Page{Items: []AssetSummary{}}
	if cursor == "" {
		var total int64
		if err := ix.DB.QueryRow("SELECT count(*) FROM assets a WHERE "+where, args...).Scan(&total); err != nil {
			return page, err
		}
		page.Total = &total
	} else {
		a, b, ok := strings.Cut(cursor, ":")
		sortAt, err1 := strconv.ParseInt(a, 10, 64)
		id, err2 := strconv.ParseInt(b, 10, 64)
		if !ok || err1 != nil || err2 != nil {
			return page, UserError(ErrInvalid, "bad cursor")
		}
		op := "<"
		if asc {
			op = ">"
		}
		where += fmt.Sprintf(" AND (a.sort_at %s ? OR (a.sort_at = ? AND a.id %s ?))", op, op)
		args = append(args, sortAt, sortAt, id)
	}
	dir := "DESC"
	if asc {
		dir = "ASC"
	}
	rows, err := ix.DB.Query(fmt.Sprintf("SELECT %s FROM assets a WHERE %s ORDER BY a.sort_at %s, a.id %s LIMIT %d", summaryCols, where, dir, dir, limit+1), args...)
	if err != nil {
		return page, err
	}
	defer rows.Close()
	for rows.Next() {
		s, err := scanSummary(rows)
		if err != nil {
			return page, err
		}
		page.Items = append(page.Items, s)
	}
	if len(page.Items) > limit {
		page.Items = page.Items[:limit]
		last := page.Items[limit-1]
		page.Next = fmt.Sprintf("%d:%d", last.SortAt, last.ID)
	}
	return page, rows.Err()
}

func (ix *Indexer) QueryIDs(f Filter, max int) ([]int64, error) {
	where, args := f.where()
	rows, err := ix.DB.Query(fmt.Sprintf("SELECT a.id FROM assets a WHERE %s ORDER BY a.sort_at DESC, a.id DESC LIMIT %d", where, max), args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var ids []int64
	for rows.Next() {
		var id int64
		if rows.Scan(&id) == nil {
			ids = append(ids, id)
		}
	}
	return ids, rows.Err()
}

type FileInfo struct {
	ID        int64  `json:"id"`
	LibraryID int64  `json:"libraryId"`
	Library   string `json:"library"`
	Path      string `json:"path"`
	FullPath  string `json:"fullPath"`
	Size      int64  `json:"size"`
	Status    string `json:"status"`
	Online    bool   `json:"online"`
}

type Asset struct {
	AssetSummary
	Size           int64      `json:"size"`
	VideoCodec     *string    `json:"videoCodec"`
	TakenAtSource  *string    `json:"takenAtSource"`
	CameraMake     *string    `json:"cameraMake"`
	CameraModel    *string    `json:"cameraModel"`
	Lat            *float64   `json:"lat"`
	Lon            *float64   `json:"lon"`
	Description    *string    `json:"description"`
	LocationText   *string    `json:"locationText"`
	ClassifiedAt   *int64     `json:"classifiedAt"`
	LiveVideoID    *int64     `json:"liveVideoId"`
	Hash           string     `json:"hash"`
	Tags           []string   `json:"tags"`
	People         []string   `json:"people"`
	Files          []FileInfo `json:"files"`
}

func (ix *Indexer) Asset(id int64) (Asset, error) {
	var a Asset
	var hash []byte
	row := ix.DB.QueryRow(`SELECT `+summaryCols+`, a.size, a.video_codec, a.taken_at_source, a.camera_make, a.camera_model, a.gps_lat, a.gps_lon,
		a.description, a.location_text, a.classified_at, a.hash,
		(SELECT v.id FROM assets v WHERE v.live_group_id = a.live_group_id AND v.kind = 'live_video' AND a.kind = 'live_photo' LIMIT 1)
		FROM assets a WHERE a.id = ?`, id)
	s := &a.AssetSummary
	err := row.Scan(&s.ID, &s.Kind, &s.Mime, &s.SortAt, &s.TakenAt, &s.Width, &s.Height, &s.DurationMs, &s.Title, &s.Rating, &s.Classified, &s.Motion, &s.Thumb, &s.Name, &s.Version,
		&a.Size, &a.VideoCodec, &a.TakenAtSource, &a.CameraMake, &a.CameraModel, &a.Lat, &a.Lon, &a.Description, &a.LocationText, &a.ClassifiedAt, &hash, &a.LiveVideoID)
	if errors.Is(err, sql.ErrNoRows) {
		return a, ErrNotFound
	}
	if err != nil {
		return a, err
	}
	if i := strings.LastIndex(s.Name, "/"); i >= 0 {
		s.Name = s.Name[i+1:]
	}
	if hash != nil {
		a.Hash = fmt.Sprintf("%x", hash)
	}
	a.Tags = ix.names(`SELECT t.name FROM asset_tags at JOIN tags t ON t.id = at.tag_id WHERE at.asset_id = ? ORDER BY t.name COLLATE NOCASE`, id)
	a.People = ix.names(`SELECT p.name FROM asset_people ap JOIN people p ON p.id = ap.person_id WHERE ap.asset_id = ? ORDER BY p.name COLLATE NOCASE`, id)
	rows, err := ix.DB.Query(`SELECT f.id, f.library_id, l.name, f.rel_path, l.root_path, f.size, f.status, l.status = 'online'
		FROM files f JOIN libraries l ON l.id = f.library_id WHERE f.asset_id = ? AND f.status IN ('ok', 'missing') ORDER BY f.status = 'ok' DESC, f.id`, id)
	if err != nil {
		return a, err
	}
	defer rows.Close()
	a.Files = []FileInfo{}
	for rows.Next() {
		var fi FileInfo
		var root string
		if err := rows.Scan(&fi.ID, &fi.LibraryID, &fi.Library, &fi.Path, &root, &fi.Size, &fi.Status, &fi.Online); err != nil {
			return a, err
		}
		fi.FullPath = platform.Join(root, fi.Path)
		a.Files = append(a.Files, fi)
	}
	return a, rows.Err()
}

func (ix *Indexer) names(q string, id int64) []string {
	out := []string{}
	rows, err := ix.DB.Query(q, id)
	if err != nil {
		return out
	}
	defer rows.Close()
	for rows.Next() {
		var s string
		if rows.Scan(&s) == nil {
			out = append(out, s)
		}
	}
	return out
}

type Patch struct {
	Title        *string   `json:"title"`
	Description  *string   `json:"description"`
	LocationText *string   `json:"locationText"`
	Rating       *int      `json:"rating"`
	TakenAt      *int64    `json:"takenAt"`
	Tags         *[]string `json:"tags"`
	People       *[]string `json:"people"`
	AddTags      []string  `json:"addTags"`
	RemoveTags   []string  `json:"removeTags"`
	AddPeople    []string  `json:"addPeople"`
	RemovePeople []string  `json:"removePeople"`
	Classified   *bool     `json:"classified"`
}

func clean(s *string) any {
	if s == nil {
		return nil
	}
	t := strings.TrimSpace(*s)
	if t == "" {
		return nil
	}
	return t
}

func cleanNames(in []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, n := range in {
		n = strings.Join(strings.Fields(n), " ")
		if n == "" || len(n) > 100 || seen[strings.ToLower(n)] {
			continue
		}
		seen[strings.ToLower(n)] = true
		out = append(out, n)
	}
	return out
}

func (ix *Indexer) ApplyPatch(ids []int64, p Patch) error {
	if len(ids) == 0 {
		return nil
	}
	if p.Rating != nil && (*p.Rating < 0 || *p.Rating > 5) {
		return UserError(ErrInvalid, "rating must be 0 to 5")
	}
	tx, err := ix.DB.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	now := db.Now()
	for _, id := range ids {
		sets := []string{"updated_at = ?"}
		args := []any{now}
		if p.Title != nil {
			sets, args = append(sets, "title = ?"), append(args, clean(p.Title))
		}
		if p.Description != nil {
			sets, args = append(sets, "description = ?"), append(args, clean(p.Description))
		}
		if p.LocationText != nil {
			sets, args = append(sets, "location_text = ?"), append(args, clean(p.LocationText))
		}
		if p.Rating != nil {
			var r any
			if *p.Rating > 0 {
				r = *p.Rating
			}
			sets, args = append(sets, "rating = ?"), append(args, r)
		}
		if p.TakenAt != nil && *p.TakenAt > 0 {
			sets = append(sets, "taken_at = ?", "sort_at = ?", "taken_at_source = 'user'")
			args = append(args, *p.TakenAt, *p.TakenAt)
		}
		if p.Classified != nil {
			if *p.Classified {
				sets = append(sets, "classified_at = coalesce(classified_at, ?)")
				args = append(args, now)
			} else {
				sets = append(sets, "classified_at = NULL")
			}
		}
		args = append(args, id)
		res, err := tx.Exec("UPDATE assets SET "+strings.Join(sets, ", ")+" WHERE id = ?", args...)
		if err != nil {
			return err
		}
		if n, _ := res.RowsAffected(); n == 0 {
			return ErrNotFound
		}
		if err := setNames(tx, "tags", "asset_tags", "tag_id", id, p.Tags, p.AddTags, p.RemoveTags); err != nil {
			return err
		}
		if err := setNames(tx, "people", "asset_people", "person_id", id, p.People, p.AddPeople, p.RemovePeople); err != nil {
			return err
		}
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	for _, id := range ids {
		ix.refreshFTS(id)
	}
	return nil
}

func setNames(tx *sql.Tx, table, link, col string, id int64, replace *[]string, add, remove []string) error {
	if replace != nil {
		if _, err := tx.Exec(fmt.Sprintf("DELETE FROM %s WHERE asset_id = ?", link), id); err != nil {
			return err
		}
		add = append(append([]string{}, *replace...), add...)
	}
	for _, n := range cleanNames(add) {
		if _, err := tx.Exec(fmt.Sprintf("INSERT INTO %s (name) VALUES (?) ON CONFLICT (name) DO NOTHING", table), n); err != nil {
			return err
		}
		if _, err := tx.Exec(fmt.Sprintf("INSERT OR IGNORE INTO %s (asset_id, %s) SELECT ?, id FROM %s WHERE name = ?", link, col, table), id, n); err != nil {
			return err
		}
	}
	for _, n := range cleanNames(remove) {
		if _, err := tx.Exec(fmt.Sprintf("DELETE FROM %s WHERE asset_id = ? AND %s = (SELECT id FROM %s WHERE name = ?)", link, col, table), id, n); err != nil {
			return err
		}
	}
	return nil
}

type NameCount struct {
	Name  string `json:"name"`
	Count int64  `json:"count"`
}

func (ix *Indexer) Suggest(kind, prefix string, limit int) ([]NameCount, error) {
	table, link, col := "tags", "asset_tags", "tag_id"
	if kind == "people" {
		table, link, col = "people", "asset_people", "person_id"
	}
	if limit <= 0 || limit > 100 {
		limit = 20
	}
	rows, err := ix.DB.Query(fmt.Sprintf(`SELECT t.name, count(l.asset_id) c FROM %s t LEFT JOIN %s l ON l.%s = t.id
		WHERE t.name LIKE ? ESCAPE '\' GROUP BY t.id HAVING c > 0 ORDER BY c DESC, t.name COLLATE NOCASE LIMIT ?`, table, link, col),
		likeEscape(strings.TrimSpace(prefix))+"%", limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []NameCount{}
	for rows.Next() {
		var n NameCount
		if rows.Scan(&n.Name, &n.Count) == nil {
			out = append(out, n)
		}
	}
	return out, rows.Err()
}

func (ix *Indexer) Cameras() ([]NameCount, error) {
	rows, err := ix.DB.Query(`SELECT camera_model, count(*) FROM assets WHERE camera_model IS NOT NULL AND camera_model != '' GROUP BY camera_model ORDER BY count(*) DESC LIMIT 50`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []NameCount{}
	for rows.Next() {
		var n NameCount
		if rows.Scan(&n.Name, &n.Count) == nil {
			out = append(out, n)
		}
	}
	return out, rows.Err()
}

func (ix *Indexer) NextUnclassified(f Filter, order string) (*Asset, int64, error) {
	f.Classified = "no"
	where, args := f.where()
	var remaining int64
	if err := ix.DB.QueryRow("SELECT count(*) FROM assets a WHERE "+where, args...).Scan(&remaining); err != nil {
		return nil, 0, err
	}
	if remaining == 0 {
		return nil, 0, nil
	}
	orderBy := "a.sort_at ASC, a.id ASC"
	switch order {
	case "newest":
		orderBy = "a.sort_at DESC, a.id DESC"
	case "random":
		orderBy = "random()"
	case "folder":
		orderBy = "(SELECT f.rel_path FROM files f WHERE f.asset_id = a.id AND f.status = 'ok' ORDER BY f.id LIMIT 1), a.id"
	}
	var id int64
	err := ix.DB.QueryRow("SELECT a.id FROM assets a WHERE "+where+" ORDER BY "+orderBy+" LIMIT 1", args...).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, remaining, nil
	}
	if err != nil {
		return nil, 0, err
	}
	a, err := ix.Asset(id)
	if err != nil {
		return nil, 0, err
	}
	return &a, remaining, nil
}

type FailedJob struct {
	ID      int64  `json:"id"`
	Type    string `json:"type"`
	AssetID int64  `json:"assetId"`
	FileID  int64  `json:"fileId"`
	Error   string `json:"error"`
}

func (ix *Indexer) FailedJobs() ([]FailedJob, error) {
	rows, err := ix.DB.Query(`SELECT id, type, asset_id, file_id, coalesce(error, '') FROM jobs WHERE status = 'failed' ORDER BY id DESC LIMIT 500`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []FailedJob{}
	for rows.Next() {
		var j FailedJob
		if rows.Scan(&j.ID, &j.Type, &j.AssetID, &j.FileID, &j.Error) == nil {
			out = append(out, j)
		}
	}
	return out, rows.Err()
}

func (ix *Indexer) RetryFailed() error {
	_, err := ix.DB.Exec(`UPDATE jobs SET status = 'pending', attempts = 0, run_after = 0 WHERE status = 'failed'
		AND NOT EXISTS (SELECT 1 FROM jobs j2 WHERE j2.type = jobs.type AND j2.file_id = jobs.file_id AND j2.asset_id = jobs.asset_id AND j2.status IN ('pending', 'running'))`)
	ix.Wake()
	return err
}

func (ix *Indexer) Regenerate(assetID int64) error {
	tx, err := ix.DB.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if err := enqueue(tx, "extract", 0, assetID, prioUpload); err != nil {
		return err
	}
	if err := enqueue(tx, "thumb", 0, assetID, prioUpload-1); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	ix.Wake()
	return nil
}
