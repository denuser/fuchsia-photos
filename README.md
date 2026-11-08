<p align="center"><img src="frontend/src/assets/logo.png" alt="Fuchsia Photos logo" width="160"></p>

# Fuchsia Photos

```
       |           _____   _   _    ____   _   _   ____    _____    ___
       |          |  ___| | | | |  / ___| | | | | / ___|  |_   _|  / _ \
      (o)         | |_    | | | | | |     | |_| | \___ \    | |   | |_| |
   .-'/ \'-.      |  _|   | |_| | | |___  |  _  |  ___) |  _| |_  |  _  |
  /  /   \  \     |_|      \___/   \____| |_| |_| |____/  |_____| |_| |_|
 `  ( ||| )  `     ____    _   _    ___    _____    ___    ____
     \|||/        |  _ \  | | | |  / _ \  |_   _|  / _ \  / ___|
      |||         | |_) | | |_| | | | | |   | |   | | | | \___ \
      |||         |  __/  |  _  | | |_| |   | |   | |_| |  ___) |
     ' ' '        |_|     |_| |_|  \___/    |_|    \___/  |____/
```

A self-hosted application for organizing photos, designed to run on a home NAS (Intel N150, 4 cores, 16 GB DDR5).

**Status:** stage 4. Login, user management, upload, processing, the timeline, albums, tags, search, bulk actions, the trash, sharing, folder scans and the preview settings work. The stage 0 benchmark set the preview format and worker count for JPEG photos. This document records the decisions made so far and the roadmap.

## Features

- Photo upload with automatic preview generation
- Organization by tags and albums
- Users managed inside the application, with no self-registration
- Two roles:
  - **Admin** sees and manages everything
  - **User** sees their own photos and whatever is shared with them
- Sharing of photos and albums with all users or with specific logins
- Admin-only folder scan: photos placed in a folder available to the application are imported on demand
- WebDAV upload for mobile devices (a hypothesis, to be validated)

Supported formats in the first version: JPEG, PNG, WebP, HEIC. RAW and video may be added later.

## Decisions

| Topic | Decision | Rationale |
|---|---|---|
| Backend | Go, modular monolith, single binary | Low memory use, simple deployment, `x/net/webdav` for the WebDAV hypothesis |
| Frontend | Vue 3, Vuetify, Vite, Pinia | Project requirement. The virtual-scrolling timeline grid is custom, as Vuetify has no such component |
| Look | PatternFly 6, built on Vuetify: its tokens go into the Vuetify themes, component defaults and one stylesheet. Fuchsia replaces the PatternFly blue. Red Hat fonts, with Inter for Cyrillic, which they lack. A menu in the app bar chooses the color scheme and the contrast mode (default, high contrast, glass), kept in the browser | A clear, consistent design system without rewriting the frontend: PatternFly has no Vue components. A theme belongs to the device, as a phone and a laptop can sit in different light |
| Database | PostgreSQL | Relations and permissions, JSONB for EXIF, search, `pgvector` as a fallback. Keep it on an SSD if the NAS has one |
| Job queue | River (on PostgreSQL), no Redis | A photo and its job are written in one transaction, and there is one service less to run. No official BullMQ client for Go was found |
| API | REST with OpenAPI, SSE for progress | Works well with browsers, files and caching. A monolith needs no internal RPC. gRPC is only reconsidered when an ML service appears |
| Image processing | libvips through govips | libvips is fast and memory-efficient. govips has a long history, a large community, and is not tied to one libvips version |
| Photo metadata | exiftool through go-exiftool | Handles HEIC and camera-specific tags. One long-running process avoids starting Perl for every photo |
| Folder scan | Copy files into the library, leave the source folder untouched | One ingest path for all sources |
| What a scan covers | A folder inside a fixed root, chosen by the administrator, with an owner for the scan. One scan at a time | Folders per user can be imported to their owners. Nothing outside the root is reachable |
| Running a scan | A River job on its own queue with one worker. It records the list of files in the database, then works through it in slices | A scan can take hours, and continues after a restart with correct counts. It takes no worker from photo processing |
| Permanently deleted photos | The row stays with its hash, stripped of everything else, and files are removed. A scan leaves such a photo out, and an upload adds it again as a new photo | Adding files to a folder and scanning it again must not bring back what the owner deleted, or fill the disk again |
| Sharing | Photos and albums, with all users or by login | Sharing an album makes all of its photos visible to the recipient |
| Rights of a recipient | View and download, read-only | A recipient cannot change, tag or pass on another user's photo, or add it to an album. Tags and albums of shared photos can be reconsidered later |
| Timeline contents | The user's photos and, by default, every processed photo of other users that the user can see, including the photos of shared albums. A "Mine / All" switch above the timeline changes this and is saved for the user. Only the user's own photos can be selected, and others open read-only in the viewer. The camera and lens filters cover all of them. A tag filter finds only the user's own photos, as tags are personal. An administrator's timeline follows the same rule, and all users' photos stay on the administrator's page | A user who had uploaded nothing saw an empty start page, even with photos shared with them. The switch sits on the page rather than in the profile, because one large shared album can push the user's own photos aside |
| "Shared with me" | Albums and photos tabs. Each user chooses whether the photos tab includes the photos of shared albums, which is off by default | One large album would otherwise push everything else out of the list |
| Choosing a recipient | Autocomplete from the logins of active users, available to every logged-in user | Convenient and free of typos. On a home NAS, users knowing each other's logins is not a concern |
| Viewer source | The viewer preview. Each user can choose in the profile to load the original when zooming in, which is off by default; otherwise zooming enlarges the preview. An original the browser cannot decode, such as HEIC outside Safari, falls back to the preview | Originals can be many times larger than previews, which matters on a phone or a slow connection. Only zooming needs more than a preview has |
| Photo details in the viewer | Each user chooses in the profile which camera details and whether the location the viewer shows. All are shown by default. The interface hides them, and the API still returns them | A display preference, not a privacy setting: whether locations are returned at all stays with the administrator |
| Zooming in the viewer | A photo larger than the screen zooms with a click, the wheel or a pinch to the resolution of its original when the user chose in the profile to load originals for zooming and the browser can decode it, and otherwise to the resolution of the viewer preview, past which the photo would only blur. A pinch scales with the distance between the fingers and keeps the point between them in place. The viewer handles the gestures itself, without a library | `@panzoom/panzoom` was used first. Its pinch zoomed linearly with the finger distance, too slowly for large photos, and its zoom point assumed a photo in the corner of its container, so a centred photo zoomed in the wrong place or off the screen |
| Timeline loading | Lists sorted by date load month by month. The server first counts the photos of each month in the browser's time zone. The grid lays out every month from these counts and loads the photos of the months near the screen, at most three at a time. The trash, sorted by deletion time, loads page by page as it scrolls | The scroll bar covers the whole library, and any year is reachable without loading everything newer first |
| Timeline navigation | Month titles in the grid and a scrubber with the years on the right. On a touch screen the scrubber shows while the list scrolls; with a mouse it stays | The right edge is where the thumb is and where phones put such a scrubber |
| Grid tiles | Rows that keep each photo's shape by default. Each user can switch to squares in the profile. A square is as many screen pixels as the grid preview size, and a little smaller so that whole squares fill a row | One size for every photo, set by the grid preview size and the same on any screen density. The grid preview size is the shortest side, so a square cut from a preview keeps full resolution |
| Menu on a phone | Hidden until the menu button opens it over the page. Each user can keep the permanent rail in the profile instead | The rail takes a sixth of a phone's width |
| Preview settings | Stored in the database and edited on an administration page. They apply to new photos only. The `render-grid` command makes the grid previews of all photos again, from their viewer previews, with the current size and quality, and in the format each photo already has. Purely technical settings, such as the number of workers, stay in the environment | Regenerating all previews takes hours and needs its own progress and safeguards. Grid previews alone are cheap to make from viewer previews, and they are what a change of the grid size affects |
| Grid preview size | The shortest side of the preview, never more than the viewer preview has. A preview's address carries a version that changes when it is made again | Square tiles crop the long side, so the short side must have the full size. A 3:2 grid preview has 1.5 times as many pixels as one limited by the long side. Previews are cached for a year, so without the version a browser would keep the old one |
| Administrator | A page with all users' photos and an owner filter, where the administrator can change sharing and move photos to the trash. Other users' albums are read-only and tags stay hidden. Restoring and permanently deleting stay with the owner | An administrator cannot lose someone's photos by accident |
| Tags | Personal to each user. A search with several tags finds photos with any of them | Other users' tag names stay private |
| Camera and lens filters | The timeline filters its photos by camera and by lens, choosing from those its photos have. Several cameras or several lenses find photos with any of them; different filters must all match. Each list counts its photos with the other filters, including tags and dates, and leaves out names with none, except those already chosen. A camera is named by its make and model, without the make when the model repeats it. A lens is LensModel, or Lens or LensID for older cameras, leaving out numeric codes. PostgreSQL keeps both in generated columns of `photos`, and the viewer shows the same lens | Photos processed before the filters get them without reprocessing, and the filter and the viewer cannot disagree |
| Album covers | The newest processed photo, unless the owner chooses one | Works without effort, and can still be set by hand |
| Trash | Photos stay until the owner restores or permanently deletes them, with no automatic expiry | Nothing is lost by waiting. Emptying the trash is the way to free disk space |
| Uploading a photo that is in the trash | Restores it, with its albums and tags | Uploading it again shows that the owner wants it back |
| TLS and HTTP/2 | Caddy in front of the application | The grid loads hundreds of previews, and HTTP/1.1 allows few connections. WebDAV Basic Auth requires TLS |

The main hardware constraints are CPU time for decoding photos and, probably, HDD throughput. Memory is not scarce.

## Architecture

All sources (browser, folder scan, WebDAV) go through a single `Ingest(file, owner)` path:

1. Stream the file to a temporary file on the same filesystem, computing SHA-256 along the way.
2. Check for duplicates with `UNIQUE (owner_id, sha256)`. Deduplication is per user only.
3. Atomically rename the file to `originals/{owner_id}/{sha[:2]}/{sha}.{ext}`. Originals are never modified.
4. In one transaction, create the photo record and a `process_photo` River job.
5. A worker reads EXIF, applies auto-rotation and sRGB conversion, generates a grid preview (400 px on the shortest side) and a viewer preview (2048 px on the longest side) on the SSD. Until a preview loads, the grid shows a neutral tile. Previews are WebP: in the [benchmark](docs/benchmark.md#results) they take about 45% less disk space than JPEG, and the slower encoding is a one-time background cost. The format, quality and sizes can be changed on the administration page, and apply to new photos. AVIF is ruled out because of its encoding cost.
6. A periodic job removes abandoned uploads and expired sessions. It leaves originals without a database record alone: after restoring an older database backup, those are the photos uploaded since that backup. A stray original is harmless, and uploading the same photo again reuses it.
7. Permanently deleting a photo from the trash removes its original and previews, and leaves a record with only its hash. A database lock on the owner and hash keeps this from colliding with an upload of the same file.

A folder scan lists the files in the chosen folder with their sizes and modification times, waits, and passes each file to ingest if neither has changed. It hashes a file before copying it, so a repeat scan reads everything but writes only what is new. Duplicates in the trash stay there, unlike with an upload.

Queue behavior:

- 3 workers, leaving one core for the UI
- Bulk imports run at low priority
- Processing resumes where it stopped after a restart

### Repository layout

This is a monorepo. The target layout is:

```
api/openapi.yaml        REST contract, the source of truth for both sides
backend/                Go module
  cmd/fuchsia-photos/   entry point (server + workers, admin creation command)
  cmd/imgbench/         performance benchmark
  internal/             auth, users, photos, ingest, media, albums, tags, sharing, scan, webdav, jobs, api
  db/migrations, db/queries   goose + sqlc
frontend/               Vue + Vuetify, TypeScript client generated from OpenAPI
deploy/                 Dockerfile, compose.yaml (app, postgres, caddy), Caddyfile
docs/                   development, deployment and benchmark guides
```

The built frontend is embedded in the binary with `go:embed`. The image is built for `linux/amd64` with Docker buildx, because cgo and libvips rule out simple cross-compilation from macOS. It includes libvips, libheif and exiftool.

### Data model

- `users` (login, argon2id password hash, admin/user role, disabled flag, whether "Shared with me" includes album photos, whether zooming in the viewer loads originals, which photo details it hides, whether the menu hides on a phone, whether the grid shows squares, whether the timeline includes shared photos), `sessions`
- `photos` (`bigint` id, owner, SHA-256, MIME type, size, dimensions, `taken_at`, EXIF as JSONB, the camera and lens generated from it, status, preview format and version, private/all visibility, `deleted_at`, `purged_at`). A `bigint` id also fits future FAISS ids (int64).
- `photo_shares`, `albums` (with private/all visibility), `album_photos`, `album_shares`
- `tags` (owner, name), `photo_tags`
- `preview_settings` (one row: format, quality and preview sizes), `metadata_settings` (one row: whether the API returns locations)
- `import_runs` (scan report: found, added, duplicates, in the trash, permanently deleted, unsupported, still changing, failed), `import_files` (files still to process, and the ones to report)

### Access control

Permissions are checked in one place: a single SQL predicate, "this photo is visible to this user", used by every query, including file delivery. A photo is visible if any of these is true:

- The user is an admin or the photo's owner
- The photo is visible to all users
- The photo, or an album containing it, is shared with the user or visible to all users

A photo in the trash is visible only to its owner. Its sharing comes back when it is restored.

Authentication uses sessions in HttpOnly cookies. There is no registration: the first admin is created with a CLI command, and the admin manages all other users.

Previews contain no EXIF. The API returns only the camera details: camera, lens, focal length, aperture, shutter speed, ISO, exposure compensation and flash. It returns the GPS location only if an administrator turns it on, and then to everyone who can see the photo. Originals keep all EXIF, including GPS data, and recipients can download them. The share dialog warns about this.

Revoking access takes effect on the server at once. Previews are cached for a year as private, so a browser that already loaded one can still show it from its cache.

## Roadmap

- **Stage 0: skeleton and benchmarks.**
  - [x] Monorepo structure, compose, migrations, CI
  - [x] Choose the libvips binding
  - [x] Benchmark on the real NAS: JPEG photos, previews on the SSD, WebP and JPEG, 1 to 3 workers. HEIC was left out as not currently relevant
- [x] **Stage 1: core.** Login, user management, roles. Browser upload (3–4 files in parallel, retrying failed files). Ingest, queue, previews, virtual-scrolling timeline, viewer, processing progress.
- [x] **Stage 2: organization.** Albums, tags, search by date and tag, bulk actions, trash with soft delete.
- [x] **Stage 3: sharing.** Visibility for all users, sharing photos and albums by login, a "Shared with me" section, the admin view of everything. Test suite for the permission predicate.
- [x] **Stage 4: folder scan.** Admin only: fixed root, owner selection, safe re-runs thanks to hashing. Skips files still being copied (size and mtime unchanged for N seconds). Report in `import_runs`. A re-run must not restore photos the owner moved to the trash, unlike a browser upload.
- [ ] **Stage 5: operations.** Nightly `pg_dump`, backup and tested restore instructions, logs and queue metrics.
- [ ] **Stage 6: WebDAV (hypothesis, starts as a spike).** Per-user upload folder, separate app password, files go through ingest. Tested with real mobile clients such as PhotoSync and FolderSync. If they work poorly, the feature is dropped and the outcome recorded here.
- [ ] **Stage 7: ML (later).** An `embed_photo` job sends a preview to an external server. The NAS stays the source of truth, and a FAISS index can be rebuilt. Search results are filtered by the permission predicate. The protocol (gRPC or HTTP) and the choice of FAISS or `pgvector` are decided at this stage.

## Acceptance criteria

- **Benchmark (stage 0):** a benchmark report with real speeds on the NAS replaces the estimates under [Expected performance](#expected-performance).
- **Backend:** `go test ./...` passes against a real PostgreSQL. Required scenarios: the permission matrix, duplicates, and a worker failure followed by a job retry.
- **End to end on the NAS:** after `docker compose up`, upload 1,000 photos from the browser. The UI stays responsive, a restart during processing loses no jobs, and every photo ends up with previews or a clear error.
- **Permissions (stage 3):** a second user gets a 404 for another user's original and previews by direct URL, and gets the files once they are shared.
- **Folder scan (stage 4):** a second run on the same folder creates no new records, and the source folder stays unchanged.
- **Backup (stage 5):** a dump restores on a clean installation.

## Expected performance

Measured on the NAS with JPEG photos, WebP previews on the SSD and 3 workers, at 3.0 photos/s. See the [benchmark results](docs/benchmark.md#results).

| Photos | Time |
|---|---|
| 1,000 | about 5.5 min |
| 10,000 | about 56 min |

The real pipeline is somewhat slower, because these numbers leave out EXIF extraction and database writes. HEIC has not been measured, and takes longer to decode.

For tens of thousands of photos, prefer copying them to the NAS and running a folder scan over uploading through the browser.

## Documentation

- [Development](docs/development.md): running locally, backend, migrations, frontend and test commands
- [Deployment](docs/deployment.md): installing and operating on the NAS
- [Benchmark](docs/benchmark.md): measuring performance on the NAS
- [Improvements](docs/improvements.md): the current list of improvements

## License

[MIT](LICENSE)
