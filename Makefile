.PHONY: help dev-db dev-migrate dev-goose dev-psql migration dev-backend dev-frontend test image bench fixtures api-types

COMPOSE_TEST = docker compose -f deploy/compose.test.yaml
DEV_DATABASE_URL = postgres://photos:photos@localhost:5432/photos
BENCH_OUT ?= .bench

# Keep the version in step with backend/go.mod. Without cgo and the other
# database drivers, the CLI builds on macOS and needs no C toolchain.
GOOSE = cd backend && CGO_ENABLED=0 go run \
	-tags 'no_clickhouse no_libsql no_mssql no_mysql no_sqlite3 no_turso no_vertica no_ydb' \
	github.com/pressly/goose/v3/cmd/goose@v3.29.0 -dir db/migrations

help:
	@echo "dev-db        start PostgreSQL for local development on port 5432"
	@echo "dev-migrate   apply pending migrations to dev-db"
	@echo "dev-goose     run a goose command on dev-db, for example ARGS=status"
	@echo "dev-psql      open psql in dev-db"
	@echo "migration     create a migration file, for example NAME=add_albums"
	@echo "dev-backend   run the server locally against dev-db"
	@echo "dev-frontend  run the Vite dev server"
	@echo "test          run backend tests in a container with PostgreSQL"
	@echo "image         build the production image for the NAS (linux/amd64)"
	@echo "bench         run the benchmark on samples/, BENCH_OUT sets the disk under test"
	@echo "fixtures      regenerate the image fixtures of internal/media"
	@echo "api-types     regenerate frontend types from api/openapi.yaml"

dev-db:
	docker run --rm --name fuchsia-photos-dev-db -p 127.0.0.1:5432:5432 \
		-e POSTGRES_USER=photos -e POSTGRES_PASSWORD=photos -e POSTGRES_DB=photos postgres:18

dev-migrate:
	cd backend && DATABASE_URL=$(DEV_DATABASE_URL) go run ./cmd/fuchsia-photos migrate

dev-goose:
	$(GOOSE) postgres $(DEV_DATABASE_URL) $(ARGS)

migration:
	$(GOOSE) -s create $(NAME) sql

dev-psql:
	docker exec -it fuchsia-photos-dev-db psql -U photos photos

dev-backend:
	cd backend && DATABASE_URL=$(DEV_DATABASE_URL) go run ./cmd/fuchsia-photos serve

dev-frontend:
	cd frontend && pnpm dev

test:
	$(COMPOSE_TEST) up --build --abort-on-container-exit --exit-code-from test test; \
	status=$$?; $(COMPOSE_TEST) down --volumes; exit $$status

image:
	docker buildx build --platform linux/amd64 --load -f deploy/Dockerfile -t fuchsia-photos:latest .

bench:
	docker build -f deploy/Dockerfile -t fuchsia-photos:latest .
	mkdir -p "$(BENCH_OUT)"
	docker run --rm --user "$$(id -u):$$(id -g)" -v "$(CURDIR)/samples:/in:ro" -v "$(abspath $(BENCH_OUT)):/out" \
		fuchsia-photos:latest imgbench -in /in -out /out

fixtures:
	docker build --target fixtures -f deploy/Dockerfile -t fuchsia-photos-fixtures .
	docker run --rm -v "$(CURDIR)/backend/internal/media/testdata:/src/backend/internal/media/testdata" \
		-u "$$(id -u):$$(id -g)" fuchsia-photos-fixtures

api-types:
	cd frontend && pnpm gen:api
