# Fuchsia Photos

A self-hosted application for organizing photos, designed to run on a home NAS (Intel N150, 4 cores, 16 GB DDR5).

**Status:** stage 2. Login, user management, upload, processing, the timeline, albums, tags, search, bulk actions and the trash work. The stage 0 benchmark on the NAS is still to be run, so the preview format and worker count are provisional defaults. This document records the decisions made so far and the roadmap.

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
| Database | PostgreSQL | Relations and permissions, JSONB for EXIF, search, `pgvector` as a fallback. Keep it on an SSD if the NAS has one |
| Job queue | River (on PostgreSQL), no Redis | A photo and its job are written in one transaction, and there is one service less to run. No official BullMQ client for Go was found |
| API | REST with OpenAPI, SSE for progress | Works well with browsers, files and caching. A monolith needs no internal RPC. gRPC is only reconsidered when an ML service appears |
| Image processing | libvips through govips | libvips is fast and memory-efficient. govips has a long history, a large community, and is not tied to one libvips version |
| Photo metadata | exiftool through go-exiftool | Handles HEIC and camera-specific tags. One long-running process avoids starting Perl for every photo |
| Folder scan | Copy files into the library, leave the source folder untouched | One ingest path for all sources |
| Sharing | Photos and albums, with all users or by login | Sharing an album makes all of its photos visible to the recipient |
| Tags | Personal to each user. A search with several tags finds photos with any of them | Other users' tag names stay private |
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
5. A worker reads EXIF, applies auto-rotation and sRGB conversion, generates a grid preview (~400 px) and a viewer preview (~2048 px) on the SSD. Until a preview loads, the grid shows a neutral tile. WebP or JPEG is chosen from benchmarks, with WebP as the default until then. AVIF is ruled out because of its encoding cost.
6. A periodic job removes abandoned uploads and expired sessions. It leaves originals without a database record alone: after restoring an older database backup, those are the photos uploaded since that backup. A stray original is harmless, and uploading the same photo again reuses it.
7. Permanently deleting a photo from the trash removes its record, original and previews. A database lock on the owner and hash keeps this from colliding with an upload of the same file.

Queue behavior:

- 2–3 workers, leaving one core for the UI
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
samples/                real photos for the benchmark
```

The built frontend is embedded in the binary with `go:embed`. The image is built for `linux/amd64` with Docker buildx, because cgo and libvips rule out simple cross-compilation from macOS. It includes libvips, libheif and exiftool.

### Data model

- `users` (login, argon2id password hash, admin/user role, disabled flag), `sessions`
- `photos` (`bigint` id, owner, SHA-256, MIME type, size, dimensions, `taken_at`, EXIF as JSONB, status, private/all visibility, `deleted_at`). A `bigint` id also fits future FAISS ids (int64).
- `photo_shares`, `albums`, `album_photos`, `album_shares`
- `tags` (owner, name), `photo_tags`
- `import_runs` (scan report: found, added, duplicates, errors)

### Access control

Permissions are checked in one place: a single SQL predicate, "this photo is visible to this user", used by every query, including file delivery. A photo is visible if any of these is true:

- The user is an admin or the photo's owner
- The photo is visible to all users
- The photo, or an album containing it, is shared with the user

Authentication uses sessions in HttpOnly cookies. There is no registration: the first admin is created with a CLI command, and the admin manages all other users.

Previews contain no EXIF, so GPS data does not leak through sharing.

## Roadmap

- **Stage 0: skeleton and benchmarks.**
  - [x] Monorepo structure, compose, migrations, CI
  - [x] Choose the libvips binding
  - [ ] Benchmark on the real NAS: JPEG and HEIC, HDD and SSD, WebP and JPEG, 2 or 3 workers
- [x] **Stage 1: core.** Login, user management, roles. Browser upload (3–4 files in parallel, retrying failed files). Ingest, queue, previews, virtual-scrolling timeline, viewer, processing progress.
- [x] **Stage 2: organization.** Albums, tags, search by date and tag, bulk actions, trash with soft delete.
- [ ] **Stage 3: sharing.** Visibility for all users, sharing photos and albums by login, a "Shared with me" section, the admin view of everything. Test suite for the permission predicate.
- [ ] **Stage 4: folder scan.** Admin only: fixed root, owner selection, safe re-runs thanks to hashing. Skips files still being copied (size and mtime unchanged for N seconds). Report in `import_runs`. A re-run must not restore photos the owner moved to the trash, unlike a browser upload.
- [ ] **Stage 5: operations.** Nightly `pg_dump`, backup and tested restore instructions, logs and queue metrics.
- [ ] **Stage 6: WebDAV (hypothesis, starts as a spike).** Per-user upload folder, separate app password, files go through ingest. Tested with real mobile clients such as PhotoSync and FolderSync. If they work poorly, the feature is dropped and the outcome recorded here.
- [ ] **Stage 7: ML (later).** An `embed_photo` job sends a preview to an external server. The NAS stays the source of truth, and a FAISS index can be rebuilt. Search results are filtered by the permission predicate. The protocol (gRPC or HTTP) and the choice of FAISS or `pgvector` are decided at this stage.

## Expected performance

These are rough estimates, not measurements, for preview generation with 3 workers. Stage 0 replaces them with real numbers from the NAS.

| Photos | JPEG, 24 MP | HEIC |
|---|---|---|
| 1,000 | 2–4 min | 6–11 min |
| 10,000 | 20–35 min | 1–2 h |

For tens of thousands of photos, prefer copying them to the NAS and running a folder scan over uploading through the browser.

## Documentation

- [Development](docs/development.md): running locally, backend, migrations, frontend and test commands
- [Deployment](docs/deployment.md): installing and operating on the NAS
- [Benchmark](docs/benchmark.md): measuring performance on the NAS

## License

[MIT](LICENSE)
