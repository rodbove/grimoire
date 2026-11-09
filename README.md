# grimoire

A self-hosted media library. It indexes photos, videos and documents in folders that you select, and lets you classify them with a title, description, tags, people, location, date and rating.

The plan is in [PLAN.md](PLAN.md).

## Features

- Libraries: add folders from the UI. Each library is tied to a volume ID, so a changed drive letter does not break it.
- Indexing: scan, file watcher, nightly rescan. Moved and renamed files keep their metadata.
- Duplicate detection with BLAKE3 hashes.
- Thumbnails and previews for JPEG, HEIC, PNG, WebP, RAW (DNG and others), video and PDF.
- iPhone Live Photos and Samsung Motion Photos.
- HEVC video plays in every browser: the server converts it to H.264 when the browser cannot play it.
- Classify mode: one file at a time, with keyboard shortcuts and quick tag pads.
- Bulk classify: select many files, or one event (photos less than 2 hours apart).
- Full text search and filters.
- Resumable uploads (tus).
- Rename, move and delete (to a 30-day trash).
- Nightly database backup to every library.
- Runs as a Windows service or a systemd service.

## Requirements

- Go 1.27 or later.
- Node.js 20 or later.
- Media tools: `vipsthumbnail` and `vipsheader` (libvips), `ffmpeg` and `ffprobe`, `pdftoppm` (Poppler), `exiftool`.

Grimoire looks for the tools in the `bin` folder next to the executable, then in `PATH`. A missing tool disables only the formats that need it. The SETUP page shows which tools it found.

## Build

1. Run `make windows` for a Windows build, `make linux-arm64` for a Raspberry Pi build, or `make build` for this machine.
2. The binary is in `dist/`. The web UI is embedded in it.

## Install on Windows

1. Copy `dist/windows/grimoire.exe` to `C:\Program Files\grimoire\`.
2. Open PowerShell as administrator in the repository.
3. Run `scripts\fetch-tools.ps1 -Dest "C:\Program Files\grimoire\bin"`.
4. Run `& "C:\Program Files\grimoire\grimoire.exe" install`.
5. Run `& "C:\Program Files\grimoire\grimoire.exe" start`.
6. Run `New-NetFirewallRule -DisplayName grimoire -Direction Inbound -Protocol TCP -LocalPort 7777 -Action Allow -Profile Private`.
7. Set the network profile to Private.
8. Set sleep to "Never" when the PC is plugged in.
9. Open `http://<pc-ip>:7777` and set a password.
10. Open LIBRARY and add your folders.

App data (database, thumbnails, logs) is in `%ProgramData%\grimoire`.

## Install on Linux

1. Install the tools: `apt install libvips-tools ffmpeg poppler-utils libimage-exiftool-perl`.
2. Copy the binary to `/usr/local/bin/grimoire`.
3. Run `sudo grimoire install`, then `sudo grimoire start`.

App data is in `/var/lib/grimoire` when it runs as root.

## Flags

| Flag | Default | Description |
| --- | --- | --- |
| `-addr` | `:7777` | Listen address. |
| `-data` | see above | App data folder. |
| `-bin` | `<exe folder>/bin` | Folder with the media tools. |
| `-workers` | CPU count - 1 | Media workers. Use 2 on a Raspberry Pi 4. |
| `-hwenc` | none | ffmpeg hardware H.264 encoder, for example `h264_v4l2m2m` on a Raspberry Pi. |

Commands: `run` (default), `install`, `uninstall`, `start`, `stop`, `restart`.

## Development

1. Run `make dev-server`. The API listens on port 7777.
2. Run `make dev-web` in a second terminal.
3. Open `http://localhost:5173`.

## Keyboard

| Where | Key | Action |
| --- | --- | --- |
| Classify | `Ctrl+Enter` | Save and go to the next file. |
| Classify | `Ctrl+Right` | Skip. |
| Classify | `Ctrl+Left` | Go back. |
| Classify | `1-4`, `Q-R`, `A-F`, `Z-V` | Toggle a quick tag. |
| Classify | `T` / `N` | Go to tags / title. |
| Viewer | `Left` / `Right` | Previous / next file. |
| Viewer | `Ctrl+S` | Save. |
| Viewer | `Ctrl+Enter` | Save and mark as classified. |
| Viewer | `I` | Show or hide the info panel. |
| Grid | `Shift+click`, `Ctrl+click` | Select files. |
