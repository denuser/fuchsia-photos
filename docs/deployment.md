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
| `IMPORT_STABLE_SECONDS` | `60` | How long a file must stay unchanged before a folder scan imports it |
| `WORKERS` | `3` | Photos processed at once. Keep at least one core free for the UI |
| `HTTP_PORT` | `80` | HTTP port on the host |
| `HTTPS_PORT` | `443` | HTTPS port on the host |

Relative paths are relative to the `deploy` folder.

The application runs as UID 1000, and Docker creates missing folders as root, so create them first and hand them over:

```sh
mkdir -p /path/to/library /path/to/cache /path/to/import
sudo chown 1000:1000 /path/to/library /path/to/cache
sudo chmod -R a+rX /path/to/import
```

The application only reads the import folder, so UID 1000 needs no more than read access there.

Then start everything and create the first administrator. Other users are managed in the web interface.

```sh
docker compose -f deploy/compose.yaml up -d --build
docker compose -f deploy/compose.yaml exec app fuchsia-photos create-admin admin
```

The last command waits for a password. Type it and press Enter. The password is visible while you type.

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

Processing continues after a restart. A job that was running during a clean stop runs again shortly after the next start. A job that was running during a crash or power loss resumes after about 3 minutes.

## Importing from a folder

For large collections, this is faster than uploading through the browser.

1. Copy the photos into the import folder on the NAS, in a subfolder per user if there are several.
2. In the web interface, as an administrator, open the account menu and choose **Import from folder**.
3. Choose the owner and the folder, and start the scan. One scan runs at a time, and it can be stopped.

The photos are copied into the library, and the source folder stays as it was. You can delete it after the scan, or keep it and scan it again after adding files: photos that are already in the library are not added twice. A scan reads every file in the folder each time, even on a repeat run, but writes only the new ones.

A scan does not bring back photos that the owner moved to the trash or deleted permanently. To get one back, the owner restores it from the trash or uploads the file through the browser.

Hidden files, symbolic links and folders whose name starts with `@` or `#`, such as `@eaDir` and `#recycle`, are left out.

A scan first lists the files, waits for `IMPORT_STABLE_SECONDS`, and skips any file whose size or modification time has changed since. The report lists these files, and the next scan picks them up, so it is safe to start a scan while files are still being copied.

A scan continues after a restart, from where it stopped. After a clean stop, this happens shortly after the next start. After a crash or power loss, it can take up to 30 minutes.

## Backups

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

## Running from an image without TLS

`deploy/compose.image.yaml` needs only the image, so the NAS does not need the source code. It leaves out Caddy and serves plain HTTP on port 10303, which has these costs:

- Passwords, session cookies and photos cross the network unencrypted, so use it only on a trusted home network and do not forward the port to the internet.
- Without HTTP/2, browsers open few connections at once, so the grid loads previews more slowly.
- Responses are not compressed.

It uses the image that CI publishes to `ghcr.io/denuser/fuchsia-photos` on every push to `master`, for `linux/amd64`. Each image has two tags:

- `latest`, the newest build of `master`;
- `sha-<commit>`, for example `sha-6593702`, which never changes.

Copy `deploy/compose.image.yaml` to a folder on the NAS as `docker-compose.yml`. Next to it, create a `.env` file with `POSTGRES_PASSWORD` and any of the other variables from [First start](#first-start), apart from `SITE_ADDRESS`, `HTTP_PORT` and `HTTPS_PORT`. Prepare the folders as described there.

Run these commands from that folder:

```sh
docker compose up -d
docker compose exec app fuchsia-photos create-admin admin
```

To update:

```sh
docker compose pull
docker compose up -d
```

To run a specific build, set `IMAGE_TAG=sha-<commit>` in `.env`. Going back to an older build is safe only if no migrations were added since: migrations run forward only, so after a new one an older build may not start against the database.

## Running on TrueNAS

On TrueNAS, the image from [Running from an image without TLS](#running-from-an-image-without-tls) runs as a custom app, with the same trade-offs. TrueNAS does not read a `.env` file, so the values go into the YAML itself.

Both containers run as the TrueNAS `apps` user and group, UID and GID 568. Create a dataset for the application, and inside it a dataset or folder for each of `library`, `cache`, `import` and `postgres`. In **Datasets**, select the parent dataset and choose **Edit** on the **Permissions** card. Set the owner and the owning group to `apps`, give `owner@` full control and `group@` modify, and apply the permissions to the child datasets. The application only reads `import`, so files copied there by other users need to be readable by `apps`.

In **Apps**, choose **Discover Apps**, then **Install via YAML** from the menu, and paste this, with your paths and password:

```yaml
services:
  postgres:
    image: postgres:18
    restart: unless-stopped
    user: '568:568'
    environment:
      POSTGRES_USER: photos
      POSTGRES_DB: photos
      POSTGRES_PASSWORD: change-me
      PGDATA: /var/lib/postgresql/data/pgdata
    command: [postgres, -c, shared_buffers=1GB, -c, effective_cache_size=4GB]
    volumes:
      - /mnt/pool/fuchsia/postgres:/var/lib/postgresql/data
    healthcheck:
      test: [CMD-SHELL, pg_isready -U photos -d photos]
      interval: 15s
      timeout: 30s
      retries: 20
      start_period: 60s

  app:
    image: ghcr.io/denuser/fuchsia-photos:latest
    restart: unless-stopped
    user: '568:568'
    depends_on:
      postgres:
        condition: service_healthy
    environment:
      DATABASE_URL: postgres://photos:change-me@postgres:5432/photos
      COOKIE_INSECURE: 'true'
      WORKERS: 3
      IMPORT_STABLE_SECONDS: 60
    ports:
      - '10303:8080'
    volumes:
      - /mnt/pool/fuchsia/library:/data/library
      - /mnt/pool/fuchsia/cache:/data/cache
      - /mnt/pool/fuchsia/import:/data/import:ro
```

The password appears twice, in `POSTGRES_PASSWORD` and in `DATABASE_URL`, and both must match. Use only URL-safe characters.

Create the first administrator from the TrueNAS shell. Docker names the container after the app, so look it up first:

```sh
sudo docker ps --format '{{.Names}}' | grep app
sudo docker exec -it <container> fuchsia-photos create-admin admin
```

### Updates

TrueNAS compares the `latest` image on the NAS with the one in the registry and shows **Update** on the app when they differ. Updating pulls the new image and recreates the containers, and migrations run on start. A pinned tag such as `sha-<commit>` never shows an update.

The check needs **Check for image updates** turned on in the **Apps** settings. It runs once a day and when the Docker service starts. To check right away, run this in the TrueNAS shell and then reload the Apps page:

```sh
sudo midclt call app.image.op.check_update
```

On newer TrueNAS releases the method is `app.image.check_update`.
