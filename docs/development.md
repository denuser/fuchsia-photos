# Development

## Requirements

- Docker
- Go 1.27
- Node.js 24 and pnpm 12
- Google Chrome, for `make e2e`
- libvips, pkg-config and exiftool: `brew install vips pkg-config exiftool`

The backend links against libvips through cgo, so it needs a working C toolchain on the host.

The host may have a newer libvips than production: Homebrew has 8.18, while the image has 8.16 from Debian. Run `make test` before pushing, as it uses the same versions as the image.

### macOS: linker errors

On macOS, cgo builds can fail with an error like this:

```
ld: multiple errors: tapi error: malformed file
.../SDKs/MacOSX27.0.sdk/usr/lib/libSystem.B.tbd:4:20: error: unknown architecture
```

It means that the Command Line Tools are older than the macOS SDK selected by default. Check the versions with `ld -v` and `xcrun --show-sdk-path`.

The fix is to update the Command Line Tools through System Settings or `softwareupdate`. Until then, select an older SDK that the installed tools understand:

```sh
ls /Library/Developer/CommandLineTools/SDKs
export SDKROOT=/Library/Developer/CommandLineTools/SDKs/MacOSX26.sdk
```

With an older SDK, the linker prints many `was built for newer 'macOS' version` warnings. They are harmless, and the binary works.

Run `make help` to list all `make` targets. All commands below run from the repository root.

## Running locally

Run each of these in its own terminal:

```sh
make dev-db         # PostgreSQL on port 5432
make dev-admin LOGIN=admin   # once: create an administrator, then type a password and press Enter
make dev-backend    # applies migrations, then serves the API on port 8080
make dev-frontend   # Vite dev server on port 5173, also reachable from the local network
```

Open <http://localhost:5173>. Vite proxies `/api` to the backend.

The development database lives in a Docker volume and the photos in `deploy/data/dev`, so they survive restarts. To try a folder scan, put photos in `deploy/data/dev/import`. To start over, stop the database and run `make dev-clean`.

## Backend

| Task | Command |
|---|---|
| Run the server | `make dev-backend` |
| Build the server | `cd backend && go build ./cmd/fuchsia-photos` |
| Format the code | `cd backend && gofmt -w .` |
| Add a Go dependency | `cd backend && go get <module>@<version> && go mod tidy` |

The server reads its settings from environment variables:

| Variable | Default | Purpose |
|---|---|---|
| `DATABASE_URL` | required | PostgreSQL connection URL |
| `HTTP_ADDR` | `:8080` | Listen address |
| `LIBRARY_DIR` | `/data/library` | Original photos |
| `CACHE_DIR` | `/data/cache` | Previews |
| `IMPORT_DIR` | `/data/import` | The folder that scans read from. Only read, never changed |
| `IMPORT_STABLE_SECONDS` | `60` | How long a file must stay unchanged before a scan imports it |
| `WORKERS` | `3` | Photos processed at once |
| `LOG_LEVEL` | `info` | `debug`, `info`, `warn` or `error`. `debug` logs every request |
| `LOG_FORMAT` | `text` | `text` or `json` |
| `COOKIE_INSECURE` | unset | Set to `true` to allow the session cookie over plain HTTP. For local development and the e2e stack |

### Database and migrations

Migrations live in `backend/db/migrations` and are embedded in the binary. The server applies pending ones on every start, including the migrations of the River job queue.

| Task | Command |
|---|---|
| Create a migration | `make migration NAME=add_albums` |
| Apply pending migrations | `make dev-migrate` |
| Show migration status | `make dev-goose ARGS=status` |
| Roll back the last migration | `make dev-goose ARGS=down` |
| Apply rolled back migrations | `make dev-goose ARGS=up` |
| Open psql | `make dev-psql` |
| Regenerate Go code after changing `db/queries` or the schema | `make sqlc` |

`make dev-goose` manages only the application's migrations, not River's.

A permanently deleted photo keeps its row in `photos`, with `purged_at` set and `deleted_at` still set. Queries that filter on `deleted_at IS NULL` leave it out by themselves, but a query about the trash must also check `purged_at IS NULL`.

A new migration file has an empty `Up` and `Down` section, and both must be filled in. Change only migrations that have not been merged yet. After that, add a new one.

### API

`api/openapi.yaml` is the contract between the backend and the frontend. Change it first, then:

1. Update the handlers in `backend/internal/api`. They are written by hand.
2. Run `make api-types` to regenerate the frontend types, and commit the result.

## Frontend

| Task | Command |
|---|---|
| Install dependencies | `cd frontend && pnpm install` |
| Run the dev server | `make dev-frontend` |
| Check types | `cd frontend && pnpm exec vue-tsc --noEmit` |
| Build for production | `cd frontend && pnpm build` |
| Regenerate API types | `make api-types` |

TypeScript stays on version 5, because `vue-tsc` does not work with version 7 yet.

## Tests

| Task | Command |
|---|---|
| All backend tests, with PostgreSQL | `make test` |
| One package, on the host | `cd backend && go test ./internal/web` |
| Everything on the host, against a scratch database | see below |
| Selected tests in a container | see below |
| The main flow in a browser | `make e2e` |
| Regenerate image fixtures | `make fixtures` |

To run everything on the host, use a scratch database rather than the development one:

```sh
docker run -d --rm --name fuchsia-photos-test-db -p 127.0.0.1:5433:5432 -e POSTGRES_PASSWORD=test postgres:18
cd backend && TEST_DATABASE_URL=postgres://postgres:test@localhost:5433/postgres go test ./...
docker stop fuchsia-photos-test-db
```

To run selected tests in a container:

```sh
docker compose -f deploy/compose.test.yaml run --rm --build test go test -count=1 -v -run TestRender ./internal/media
docker compose -f deploy/compose.test.yaml down --volumes
```

Things to know:

- Without `TEST_DATABASE_URL`, the database tests are skipped, and `go test` still reports `ok`. `make test` sets it.
- Each test creates and drops its own database next to the one in `TEST_DATABASE_URL`, so tests run in parallel without interfering. `TestMigrate` in `./db` uses that database directly and leaves the schema in place.
- API, job and database tests need `TEST_DATABASE_URL`, including on the host. The job tests also need exiftool.
- The frontend has no unit tests. Its automated checks are type checks, the build and `make e2e`.

### Browser test

`make e2e` builds the production image, starts it with a throwaway database on port 18080, runs `frontend/e2e/main-flow.mjs` in the installed Google Chrome, and removes everything afterwards. It does not touch the development data, and it can run while `make dev-db` is up. The first run takes a few minutes to build the image.

It covers one path through the application: upload, selection, albums, tags, search, sharing with a second user, the trash and a folder scan. Add a step when a stage adds something to that path, and leave details to the backend tests. Run it before pushing a change to the interface.

On failure it prints the failed step and saves a screenshot to `frontend/e2e/results/`. To watch it run, set `E2E_HEADED=1`.

## CI

`.github/workflows/ci.yml` builds the `linux/amd64` production image, which includes the frontend type checks and build and the backend build. On pull requests it only builds it. On pushes to `master` it also publishes it to `ghcr.io/denuser/fuchsia-photos` with the tags `latest` and `sha-<commit>`; only this job can write packages.

Tests do not run in CI, so run `make test` and, for interface changes, `make e2e` before pushing.
