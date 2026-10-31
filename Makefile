.PHONY: help dev-db dev-clean dev-admin dev-migrate dev-goose dev-psql migration dev-backend dev-frontend test e2e image bench fixtures api-types sqlc

COMPOSE_TEST = docker compose -f deploy/compose.test.yaml
COMPOSE_E2E = docker compose -f deploy/compose.e2e.yaml
E2E_PASSWORD = e2e-password
DEV_DATABASE_URL = postgres://photos:photos@localhost:5432/photos
BENCH_OUT ?= .bench
DEV_DATA = $(CURDIR)/deploy/data/dev
DEV_ENV = DATABASE_URL=$(DEV_DATABASE_URL) LIBRARY_DIR=$(DEV_DATA)/library CACHE_DIR=$(DEV_DATA)/cache COOKIE_INSECURE=true

# Keep the version in step with backend/go.mod. Without cgo and the other
# database drivers, the CLI builds on macOS and needs no C toolchain.
GOOSE = cd backend && CGO_ENABLED=0 go run \
	-tags 'no_clickhouse no_libsql no_mssql no_mysql no_sqlite3 no_turso no_vertica no_ydb' \
	github.com/pressly/goose/v3/cmd/goose@v3.29.0 -dir db/migrations

help:
	@echo "dev-db        start PostgreSQL for local development on port 5432"
	@echo "dev-clean     delete the development database and photos"
	@echo "dev-admin     create an administrator in dev-db, for example LOGIN=admin"
	@echo "dev-migrate   apply pending migrations to dev-db"
	@echo "dev-goose     run a goose command on dev-db, for example ARGS=status"
	@echo "dev-psql      open psql in dev-db"
	@echo "migration     create a migration file, for example NAME=add_albums"
	@echo "dev-backend   run the server locally against dev-db"
	@echo "dev-frontend  run the Vite dev server"
	@echo "test          run backend tests in a container with PostgreSQL"
	@echo "e2e           run the main flow in Chrome against the production image"
	@echo "image         build the production image for the NAS (linux/amd64)"
	@echo "bench         run the benchmark on samples/, BENCH_OUT sets the disk under test"
	@echo "fixtures      regenerate the image fixtures of internal/media"
	@echo "api-types     regenerate frontend types from api/openapi.yaml"
	@echo "sqlc          regenerate backend/internal/store from db/queries"

dev-db:
	docker run --rm --name fuchsia-photos-dev-db -p 127.0.0.1:5432:5432 -v fuchsia-photos-dev-db:/var/lib/postgresql \
		-e POSTGRES_USER=photos -e POSTGRES_PASSWORD=photos -e POSTGRES_DB=photos postgres:18

dev-clean:
	docker volume rm fuchsia-photos-dev-db
	rm -rf "$(DEV_DATA)"

dev-admin:
	cd backend && $(DEV_ENV) go run ./cmd/fuchsia-photos create-admin $(LOGIN)

dev-migrate:
	cd backend && $(DEV_ENV) go run ./cmd/fuchsia-photos migrate

dev-goose:
	$(GOOSE) postgres $(DEV_DATABASE_URL) $(ARGS)

migration:
	$(GOOSE) -s create $(NAME) sql

dev-psql:
	docker exec -it fuchsia-photos-dev-db psql -U photos photos

dev-backend:
	cd backend && $(DEV_ENV) go run ./cmd/fuchsia-photos serve

dev-frontend:
	cd frontend && pnpm dev

test:
	$(COMPOSE_TEST) up --build --abort-on-container-exit --exit-code-from test test; \
	status=$$?; $(COMPOSE_TEST) down --volumes; exit $$status

e2e:
	$(COMPOSE_E2E) up --build --detach --wait app; \
	status=$$?; \
	if [ $$status -eq 0 ]; then \
		for i in $$(seq 60); do curl -sf http://127.0.0.1:18080/api/v1/health >/dev/null && break; sleep 1; done; \
		printf '%s\n' "$(E2E_PASSWORD)" | $(COMPOSE_E2E) exec -T app fuchsia-photos create-admin admin && \
		(cd frontend && E2E_PASSWORD="$(E2E_PASSWORD)" pnpm e2e); \
		status=$$?; \
	fi; \
	if [ $$status -ne 0 ]; then $(COMPOSE_E2E) logs --tail 30 app; fi; \
	$(COMPOSE_E2E) down --volumes; exit $$status

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

sqlc:
	docker run --rm -u "$$(id -u):$$(id -g)" -v "$(CURDIR)/backend:/src" -w /src sqlc/sqlc:1.31.1 generate
