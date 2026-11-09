CREATE TABLE settings (
  key   TEXT PRIMARY KEY,
  value TEXT NOT NULL
);

CREATE TABLE sessions (
  token      TEXT PRIMARY KEY,
  expires_at INTEGER NOT NULL
);

CREATE TABLE libraries (
  id               INTEGER PRIMARY KEY,
  name             TEXT NOT NULL,
  root_path        TEXT NOT NULL,
  volume_id        TEXT NOT NULL DEFAULT '',
  volume_rel       TEXT NOT NULL DEFAULT '',
  is_upload_target INTEGER NOT NULL DEFAULT 0,
  status           TEXT NOT NULL DEFAULT 'online',
  last_scan_at     INTEGER,
  created_at       INTEGER NOT NULL
);

CREATE TABLE assets (
  id              INTEGER PRIMARY KEY,
  hash            BLOB UNIQUE,
  quick_hash      BLOB NOT NULL,
  kind            TEXT NOT NULL,
  mime            TEXT NOT NULL,
  size            INTEGER NOT NULL,
  width           INTEGER,
  height          INTEGER,
  duration_ms     INTEGER,
  video_codec     TEXT,
  taken_at        INTEGER,
  taken_at_source TEXT,
  sort_at         INTEGER NOT NULL,
  camera_make     TEXT,
  camera_model    TEXT,
  gps_lat         REAL,
  gps_lon         REAL,
  title           TEXT,
  description     TEXT,
  location_text   TEXT,
  rating          INTEGER,
  classified_at   INTEGER,
  live_group_id   TEXT,
  motion          INTEGER NOT NULL DEFAULT 0,
  thumb_status    TEXT NOT NULL DEFAULT 'pending',
  created_at      INTEGER NOT NULL,
  updated_at      INTEGER NOT NULL
);

CREATE INDEX assets_sort ON assets(sort_at, id);
CREATE INDEX assets_classified ON assets(classified_at);
CREATE INDEX assets_quick ON assets(quick_hash, size);
CREATE INDEX assets_live ON assets(live_group_id) WHERE live_group_id IS NOT NULL;
CREATE INDEX assets_camera ON assets(camera_model);

CREATE TABLE files (
  id            INTEGER PRIMARY KEY,
  asset_id      INTEGER REFERENCES assets(id) ON DELETE SET NULL,
  library_id    INTEGER NOT NULL REFERENCES libraries(id) ON DELETE CASCADE,
  rel_path      TEXT NOT NULL,
  size          INTEGER NOT NULL,
  mtime         INTEGER NOT NULL,
  status        TEXT NOT NULL,
  first_seen_at INTEGER NOT NULL,
  last_seen_at  INTEGER NOT NULL,
  UNIQUE (library_id, rel_path)
);

CREATE INDEX files_asset ON files(asset_id);
CREATE INDEX files_status ON files(status);

CREATE TABLE tags (
  id   INTEGER PRIMARY KEY,
  name TEXT NOT NULL UNIQUE COLLATE NOCASE
);

CREATE TABLE asset_tags (
  asset_id INTEGER NOT NULL REFERENCES assets(id) ON DELETE CASCADE,
  tag_id   INTEGER NOT NULL REFERENCES tags(id) ON DELETE CASCADE,
  PRIMARY KEY (asset_id, tag_id)
);

CREATE INDEX asset_tags_tag ON asset_tags(tag_id);

CREATE TABLE people (
  id   INTEGER PRIMARY KEY,
  name TEXT NOT NULL UNIQUE COLLATE NOCASE
);

CREATE TABLE asset_people (
  asset_id  INTEGER NOT NULL REFERENCES assets(id) ON DELETE CASCADE,
  person_id INTEGER NOT NULL REFERENCES people(id) ON DELETE CASCADE,
  PRIMARY KEY (asset_id, person_id)
);

CREATE INDEX asset_people_person ON asset_people(person_id);

CREATE TABLE jobs (
  id        INTEGER PRIMARY KEY,
  type      TEXT NOT NULL,
  file_id   INTEGER NOT NULL DEFAULT 0,
  asset_id  INTEGER NOT NULL DEFAULT 0,
  priority  INTEGER NOT NULL DEFAULT 0,
  attempts  INTEGER NOT NULL DEFAULT 0,
  status    TEXT NOT NULL,
  error     TEXT,
  run_after INTEGER NOT NULL DEFAULT 0
);

CREATE UNIQUE INDEX jobs_active ON jobs(type, file_id, asset_id) WHERE status IN ('pending', 'running');
CREATE INDEX jobs_claim ON jobs(status, priority DESC, id);

CREATE TABLE trash (
  id                INTEGER PRIMARY KEY,
  file_id           INTEGER NOT NULL REFERENCES files(id) ON DELETE CASCADE,
  original_rel_path TEXT NOT NULL,
  trash_rel_path    TEXT NOT NULL,
  deleted_at        INTEGER NOT NULL
);

CREATE TABLE uploads (
  id          TEXT PRIMARY KEY,
  library_id  INTEGER NOT NULL REFERENCES libraries(id) ON DELETE CASCADE,
  folder      TEXT NOT NULL,
  name        TEXT NOT NULL,
  size        INTEGER NOT NULL,
  offset      INTEGER NOT NULL DEFAULT 0,
  dup         TEXT NOT NULL,
  result      TEXT,
  created_at  INTEGER NOT NULL
);

CREATE VIRTUAL TABLE assets_fts USING fts5(
  title, description, location, tags, people, filenames, camera,
  tokenize = 'unicode61 remove_diacritics 2'
);
