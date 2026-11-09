# Grimoire: Plan

## 1. Goal

Grimoire is a self-hosted app to store, browse, search, and classify photos, videos, and documents.

- It runs on one machine on the home network.
- The user adds directories ("libraries") in the UI.
- The app indexes the files in each library.
- The user adds metadata (title, description, tags, people, location) to each file.
- A classify queue shows files that have no metadata, one at a time.
- The app runs on Windows first. It runs on Linux (Raspberry Pi, arm64) later.

## 2. Scope

### Version 1

- Libraries: add, edit, remove, rescan.
- Indexing: scan, watch for changes, extract EXIF, make thumbnails.
- Browse: virtualized grid, timeline by date, folder view.
- Viewer: full-size photo, video playback, metadata panel.
- Classify: single-file queue and bulk classify.
- Search: full text and filters.
- Upload: drag and drop, resumable, to a chosen library.
- File operations: rename, move, delete (to app trash).
- Auth: one password.
- Runs as a Windows service.

### Later

- Albums.
- Linux/arm64 build and Raspberry Pi hardware.
- Backup and offsite copy.
- XMP sidecar export.
- Map view.
- Optional local AI suggestions.

## 3. Constraints

- Data size: more than 1 TB. Estimate 200,000 to 500,000 files.
- Sources: iPhone and Samsung phones, plus other devices.
- Target hardware later: Raspberry Pi 4. All design choices must work with low CPU and low RAM.
- The app has write permission on library folders.
- The app changes a file only when the user starts an operation (upload, rename, move, delete).

## 4. Stack

| Part | Choice | Reason |
| --- | --- | --- |
| Backend | Go | One binary for Windows and Linux arm64. Low RAM. Good concurrency. |
| HTTP router | `chi` | Small, standard `net/http`. |
| Database | SQLite, `modernc.org/sqlite` | Pure Go, no cgo. Has FTS5. WAL mode. |
| Thumbnails | `vipsthumbnail` (libvips), bundled | Fastest image resize. Supports HEIC, JPEG, PNG, WebP, TIFF. |
| Metadata extract | `exiftool` in `-stay_open` mode, bundled | Best support for HEIC, MOV, MP4, DNG, Samsung and Apple tags. |
| Video | `ffmpeg` and `ffprobe`, bundled | Video thumbnails, duration, codec, preview transcode. |
| PDF | `pdftoppm` (Poppler), bundled | PDF first-page thumbnail. |
| Hash | BLAKE3 | Faster than SHA-256. |
| File watch | `fsnotify` | Uses `ReadDirectoryChangesW` on Windows and `inotify` on Linux. |
| Service | `kardianos/service` | Windows service and systemd unit from the same code. |
| Frontend | SvelteKit, static adapter, TypeScript | Small bundle, fast UI. Embedded in the Go binary with `go:embed`. |

No cgo. Cross-compile from any OS with `GOOS` and `GOARCH`.

## 5. Architecture

```
Browser (LAN)
    |
    v
grimoire.exe (Go)
    |-- HTTP API + static Svelte app
    |-- Auth (session cookie)
    |-- Library manager (add, remove, online/offline)
    |-- Scanner (walk, detect changes)
    |-- Watcher (fsnotify, debounce)
    |-- Job queue (SQLite table, worker pool)
    |     |-- hash
    |     |-- extract metadata (exiftool)
    |     |-- make thumbnail (vipsthumbnail, ffmpeg, pdftoppm)
    |     |-- make video preview (ffmpeg, on demand)
    |-- File operations (upload, rename, move, trash)
    |-- SQLite DB
    |
    v
Library folders (D:\Photos, E:\Phone, ...)
App data (%ProgramData%\grimoire\)
```

### App data layout

```
%ProgramData%\grimoire\
  grimoire.db
  config.toml
  thumbs\ab\cd\<hash>_256.webp
  previews\ab\cd\<hash>_1280.webp
  video\ab\cd\<hash>_720.mp4
  logs\
  bin\ (vipsthumbnail, exiftool, ffmpeg, ffprobe, pdftoppm)
```

On Linux the root is `/var/lib/grimoire/`.

## 6. Data model

Metadata belongs to the content, not to the path. If the same file is in two places, it has one set of metadata.

- **asset**: one unique content (one hash).
- **file**: one location of an asset on disk.

```sql
CREATE TABLE libraries (
  id            INTEGER PRIMARY KEY,
  name          TEXT NOT NULL,
  root_path     TEXT NOT NULL,
  volume_id     TEXT,
  is_upload_target INTEGER NOT NULL DEFAULT 0,
  status        TEXT NOT NULL,
  last_scan_at  INTEGER,
  created_at    INTEGER NOT NULL
);

CREATE TABLE assets (
  id            INTEGER PRIMARY KEY,
  hash          BLOB UNIQUE,
  quick_hash    BLOB NOT NULL,
  kind          TEXT NOT NULL,
  mime          TEXT NOT NULL,
  size          INTEGER NOT NULL,
  width         INTEGER,
  height        INTEGER,
  duration_ms   INTEGER,
  video_codec   TEXT,
  taken_at      INTEGER,
  taken_at_source TEXT,
  camera_make   TEXT,
  camera_model  TEXT,
  gps_lat       REAL,
  gps_lon       REAL,
  title         TEXT,
  description   TEXT,
  location_text TEXT,
  rating        INTEGER,
  classified_at INTEGER,
  live_group_id TEXT,
  thumb_status  TEXT NOT NULL DEFAULT 'pending',
  created_at    INTEGER NOT NULL,
  updated_at    INTEGER NOT NULL
);

CREATE TABLE files (
  id            INTEGER PRIMARY KEY,
  asset_id      INTEGER REFERENCES assets(id),
  library_id    INTEGER NOT NULL REFERENCES libraries(id),
  rel_path      TEXT NOT NULL,
  size          INTEGER NOT NULL,
  mtime         INTEGER NOT NULL,
  status        TEXT NOT NULL,
  first_seen_at INTEGER NOT NULL,
  last_seen_at  INTEGER NOT NULL,
  UNIQUE (library_id, rel_path)
);

CREATE TABLE tags (
  id   INTEGER PRIMARY KEY,
  name TEXT NOT NULL UNIQUE COLLATE NOCASE
);

CREATE TABLE asset_tags (
  asset_id INTEGER NOT NULL REFERENCES assets(id) ON DELETE CASCADE,
  tag_id   INTEGER NOT NULL REFERENCES tags(id) ON DELETE CASCADE,
  PRIMARY KEY (asset_id, tag_id)
);

CREATE TABLE people (
  id   INTEGER PRIMARY KEY,
  name TEXT NOT NULL UNIQUE COLLATE NOCASE
);

CREATE TABLE asset_people (
  asset_id  INTEGER NOT NULL REFERENCES assets(id) ON DELETE CASCADE,
  person_id INTEGER NOT NULL REFERENCES people(id) ON DELETE CASCADE,
  PRIMARY KEY (asset_id, person_id)
);

CREATE TABLE jobs (
  id         INTEGER PRIMARY KEY,
  type       TEXT NOT NULL,
  asset_id   INTEGER,
  file_id    INTEGER,
  priority   INTEGER NOT NULL DEFAULT 0,
  attempts   INTEGER NOT NULL DEFAULT 0,
  status     TEXT NOT NULL,
  error      TEXT,
  run_after  INTEGER NOT NULL
);

CREATE TABLE trash (
  id          INTEGER PRIMARY KEY,
  file_id     INTEGER NOT NULL,
  original_rel_path TEXT NOT NULL,
  trash_rel_path    TEXT NOT NULL,
  deleted_at  INTEGER NOT NULL
);

CREATE VIRTUAL TABLE assets_fts USING fts5(
  title, description, location_text, tags, people, filenames,
  content='', tokenize='unicode61 remove_diacritics 2'
);
```

Indexes:

- `assets(taken_at, id)` for the timeline and keyset pages.
- `assets(classified_at)` for the classify queue.
- `assets(quick_hash)` for move detection.
- `files(asset_id)`.

`kind` values: `photo`, `video`, `live_photo`, `document`, `other`.

## 7. Libraries

### Add a library

1. The user opens Settings > Libraries > Add.
2. The user selects or types a path.
3. The server checks that the path exists and is writable.
4. The server reads the volume GUID (Windows) or filesystem UUID (Linux).
5. The server saves the library and starts a scan.

### Drive letter changes

- The app stores paths relative to the library root.
- The app stores the volume GUID.
- At startup, if a root path is missing, the app looks for the volume GUID on all mounted volumes.
- If the app finds the volume, it updates `root_path`.
- If the app does not find the volume, the library status is `offline`. Metadata stays. Files show an "offline" badge.

### Overlap

- The app refuses a library that is inside another library, or that contains another library.

## 8. Indexing

### Scan

1. Walk the library tree.
2. Skip hidden and system folders, `$RECYCLE.BIN`, `System Volume Information`, and the app trash folder.
3. Skip OneDrive and other cloud placeholder files (attribute `FILE_ATTRIBUTE_RECALL_ON_DATA_ACCESS` or `FILE_ATTRIBUTE_OFFLINE`). Reading them starts a download.
4. For each file, compare `(rel_path, size, mtime)` with the DB.
5. If the values are the same, skip the file.
6. If the file is new or changed, add an `index` job.
7. At the end, mark files that were not seen as `missing`.

### Index job

1. Compute `quick_hash`: BLAKE3 of size + first 1 MB + last 1 MB.
2. Look for a `missing` file with the same `quick_hash` and size. If found, treat as a move: link the new file to the old asset.
3. Else create a new asset.
4. Add `extract`, `thumb`, and `full_hash` jobs.

### Full hash

- Low priority. Runs after all thumbnails are done.
- 1 TB on a USB HDD at about 150 MB/s takes about 2 hours. On SSD it is faster.
- When the full hash matches an existing asset, merge the two assets (duplicate detection).

### Extract job

- Send the file path to the `exiftool -stay_open` process. Read JSON output.
- Date order: `SubSecDateTimeOriginal`, `DateTimeOriginal`, `CreateDate` (QuickTime, UTC), `MediaCreateDate`, then file `mtime`.
- Save `taken_at_source` so the UI can show when the date is a guess.
- Save camera, GPS, size, duration, codec.

### Thumb job

| Type | Tool | Output |
| --- | --- | --- |
| JPEG, HEIC, PNG, WebP, TIFF | `vipsthumbnail` | 256 px WebP |
| DNG and RAW | `exiftool -b -PreviewImage`, then `vipsthumbnail` | 256 px WebP |
| Video | `ffmpeg -ss 1 -frames:v 1`, then `vipsthumbnail` | 256 px WebP |
| PDF | `pdftoppm -f 1 -l 1`, then `vipsthumbnail` | 256 px WebP |

- Grid thumbnails (256 px) are made for all assets. Estimate: 15 KB x 500,000 = 7.5 GB.
- Large previews (1280 px) are made on demand and kept in an LRU cache with a size limit (default 5 GB).

### Watcher

- Watch each library root recursively with `fsnotify`.
- Debounce events for 2 seconds per path. Phones and copy tools write files in many steps.
- If the watcher fails or overflows, start a scan of that library.
- Also run a scheduled scan every night as a safety net.

### Workers

- Worker count: CPU count minus 1 for thumbnails. 1 worker for full hash (disk bound).
- Jobs are in SQLite. A restart continues from the last state.
- Failed jobs retry 3 times with backoff. After that, status is `failed` and the UI lists them.

## 9. Phone formats

### iPhone

- Photos: HEIC. Some JPEG. ProRAW is DNG.
- Videos: MOV, usually HEVC (H.265). Some H.264.
- Live Photos: one HEIC and one short MOV with the same `ContentIdentifier`.
  - The app groups them with `live_group_id`.
  - The grid shows one item. The viewer plays the video on hover or press.
- Edited photos exported from Photos app can have `IMG_E1234.HEIC` and `IMG_1234.HEIC`. The app keeps both as separate assets.

### Samsung

- Photos: JPEG, or HEIC when "High efficiency pictures" is on.
- Videos: MP4, H.264 or HEVC.
- Motion Photos: one JPEG or HEIC with an MP4 at the end of the file (`MotionPhoto_Data`, or `MicroVideo` XMP tags).
  - The app detects the embedded video with exiftool.
  - The viewer extracts the video stream on demand.

### Video playback in the browser

- H.264 MP4 plays in all browsers.
- HEVC plays in Safari and in Chrome or Edge on Windows only when the hardware supports it.
- Plan:
  1. The viewer tries the original file first.
  2. If the browser cannot play it, the viewer asks for a preview.
  3. The server transcodes to H.264 720p with `ffmpeg` and caches the result.
- On Raspberry Pi 4, transcode is slow. The Pi build uses hardware H.264 encode (`h264_v4l2m2m`) or shows "download original".

## 10. Classify

### Single-file queue

- The queue shows assets with `classified_at IS NULL`.
- Order options: oldest first, newest first, by folder, random.
- Filter options: library, folder, kind, date range.
- Layout: large preview on the left, form on the right.
- Form fields: title, description, tags (autocomplete), people (autocomplete), location, rating, date (fix wrong dates).
- The form shows EXIF data and the current path.
- Keyboard:
  - `Ctrl+Enter`: save and go to next.
  - `Ctrl+S`: skip.
  - `Ctrl+Z`: go to previous.
  - `T`: focus tags.
- The header shows the number of files left.

### Bulk classify

- 500,000 files is too many for one at a time.
- The user selects many assets in the grid (click, shift-click, drag box).
- The user applies tags, people, location, or description to all selected assets.
- "Event" grouping: the app groups photos with less than 2 hours between them. The user can select one event and classify it in one step.

### Classified state

- An asset is classified when the user saves it from the queue or bulk form.
- The user can mark it unclassified again.

## 11. Search

- One search box. FTS5 over title, description, location, tags, people, file names.
- Filters:
  - kind (photo, video, live photo, document)
  - library and folder
  - date range
  - tags (all of / any of)
  - people
  - camera
  - classified or unclassified
  - rating
  - duplicates
  - offline
- Results use keyset pages on `(taken_at, id)`. No `OFFSET`.
- The grid is virtualized. It renders only the visible rows.

## 12. File operations

All operations change the disk first, then the DB, in one controlled step. The watcher ignores events that the app made itself.

- **Upload**
  - The user selects the target library and an optional folder.
  - Default folder: `YYYY/MM/` from `taken_at`, or upload date if no EXIF date.
  - Protocol: tus (resumable). Files over 4 GB are common for phone video.
  - The server writes to a temp file in the same library, then renames it into place.
  - If a file with the same full hash exists, the server asks: keep both or skip.
  - Name conflict: add a suffix, for example `IMG_1234 (1).HEIC`.
- **Rename**: same folder, new name. The server refuses invalid Windows names and reserved names (`CON`, `NUL`, ...).
- **Move**: to another folder or another library. Across volumes: copy, verify hash, then delete the source.
- **Delete**: move to `<library root>\.grimoire-trash\`. The trash view lists items. Restore is possible. Items older than 30 days are removed for good.
- **Download**: original file, or a zip of the selection (streamed, no temp file).

## 13. API

All routes are under `/api`. JSON in and out.

```
POST   /auth/login
POST   /auth/logout

GET    /libraries
POST   /libraries
PATCH  /libraries/{id}
DELETE /libraries/{id}
POST   /libraries/{id}/scan

GET    /assets?q=&kind=&tags=&from=&to=&classified=&cursor=&limit=
GET    /assets/{id}
PATCH  /assets/{id}
POST   /assets/bulk
GET    /assets/{id}/thumb
GET    /assets/{id}/preview
GET    /assets/{id}/original
GET    /assets/{id}/video
GET    /assets/events

GET    /classify/next?order=&filters
GET    /classify/count

GET    /tags?prefix=
GET    /people?prefix=

POST   /files/{id}/rename
POST   /files/{id}/move
DELETE /files/{id}
GET    /trash
POST   /trash/{id}/restore

POST   /uploads            (tus)
PATCH  /uploads/{id}       (tus)

GET    /jobs/status
GET    /events             (server-sent events: scan progress, job counts)
```

`/assets/{id}/original` supports HTTP `Range` so videos can seek.

## 14. Security

- One password. Set on first start.
- Password hash: Argon2id.
- Session: HttpOnly cookie, SameSite=Strict, 30-day expiry.
- CSRF: the API accepts state changes only with a custom header.
- Bind to `0.0.0.0:7777`. The installer adds a Windows Firewall rule for the Private profile only.
- No TLS in version 1 (LAN only). For remote access later, use Tailscale or a reverse proxy with TLS.
- All paths from the client are resolved and checked to be inside a library root. No `..` escape.

## 15. Windows setup

1. Install `grimoire.exe` and the `bin\` tools to `C:\Program Files\grimoire\`.
2. Run `grimoire.exe install`. This registers the Windows service.
3. Run `grimoire.exe start`.
4. Set the network profile to Private.
5. Set power options: sleep "Never" when plugged in. A sleeping PC stops the server.
6. Add `%ProgramData%\grimoire\` to Windows Defender exclusions. This makes thumbnail writes faster.
7. Open `http://<pc-ip>:7777` from any device on the LAN.
8. Set the password.
9. Add the first library.

Later: an installer (Inno Setup or WiX) does steps 1 to 4.

## 16. Backup (later)

- RAID is not backup. A delete or a ransomware attack affects all mirrors.
- 3-2-1 rule: 3 copies, 2 different media, 1 offsite.

### Phase 1: protect the DB

- The metadata is the user's work. It does not exist anywhere else.
- Every night, the app runs `VACUUM INTO` to make a DB copy.
- The app writes the copy to `<library root>\.grimoire\db-backup\` on every library. Keep 7 daily and 4 weekly copies.

### Phase 2: protect the files

- Windows: a second drive with a scheduled `robocopy /MIR` is not enough (it copies deletes). Use a tool with versions: `restic` or `Kopia`.
- Linux (Pi): btrfs on data drives, nightly snapshots, `btrfs send` to a backup drive.

### Phase 3: offsite

- `restic` or `Kopia` to Backblaze B2, or to a second machine at another location.
- Estimate for 1.5 TB on B2: about 9 USD per month.

### Phase 4: XMP sidecars

- Optional export of metadata to `.xmp` files next to each original.
- Other tools (Lightroom, digiKam) can read them.
- Metadata stays with the files if the app is not available.

## 17. Raspberry Pi (later)

Hardware notes for the Pi 4 move:

- All drives connect by USB 3.0. Both blue ports share one bus (about 4 Gbps).
- Use a powered USB 3.0 hub or a multi-bay enclosure with its own power supply.
- USB-SATA bridges: use ASMedia ASM1153E or ASM235CM. NVMe enclosures: RTL9210B or JMS583.
- Boot the OS from an SSD, not from an SD card.
- Use Raspberry Pi OS Lite 64-bit.
- Check power with `vcgencmd get_throttled`. The result must be `0x0`.
- Check that drives use UAS with `lsusb -t`.
- Mount drives by UUID in `/etc/fstab` with `noatime,nofail`.

Software changes for the Pi:

- Build with `GOOS=linux GOARCH=arm64`.
- Install `libvips-tools`, `libimage-exiftool-perl`, `ffmpeg`, `poppler-utils` from apt.
- Run as a systemd service (same `kardianos/service` code).
- Library volume ID uses the filesystem UUID.
- Use 2 thumbnail workers. Pi 4 RAM and heat are limits.
- Copy the DB and the thumbnail cache from Windows. Change library root paths in the UI. The volume ID match does the rest when the drives move with the app.

## 18. Project layout

```
grimoire/
  cmd/grimoire/main.go
  internal/
    api/
    auth/
    config/
    db/
      migrations/
    library/
    scanner/
    watcher/
    jobs/
    media/
      exiftool.go
      vips.go
      ffmpeg.go
      pdf.go
    fileops/
    search/
    platform/
      volume_windows.go
      volume_linux.go
      attrs_windows.go
      attrs_linux.go
  web/
    src/
      routes/
        +page.svelte          (grid / timeline)
        classify/
        search/
        libraries/
        trash/
        settings/
      lib/
        api/
        components/
    static/
  bin/                        (bundled tools per OS, not in git)
  scripts/
    fetch-tools.ps1
    fetch-tools.sh
  Makefile
  go.mod
```

## 19. Milestones

1. **Skeleton**: Go server, SQLite migrations, Svelte app embedded, login, Windows service.
2. **Libraries and scan**: add library, walk, `files` and `assets` rows, quick hash, volume GUID.
3. **Media pipeline**: job queue, exiftool, thumbnails for photos, videos, PDFs.
4. **Browse**: virtualized grid, timeline, viewer, original download with Range.
5. **Classify**: single-file queue, tags and people autocomplete, keyboard.
6. **Search**: FTS5, filters, keyset pages.
7. **Bulk and events**: multi-select, bulk edit, event grouping.
8. **Phone formats**: Live Photos, Samsung Motion Photos, HEVC preview transcode.
9. **File operations**: upload (tus), rename, move, trash.
10. **Watcher**: fsnotify, debounce, nightly scan.
11. **DB backup**: nightly `VACUUM INTO` to each library.
12. **Packaging**: Windows installer, Linux arm64 build.

## 20. Open questions

- Does a library always map to one drive, or can one drive have many libraries?
- Is there a maximum size for the thumbnail cache disk?
- Do edited iPhone exports (`IMG_E*`) link to their original as a version?
- Is a map view needed in version 1?
