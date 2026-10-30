# Deployment

The application runs on the NAS as three containers: the application, PostgreSQL and Caddy. All commands below run from the repository root.

## First start

```sh
cp deploy/.env.example deploy/.env
```

Edit `deploy/.env`:

| Variable | Default | Purpose |
|---|---|---|
| `POSTGRES_PASSWORD` | required | Database password. Use only URL-safe characters |
| `SITE_ADDRESS` | `localhost` | Host name or IP address of the NAS |
| `POSTGRES_DIR` | `./data/postgres` | Database files. Use an SSD if the NAS has one |
| `CACHE_DIR` | `./data/cache` | Previews. Use an SSD if the NAS has one |
| `LIBRARY_DIR` | `./data/library` | Original photos |
| `IMPORT_DIR` | `./data/import` | Folder scanned for imports, mounted read-only |
| `HTTP_PORT` | `80` | HTTP port on the host |
| `HTTPS_PORT` | `443` | HTTPS port on the host |

Relative paths are relative to the `deploy` folder. The application runs as UID 1000, so it needs write access to `LIBRARY_DIR` and `CACHE_DIR`.

Then start everything:

```sh
docker compose -f deploy/compose.yaml up -d --build
```

Without a public domain, Caddy issues a certificate from its local CA, and browsers show a warning until that CA is trusted. To use a public certificate for a real domain, remove the `tls internal` line from `deploy/Caddyfile`.

## Operations

| Task | Command |
|---|---|
| Show container status | `docker compose -f deploy/compose.yaml ps` |
| Follow application logs | `docker compose -f deploy/compose.yaml logs -f app` |
| Stop | `docker compose -f deploy/compose.yaml down` |
| Update after pulling new code | `docker compose -f deploy/compose.yaml up -d --build` |
| Apply migrations by hand | `docker compose -f deploy/compose.yaml exec app fuchsia-photos migrate` |
| Open psql | `docker compose -f deploy/compose.yaml exec postgres psql -U photos photos` |

Migrations are also applied automatically each time the application starts.

Backups are not set up yet. They are planned for stage 5.

## Building on another machine

Instead of building on the NAS, you can build the `linux/amd64` image elsewhere and copy it over:

```sh
make image
docker save fuchsia-photos:latest | ssh <nas> docker load
```

On the NAS, start it without `--build`:

```sh
docker compose -f deploy/compose.yaml up -d
```

On an ARM machine such as an Apple Silicon Mac, `make image` runs under emulation and is noticeably slower.
