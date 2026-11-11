# Architecture

The decisions made so far and how the application is built. What is planned is in the [roadmap](roadmap.md).

## Decisions

| Topic | Decision | Rationale |
|---|---|---|
| Backend | Go, modular monolith, single binary | Low memory use, simple deployment, `x/net/webdav` for WebDAV uploads |
| Frontend | Vue 3, Vuetify, Vite, Pinia | Project requirement. The virtual-scrolling timeline grid is custom, as Vuetify has no such component |
| Look | PatternFly 6, built on Vuetify: its tokens go into the Vuetify themes, component defaults and one stylesheet. Fuchsia replaces the PatternFly blue. Red Hat fonts, with Inter for Cyrillic, which they lack. Font Awesome Free icons, bundled one SVG at a time. A menu in the app bar chooses the color scheme and the contrast mode (default, high contrast, glass), kept in the browser | A clear, consistent design system without rewriting the frontend: PatternFly has no Vue components. A theme belongs to the device, as a phone and a laptop can sit in different light |
| Database | PostgreSQL | Relations and permissions, JSONB for EXIF, search, `pgvector` for photo embeddings. Keep it on an SSD if the NAS has one |
| Job queue | River (on PostgreSQL), no Redis | A photo and its job are written in one transaction, and there is one service less to run. No official BullMQ client for Go was found |
| API | REST with OpenAPI, SSE for progress | Works well with browsers, files and caching. A monolith needs no internal RPC. The ML service is called over HTTP too: its requests are few and large, so gRPC would gain nothing |
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
| WebDAV contents | Each user has a folder tree of their own, kept in the database. A file in it records an upload and points to the photo it became; the file itself is not kept, as the library has the original. A GET returns that original | Sync clients compare the server's listing with the phone to decide what to send. An empty folder would make them send everything again on every sync |
| WebDAV passwords | App passwords, any number per user, named after the device. The server makes a random one, shows it once and stores its SHA-256. Each can be revoked on its own. The account password does not work over WebDAV | Basic Auth sends the password with every request, too often for argon2id, and a random password needs no slow hash. A lost phone is revoked without logging out the others |
| Deleting over WebDAV | Removes the file from the WebDAV tree only. The photo stays in the library | Sync apps delete on the server what is gone from the phone, and clearing space on a phone must not delete its photos here |
| WebDAV and the trash | As with a folder scan: a photo in the trash stays there, and a permanently deleted one is not added again. Files that are not supported photos, such as videos, are refused with 415 | A phone that sends its camera roll again gives no sign that the owner wants a deleted photo back |
| WebDAV albums | The owner links a folder of their WebDAV tree to one of their albums in the profile, and photos uploaded into that folder or its subfolders are added to the album. The nearest linked folder decides, so a subfolder can have an album of its own. This includes a photo the library already has, unless it is in the trash. A link names a path: it can be made before a sync app creates the folder, and it stays when the folder is deleted or moved. Folders are linked, not app passwords. Folder names never create albums. Deleting or moving a file over WebDAV leaves the photo in its album | A sync app pairs each phone folder with a server folder, so the camera and a messenger can go to different albums with nothing set up on the phone. Albums made from folder names would fill the list with `DCIM` and `Screenshots`. Removing photos from an album on delete would contradict keeping them in the library. A link per app password would send everything from a phone to one album, or need a password per folder pair |
| Database backups | A `backup` container on the `postgres:18` image dumps the database every night with `pg_dump` and keeps the latest 14 dumps. Originals are backed up as folder snapshots, such as ZFS snapshots on TrueNAS. Previews are not backed up: the `render-previews` command makes them again from the originals | `pg_dump` always matches the server version, the application image stays as it is, and the setup stays a single YAML on TrueNAS. Previews take hours to remake, but nothing is lost without them |
| Restoring a dump | Restoring moves the photo numbers a million past the dump's | Browsers cache previews for a year by photo number. Photos uploaded after the dump are lost from the database, and a new photo with the number of one of them would show its cached preview |
| Logs | Plain text by default, or JSON, on the container output, at a level set in the environment. Each request is logged, at `info` when it fails or takes over a second and at `debug` otherwise. Docker keeps five files of 10 MB | Errors and slow requests are visible without the noise of every preview |
| Queue metrics | A section of the administration page: jobs by kind and state, finished and failed in the last hour and day, and the latest errors. It updates through server-sent events, with the server reading the queue every 2 seconds and sending only changes | Visible without a monitoring system. SSE as for processing progress. River does not report finished jobs, so the server polls its table |
| ML service | A separate stateless service in Python that turns a viewer preview or a search text into a vector. It has no access to the database: the application calls it, stores the vectors and checks permissions. Its address and a token are set in the environment. It runs on a Mac first, natively with PyTorch on the GPU, and can move to a cloud GPU without recomputing vectors | The service can run anywhere, including outside the home network, with no database credentials. Docker on macOS has no GPU access. The same model weights and preprocessing give the same vectors on any hardware |
| ML availability | The application asks the service for its health every 30 seconds and keeps its state: unavailable, available, or running a model other than the configured one. Embedding runs only while it is available. The interface disables text search while the service is down, and the administration page shows the state, the model and the progress | The test machine is often switched off, and the library must work without it. A different model would mix incomparable vectors |
| Embedding queue | No job per photo. A periodic job on its own queue with one worker takes processed photos that have no vector for the configured model, in batches, and stops when none are left or the service goes away. A photo that fails three times is skipped and listed on the administration page | Per-photo jobs would fail or pile up while the service is off for days. New photos, a first run over the library and a change of model need nothing extra: they all show up as photos without a vector |
| Embedding storage | `pgvector` in the main database, half-precision vectors keyed by photo and model. No FAISS. Search starts with an exact scan, and an HNSW index per model is added only if it gets slow. The database runs on `pgvector/pgvector:pg18-trixie`, and the `backup` container stays on `postgres:18` | The permission predicate and the vector search run in one query. Filtering FAISS results afterwards would leave a user who sees a small part of the library with few or no results. A home library is small enough for an exact scan. The `trixie` image is built on the same Debian release as `postgres:18`, so an existing database keeps its collations; the plain `pg18` tag is an older release with an older glibc |
| Interface language | English and Russian, with vue-i18n and the messages in TypeScript files, one per language. Each user chooses the language in the profile. The default is automatic: the browser's language, or English when the application does not support it. The choice is stored with the user. Dates, numbers, Vuetify's own texts and place names follow the chosen language. The API keeps its errors in English, and the interface shows its own text for the ones it knows | The users of one library may prefer different languages. The browser's language is the best guess before a user chooses. vue-i18n is the usual choice for Vue, has Russian plural forms through a rule, and Vuetify has an adapter for it. TypeScript messages let the type check catch a missing translation or a mistyped key |
| Place names (planned) | The viewer shows the name of the place in place of the coordinates, linked to the map as the coordinates are now. The names come from the GeoNames `cities1000` dump (CC BY 4.0) in a database table: processing finds the populated place nearest to the photo's location, and a periodic job does the same for photos processed before. The photo keeps the place's identifier, and the name is shown in the interface language, or in English when GeoNames has no name in it. A place name is location data: the API returns it only when an administrator allows locations, and the viewer hides it with the location | Coordinates never leave the NAS, and no internet is needed. The public Nominatim would receive the location of every photo, allows one request per second and discourages bulk lookups. A self-hosted Nominatim or Photon needs gigabytes of map data. The cost is accuracy: the name is of a town or village, not a street, and outside towns the nearest village can be a few kilometres away |
| TLS and HTTP/2 | Caddy in front of the application | The grid loads hundreds of previews, and HTTP/1.1 allows few connections. WebDAV Basic Auth requires TLS |

The main hardware constraints are CPU time for decoding photos and, probably, HDD throughput. Memory is not scarce.

## Ingest and processing

All sources (browser, folder scan, WebDAV) go through a single `Ingest(file, owner)` path:

1. Stream the file to a temporary file on the same filesystem, computing SHA-256 along the way.
2. Check for duplicates with `UNIQUE (owner_id, sha256)`. Deduplication is per user only.
3. Atomically rename the file to `originals/{owner_id}/{sha[:2]}/{sha}.{ext}`. Originals are never modified.
4. In one transaction, create the photo record and a `process_photo` River job.
5. A worker reads EXIF, applies auto-rotation and sRGB conversion, generates a grid preview (400 px on the shortest side) and a viewer preview (2048 px on the longest side) on the SSD. Until a preview loads, the grid shows a neutral tile. Previews are WebP: in the [benchmark](benchmark.md#results) they take about 45% less disk space than JPEG, and the slower encoding is a one-time background cost. The format, quality and sizes can be changed on the administration page, and apply to new photos. AVIF is ruled out because of its encoding cost.
6. A periodic job removes abandoned uploads and expired sessions. It leaves originals without a database record alone: after restoring an older database backup, those are the photos uploaded since that backup. A stray original is harmless, and uploading the same photo again reuses it.
7. Permanently deleting a photo from the trash removes its original and previews, and leaves a record with only its hash. A database lock on the owner and hash keeps this from colliding with an upload of the same file.

A folder scan lists the files in the chosen folder with their sizes and modification times, waits, and passes each file to ingest if neither has changed. It hashes a file before copying it, so a repeat scan reads everything but writes only what is new. Duplicates in the trash stay there, unlike with an upload.

WebDAV serves each user's tree at `/webdav/`. `x/net/webdav` handles listings, folders, moves, deletes and locks over the tree in the database. Uploads and downloads bypass it: a PUT streams the file into ingest, which treats the trash as a folder scan does, and records the file with its size and the modification time the client sends in `X-OC-Mtime`. If the file's folder, or a folder above it, is linked to an album, the photo is added to the album of the nearest one before the file is recorded, so that a failure makes the client send the file again. The server logs every WebDAV request with its client, to see how sync apps behave.

Queue behavior:

- 3 workers, leaving one core for the UI
- Bulk imports run at low priority
- Processing resumes where it stopped after a restart

## Machine learning

The ML service has three endpoints: `GET /health` returns the model, the vector size and the preprocessing version; `POST /embed/image` takes a batch of viewer previews; `POST /embed/text` takes a search text. The model name stored with a vector includes the preprocessing version, as resizing and normalization happen in the service.

- **Embedding:** while the service is available, a periodic job on the `ml` queue selects processed photos without a vector for the configured model, sends their viewer previews in batches and writes the vectors. It starts every minute and as soon as the service becomes available.
- **Text search:** a page of its own. The application gets the vector of the text from the service, then sorts the photos the user can see by their distance to it and leaves out those below the model's similarity threshold, which the service gives with the vector. Without the threshold a query with no match would still return the nearest photos. Results are ordered by relevance, not by date, so they do not fit the timeline with its months.
- **Similar photos:** the application takes the photo's vector from the database and finds the nearest ones the user can see, with no threshold. This needs no ML service.
- **Deleting:** the permission predicate hides photos in the trash, and permanently deleting a photo removes its vectors with its previews.

## Repository layout

This is a monorepo. The target layout is:

```
api/openapi.yaml        REST contract, the source of truth for both sides
backend/                Go module
  cmd/fuchsia-photos/   entry point (server + workers, admin creation command)
  cmd/imgbench/         performance benchmark
  internal/             auth, users, photos, ingest, media, albums, tags, sharing, scan, webdav, jobs, api
  db/migrations, db/queries   goose + sqlc
frontend/               Vue + Vuetify, TypeScript client generated from OpenAPI
ml/                     ML service: Python, FastAPI, PyTorch, uv
deploy/                 Dockerfile, compose.yaml (app, postgres, backup, caddy), Caddyfile
docs/                   architecture, roadmap, development, deployment and benchmark guides
scripts/release.sh      prepares a release for `make release`
```

The built frontend is embedded in the binary with `go:embed`. The image is built for `linux/amd64` and `linux/arm64` with Docker buildx, each on its own architecture, because cgo and libvips rule out simple cross-compilation. It includes libvips, libheif and exiftool.

## Data model

- `users` (login, argon2id password hash, admin/user role, disabled flag, whether "Shared with me" includes album photos, whether zooming in the viewer loads originals, which photo details it hides, whether the menu hides on a phone, whether the grid shows squares, whether the timeline includes shared photos), `sessions`
- `photos` (`bigint` id, owner, SHA-256, MIME type, size, dimensions, `taken_at`, EXIF as JSONB, the camera and lens generated from it, status, preview format and version, private/all visibility, `deleted_at`, `purged_at`).
- `photo_shares`, `albums` (with private/all visibility), `album_photos`, `album_shares`
- `tags` (owner, name), `photo_tags`
- `preview_settings` (one row: format, quality and preview sizes), `metadata_settings` (one row: whether the API returns locations)
- `app_passwords` (user, device name, SHA-256 of the password, last use), `webdav_entries` (user, path, folder or file, size, modification time, the photo), `webdav_albums` (user, folder path, album)
- `photo_embeddings` (photo, model, vector), `photo_embedding_failures` (photo, model, attempts, the last error)
- `import_runs` (scan report: found, added, duplicates, in the trash, permanently deleted, unsupported, still changing, failed), `import_files` (files still to process, and the ones to report)

## Access control

Permissions are checked in one place: a single SQL predicate, "this photo is visible to this user", used by every query, including file delivery. A photo is visible if any of these is true:

- The user is an admin or the photo's owner
- The photo is visible to all users
- The photo, or an album containing it, is shared with the user or visible to all users

A photo in the trash is visible only to its owner. Its sharing comes back when it is restored.

Authentication uses sessions in HttpOnly cookies. There is no registration: the first admin is created with a CLI command, and the admin manages all other users.

Previews contain no EXIF. The API returns only the camera details: camera, lens, focal length, aperture, shutter speed, ISO, exposure compensation and flash. It returns the GPS location only if an administrator turns it on, and then to everyone who can see the photo. Originals keep all EXIF, including GPS data, and recipients can download them. The share dialog warns about this.

Revoking access takes effect on the server at once. Previews are cached for a year as private, so a browser that already loaded one can still show it from its cache.
