# Development

## Requirements

- Docker
- Go 1.27
- Node.js 24 and pnpm 12

libvips is not needed on the host. The server itself builds without it, and everything that uses it (`internal/media`, `cmd/imgbench`) builds and runs in containers.

Run `make help` to list all `make` targets. All commands below run from the repository root.

## Running locally

Run each of these in its own terminal:

```sh
make dev-db         # PostgreSQL on port 5432
make dev-backend    # applies migrations, then serves the API on port 8080
make dev-frontend   # Vite dev server on port 5173
```

Open <http://localhost:5173>. Vite proxies `/api` to the backend.

`make dev-db` keeps no data: the database is empty after each restart.

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

`make dev-goose` manages only the application's migrations, not River's.

A new migration file has an empty `Up` and `Down` section, and both must be filled in. Change only migrations that have not been merged yet. After that, add a new one.

### API

`api/openapi.yaml` is the contract between the backend and the frontend. Change it first, then:

1. Update the handlers in `backend/internal/api`. They are written by hand for now.
2. Run `make api-types` to regenerate the frontend types, and commit the result. CI fails if they are out of date.

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
| API and frontend serving, on the host | `cd backend && go test ./internal/api ./internal/web` |
| Database tests, against `make dev-db` | `cd backend && TEST_DATABASE_URL=postgres://photos:photos@localhost:5432/photos go test ./db` |
| Selected tests in a container | see below |
| Regenerate image fixtures | `make fixtures` |

To run selected tests in a container:

```sh
docker compose -f deploy/compose.test.yaml run --rm --build test go test -count=1 -v -run TestRender ./internal/media
docker compose -f deploy/compose.test.yaml down --volumes
```

Things to know:

- `go test ./...` fails on the host, because `internal/media` needs libvips. Use `make test`.
- Without `TEST_DATABASE_URL`, the database tests are skipped, and `go test` still reports `ok`. `make test` sets it.
- The database tests leave the schema in place, so use a disposable database.
- The frontend has no tests yet. Type checks and the build are its only automated checks.

## CI

`.github/workflows/ci.yml` runs on pushes to `master` and on pull requests:

- `make test`
- Frontend: a check that the generated API types are current, type checks and the build
- A `linux/amd64` production image, with a check that it decodes every supported format
