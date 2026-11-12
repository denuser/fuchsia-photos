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
| `LOG_LEVEL` | `info` | `debug`, `info`, `warn` or `error`. See [Logs](#logs) |
| `LOG_FORMAT` | `text` | `text`, or `json` for a log collector |
| `BACKUP_DIR` | `./data/backups` | Nightly database dumps. See [Backups](#backups) |
| `BACKUP_TIME` | `03:00` | When the nightly dump runs, as `HH:MM` |
| `BACKUP_KEEP` | `14` | How many nightly dumps are kept |
| `TZ` | `UTC` | Time zone of `BACKUP_TIME`, for example `Europe/Moscow` |
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
| Dump the database now | `docker compose -f deploy/compose.yaml exec backup pg_dump --format=custom --file=/backups/manual.dump` |
| Make all previews again | `docker compose -f deploy/compose.yaml exec app fuchsia-photos render-previews` |

Migrations are also applied automatically each time the application starts.

Processing continues after a restart. A job that was running during a clean stop runs again shortly after the next start. A job that was running during a crash or power loss resumes after about 3 minutes.

### Logs

The application writes its log to the container output, which `docker compose logs app` shows. At the default `info` level, it holds errors, requests that failed or took over a second, and every WebDAV request. `debug` adds every other request. Set `LOG_FORMAT=json` to feed the log to a collector.

Docker keeps the application's log in five files of up to 10 MB, dropping the oldest.

### Job queue

The **Queue** section of the administration page shows the background jobs by kind: waiting, running, waiting for another attempt, and finished or failed in the last hour and day. Under it are the latest errors, with the photo each job worked on. It updates on its own while the page is open. Finished jobs are kept for a day.

### Making grid previews again

Preview settings apply to new photos. After changing the grid preview size, or after an update that changes how grid previews are made, queue new grid previews of all photos:

```sh
docker compose -f deploy/compose.yaml exec app fuchsia-photos render-grid
```

The running server makes them in the background, from the viewer previews, after any new photos waiting for processing. Each photo keeps its preview format. Running the command again while jobs are waiting does not queue a photo twice. An open timeline picks up the new previews without reloading.

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

## Uploading from a phone over WebDAV

Sync apps such as PhotoSync and FolderSync can upload photos to `https://<address>/webdav/`, or `https://<address>:10304/webdav/` when [running from an image](#running-from-an-image).

FolderSync needs HTTPS: given an `http://` address, it still tries TLS and reports "unable to parse TLS packet header". With the certificate from Caddy's local CA, turn on "Allow self-signed certificate" in the account settings.

1. In the web interface, open **Profile**. Under **Upload from a phone**, choose **New app password** and name the device.
2. In the app, add a WebDAV server with the address, your login and the app password. The account password does not work there.
3. Set the app to upload only. Photos and videos are accepted, other files are refused. The video of an iPhone Live Photo is paired with its photo and plays as its motion, as long as the app keeps Apple's `ContentIdentifier` tag in both files.
4. To sort uploads into albums, give each folder pair in the app its own server folder, such as `/Camera` and `/Telegram`. Then, under **Albums for folders** in the profile, choose **Link a folder** and pick the folder and the album. A folder can be linked before the app creates it, and `/` links the whole tree.

Files deleted in the app, or by the app's sync, disappear from the WebDAV folder and stay in the library. Photos in the trash stay there when the app sends them again. A photo can be up to 512 MiB and a video up to 4 GiB.

Revoke a password in the profile if a device is lost. Whoever has it can upload photos as that user and download the originals uploaded over WebDAV, but not log in to the web interface.

The server logs every WebDAV request with its method, path, status and client, to see how real clients behave.

## Backups

| What | Where | How |
|---|---|---|
| Database | `BACKUP_DIR` | Dumped every night by the `backup` container |
| Originals | `LIBRARY_DIR` | Snapshots of the folder, for example ZFS snapshots on TrueNAS |
| Previews | `CACHE_DIR` | Not backed up. After a restore, `render-previews` makes them again from the originals |

Back up the dumps and the originals together, and keep a copy off the NAS: a snapshot on the same pool does not survive the loss of the pool. On TrueNAS, add a periodic snapshot task for the `backups` and `library` datasets that runs daily, shortly after `BACKUP_TIME`, and a replication task to another disk or machine.

### Nightly dumps

The `backup` container runs `pg_dump` every day at `BACKUP_TIME` in the `TZ` time zone. It writes `photos-YYYY-MM-DD-HHMM.dump` in the PostgreSQL custom format and keeps the latest `BACKUP_KEEP` of them. Its log shows each dump with its size, or the error:

```sh
docker compose -f deploy/compose.yaml logs backup
```

A dump made by hand, as in [Operations](#operations), is kept as long as its name does not start with `photos-`.

The dump and the snapshot of the originals are taken at different times. A photo uploaded in between has an original but no record after a restore, which is harmless: uploading it again reuses the original. A photo permanently deleted in between has a record but no original, and shows as failed after a restore. Taking the snapshot shortly after the dump keeps this window small.

### Restoring

This works on the same installation or on a new one. Use the same version of the application or a newer one: migrations run forward only, so a newer version upgrades an older dump when it starts.

1. Restore the originals into `LIBRARY_DIR` from the snapshot, unless they are intact. Empty `CACHE_DIR`, as every preview is made again.
2. Copy the dump into `BACKUP_DIR`.
3. Stop the application, leaving the database and the `backup` container running. On a new installation, start only those two:

   ```sh
   docker compose -f deploy/compose.yaml stop app
   docker compose -f deploy/compose.yaml up -d postgres backup
   ```

4. Replace the database with the dump:

   ```sh
   docker compose -f deploy/compose.yaml exec backup pg_restore --clean --if-exists --create --dbname=postgres /backups/photos-2026-11-10-0300.dump
   ```

5. Move photo numbers past the ones used since the dump. Browsers keep previews for a year by photo number, so a new photo must not get the number of one uploaded after the dump:

   ```sh
   docker compose -f deploy/compose.yaml exec backup psql -c "SELECT setval(pg_get_serial_sequence('photos', 'id'), nextval(pg_get_serial_sequence('photos', 'id')) + 1000000)"
   ```

6. Start everything and queue new previews of all photos:

   ```sh
   docker compose -f deploy/compose.yaml up -d
   docker compose -f deploy/compose.yaml exec app fuchsia-photos render-previews
   ```

Logins and passwords are those of the dump. The previews are made in the background, after any new photos, and the **Queue** section of the administration page shows how far they are. Until a photo has them, its tile stays empty. A photo whose original is missing shows as failed.

On TrueNAS, run the commands in the shell with `sudo docker exec` and the container name in place of `docker compose -f deploy/compose.yaml exec` and the service name, as in [Running on TrueNAS](#running-on-truenas). Stop and start the application container with `sudo docker stop` and `sudo docker start`.

## Building on another machine

Instead of building on the NAS, you can build the image elsewhere and copy it over. `make image` builds for `linux/amd64`; for an ARM NAS, run `make image PLATFORM=linux/arm64`:

```sh
make image
docker save fuchsia-photos:latest | ssh <nas> docker load
```

On the NAS, start it without `--build`:

```sh
docker compose -f deploy/compose.yaml up -d
```

A build for an architecture other than the machine's own, such as `linux/amd64` on an Apple Silicon Mac, runs under emulation and is noticeably slower.

## Running from an image

`deploy/compose.image.yaml` needs only the image, so the NAS does not need the source code. The Caddyfile is written into it. Caddy serves HTTPS on port 10304, as ports 80 and 443 are often taken, for example by the TrueNAS web interface, and there is no plain HTTP port. Open the application at `https://<address>:10304/`. The certificate comes from Caddy's local CA, as in [First start](#first-start).

Caddy runs on the host network, and the application listens only on `127.0.0.1:10303` for it. This lets the NAS have several addresses, such as one at home and one in a VPN: a client that connects by IP address sends no server name, and Caddy then picks the certificate by the address the connection came to, which it cannot see from a Docker network.

This file uses Compose `configs` with `content`, which needs Docker Compose 2.23 or later.

It uses the image that CI publishes to `ghcr.io/denuser/fuchsia-photos`, for `linux/amd64` and `linux/arm64`; Docker picks the one for the NAS. The `arm64` image has not been tried on an ARM NAS yet. The tags are:

- `latest`, the newest build of `master`;
- `sha-<commit>`, for example `sha-6593702`, which never changes;
- `X.Y.Z`, for example `0.1.0`, a release, which never changes;
- `X.Y`, the newest release of that minor version.

Release notes are on the GitHub releases page and in `CHANGELOG.md`.

Copy `deploy/compose.image.yaml` to a folder on the NAS as `docker-compose.yml`. Next to it, create a `.env` file with `POSTGRES_PASSWORD`, `SITE_ADDRESS` and any of the other variables from [First start](#first-start), apart from `HTTP_PORT`. Some of them differ here:

| Variable | Default | Purpose |
|---|---|---|
| `SITE_ADDRESS` | required | Addresses of the NAS separated by commas, for example `192.168.0.101, 10.8.1.7` |
| `HTTPS_PORT` | `10304` | HTTPS port on the host |
| `CADDY_DIR` | `./data/caddy` | Caddy's certificates and local CA. Keep it, or browsers and phones that trust the CA have to accept a new one |

Prepare the folders as described there.

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

To run a specific release or build, set `IMAGE_TAG=0.1.0` or `IMAGE_TAG=sha-<commit>` in `.env`. Going back to an older build is safe only if no migrations were added since: migrations run forward only, so after a new one an older build may not start against the database.

## Running on TrueNAS

On TrueNAS, the setup from [Running from an image](#running-from-an-image) runs as a custom app. TrueNAS does not read a `.env` file, so the values go into the YAML itself.

Both containers run as the TrueNAS `apps` user and group, UID and GID 568. Create a dataset for the application, and inside it a dataset or folder for each of `library`, `cache`, `import`, `postgres`, `backups` and `caddy`. Separate datasets for `library` and `backups` can be snapshotted on their own, as described in [Backups](#backups). In **Datasets**, select the parent dataset and choose **Edit** on the **Permissions** card. Set the owner and the owning group to `apps`, give `owner@` full control and `group@` modify, and apply the permissions to the child datasets. The application only reads `import`, so files copied there by other users need to be readable by `apps`.

In **Apps**, choose **Discover Apps**, then **Install via YAML** from the menu, and paste this, with your paths and password:

```yaml
services:
  postgres:
    image: pgvector/pgvector:pg18-trixie
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
      WORKERS: 3
      IMPORT_STABLE_SECONDS: 60
      LOG_LEVEL: info
    ports:
      - '127.0.0.1:10303:8080'
    logging:
      driver: json-file
      options:
        max-size: 10m
        max-file: '5'
    volumes:
      - /mnt/pool/fuchsia/library:/data/library
      - /mnt/pool/fuchsia/cache:/data/cache
      - /mnt/pool/fuchsia/import:/data/import:ro

  backup:
    image: postgres:18
    restart: unless-stopped
    user: '568:568'
    depends_on:
      postgres:
        condition: service_healthy
    environment:
      PGHOST: postgres
      PGUSER: photos
      PGDATABASE: photos
      PGPASSWORD: change-me
      BACKUP_TIME: '03:00'
      BACKUP_KEEP: 14
      TZ: Europe/Moscow
    volumes:
      - /mnt/pool/fuchsia/backups:/backups
    entrypoint: [sh, -c]
    command:
      - |
        trap 'exit 0' TERM INT
        echo "backup: every day at $$BACKUP_TIME $$TZ, keeping $$BACKUP_KEEP dumps"
        while true; do
          now=$$(date +%s)
          next=$$(date -d "today $$BACKUP_TIME" +%s) || exit 1
          [ "$$next" -gt "$$now" ] || next=$$(date -d "tomorrow $$BACKUP_TIME" +%s)
          sleep $$((next - now)) & wait $$!
          name=photos-$$(date +%Y-%m-%d-%H%M).dump
          if pg_dump --format=custom --file=/backups/.partial; then
            mv /backups/.partial "/backups/$$name"
            echo "backup: wrote $$name, $$(du -h "/backups/$$name" | cut -f1)"
            ls -1 /backups/photos-*.dump | head -n -"$$BACKUP_KEEP" | xargs -r rm -v
          else
            echo "backup: pg_dump failed" >&2
            rm -f /backups/.partial
          fi
        done

  caddy:
    image: caddy:2
    restart: unless-stopped
    user: '568:568'
    depends_on:
      - app
    network_mode: host
    configs:
      - source: caddyfile
        target: /etc/caddy/Caddyfile
    volumes:
      - /mnt/pool/fuchsia/caddy:/data

configs:
  caddyfile:
    content: |
      {
        https_port 10304
        auto_https disable_redirects
        skip_install_trust
        admin off
      }

      192.168.0.101, 10.8.1.7 {
        tls internal
        encode zstd gzip
        reverse_proxy 127.0.0.1:10303
      }
```

Replace `192.168.0.101, 10.8.1.7` with the addresses that browsers and phones use for the NAS, separated by commas. Each address gets its own certificate.

An app installed from the earlier YAML, which served plain HTTP on port 10303, changes over when you edit its YAML to this one. The session cookie then needs HTTPS, so log in again at the new address.

The password appears three times, in `POSTGRES_PASSWORD`, `DATABASE_URL` and `PGPASSWORD`, and all must match. Set `TZ` to your time zone for the nightly dump at `BACKUP_TIME`. In the `backup` script, `$$` stands for `$`, as Compose would otherwise fill in the variables itself. Use only URL-safe characters.

Create the first administrator from the TrueNAS shell. Docker names the container after the app, so look it up first:

```sh
sudo docker ps --format '{{.Names}}' | grep app
sudo docker exec -it <container> fuchsia-photos create-admin admin
```

### Updates

TrueNAS compares the `latest` image on the NAS with the one in the registry and shows **Update** on the app when they differ. Updating pulls the new image and recreates the containers, and migrations run on start. A pinned tag such as `0.1.0` or `sha-<commit>` never shows an update.

The check needs **Check for image updates** turned on in the **Apps** settings. It runs once a day and when the Docker service starts. To check right away, run this in the TrueNAS shell and then reload the Apps page:

```sh
sudo midclt call app.image.op.check_update
```

On newer TrueNAS releases the method is `app.image.check_update`.

Since stage 7 the database needs the `vector` extension, and the `postgres` service uses `pgvector/pgvector:pg18-trixie` instead of `postgres:18`. Before updating an installation from an older YAML, change that line in **Edit**. Both images are PostgreSQL 18 on the same Debian release, so the existing database starts as it is. Without the change the application stops at start with an error about the `vector` extension. The `backup` service stays on `postgres:18`.
