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
| `FOLDERS_DIR` | `./data/folders` | Folders read where they are, mounted read-only. See [Library folders](#library-folders) |
| `IMPORT_STABLE_SECONDS` | `60` | How long a file must stay unchanged before a folder scan imports it |
| `WORKERS` | `3` | Photos processed at once. Keep at least one core free for the UI |
| `MEMORY_LIMIT` | `4G` | Memory the application container may take. See [RAW photos](#raw-photos) |
| `LOG_LEVEL` | `info` | `debug`, `info`, `warn` or `error`. See [Logs](#logs) |
| `LOG_FORMAT` | `text` | `text`, or `json` for a log collector |
| `BACKUP_DIR` | `./data/backups` | Nightly database dumps. See [Backups](#backups) |
| `BACKUP_TIME` | `03:00` | When the nightly dump runs, as `HH:MM` |
| `BACKUP_KEEP` | `14` | How many nightly dumps are kept |
| `TZ` | `UTC` | Time zone of `BACKUP_TIME`, for example `Europe/Moscow` |
| `ML_URL` | empty | The ML service, for example `http://mac.local:8090`. See [Search and people](#search-and-people) |
| `ML_TOKEN` | empty | Token the application sends to the ML service. People need it |
| `HTTP_PORT` | `80` | HTTP port on the host |
| `HTTPS_PORT` | `443` | HTTPS port on the host |

Relative paths are relative to the `deploy` folder.

The application runs as UID 1000, and Docker creates missing folders as root, so create them first and hand them over:

```sh
mkdir -p /path/to/library /path/to/cache /path/to/import /path/to/folders
sudo chown 1000:1000 /path/to/library /path/to/cache
sudo chmod -R a+rX /path/to/import /path/to/folders
```

The application only reads the import and library folders, so UID 1000 needs no more than read access there.

Then start everything and create the first administrator. Other users are managed in the web interface.

```sh
docker compose -f deploy/compose.yaml up -d --build
docker compose -f deploy/compose.yaml exec app magellanica-photos create-admin admin
```

The last command waits for a password. Type it and press Enter. The password is visible while you type.

Without a public domain, Caddy issues a certificate from its local CA, and browsers show a warning until that CA is trusted. [Trusting the certificate](#trusting-the-certificate) removes the warning. To use a public certificate for a real domain, remove the `tls internal` line from `deploy/Caddyfile`.

### Trusting the certificate

The warning appears because the certificate is signed by a CA that Caddy created on the NAS, and your devices do not know it. The traffic is encrypted either way; trusting the CA once on each device makes the warning go away for good, and lets sync apps that refuse self-signed certificates connect.

Copy the CA's certificate out of the Caddy container. Run this next to the `docker-compose.yml` of [a release image](#running-from-an-image), or add `-f deploy/compose.yaml` when running from the source:

```sh
docker compose cp caddy:/data/caddy/pki/authorities/local/root.crt ./magellanica-photos-ca.crt
```

The file is public and holds no secret, so it is fine to send it by e-mail or a messenger. Never share the `root.key` next to it. Then install it on each device:

- **macOS:** `sudo security add-trusted-cert -d -r trustRoot -k /Library/Keychains/System.keychain magellanica-photos-ca.crt`, or open the file in Keychain Access, then open the certificate and set **Trust** to **Always Trust**. Restart the browser.
- **Windows:** in an administrator terminal, `certutil -addstore -f Root magellanica-photos-ca.crt`, or open the file and choose **Install Certificate**, **Local Machine**, then **Trusted Root Certification Authorities**.
- **Linux:** on Debian and Ubuntu, copy the file to `/usr/local/share/ca-certificates/magellanica-photos-ca.crt` and run `sudo update-ca-certificates`. Chrome and Firefox keep their own list: import the file under **Settings**, **Certificates**, **Authorities**.
- **Firefox on any system** may ignore the system store. Import the file under **Settings**, **Privacy & Security**, **Certificates**, **View Certificates**, **Authorities**, and tick "Trust this CA to identify websites".
- **iPhone and iPad:** send the file to the device and open it, allow the profile in **Settings**, **Profile Downloaded**, and install it. Then turn on full trust in **Settings**, **General**, **About**, **Certificate Trust Settings**. Without that last step Safari still warns.
- **Android:** **Settings**, **Security**, **Encryption & credentials**, **Install a certificate**, **CA certificate**, and choose the file. The menu differs between makers. Chrome then trusts it, but many apps ignore CAs added by the user, so FolderSync still needs "Allow self-signed certificate", as described under [WebDAV](#uploading-from-a-phone-over-webdav).

The CA lives in Caddy's data folder, which `CADDY_DIR` sets when running from an image. Keep that folder, or Caddy creates a new CA and every device has to trust it again. A new address in `SITE_ADDRESS` needs nothing on the devices: the same CA signs its certificate.

To skip the manual step altogether, use a real domain name. Point a name such as `photos.example.com` at the NAS in your router's DNS, and have Caddy get a public certificate with the DNS challenge, which needs no open ports but a Caddy build with the plugin for your DNS provider. Then remove `tls internal` from the Caddyfile.

## Operations

| Task | Command |
|---|---|
| Show container status | `docker compose -f deploy/compose.yaml ps` |
| Follow application logs | `docker compose -f deploy/compose.yaml logs -f app` |
| Stop | `docker compose -f deploy/compose.yaml down` |
| Update after pulling new code | `docker compose -f deploy/compose.yaml up -d --build` |
| Apply migrations by hand | `docker compose -f deploy/compose.yaml exec app magellanica-photos migrate` |
| Open psql | `docker compose -f deploy/compose.yaml exec postgres psql -U photos photos` |
| Dump the database now | `docker compose -f deploy/compose.yaml exec backup pg_dump --format=custom --file=/backups/manual.dump` |
| Make all previews again | `docker compose -f deploy/compose.yaml exec app magellanica-photos render-previews` |

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
docker compose -f deploy/compose.yaml exec app magellanica-photos render-grid
```

The running server makes them in the background, from the viewer previews, after any new photos waiting for processing. Each photo keeps its preview format. Running the command again while jobs are waiting does not queue a photo twice. An open timeline picks up the new previews without reloading.

## Importing from a folder

For large collections, this is faster than uploading through the browser.

1. Copy the photos into the import folder on the NAS, in a subfolder per user if there are several.
2. In the web interface, as an administrator, open the account menu and choose **Import from folder**.
3. Choose the owner and the folder, and start the scan. One scan runs at a time, and it can be stopped.

The application reads the import folder as the user it runs as: UID 1000 with the compose files, and `apps` (UID 568) on TrueNAS. Files copied there by another user may not be readable by it. The scan then ends with errors, and its details list the files with "permission denied". Make them readable by that user, on TrueNAS for example in the Permissions editor of the dataset, and start the scan again.

The photos are copied into the library, and the source folder stays as it was. You can delete it after the scan, or keep it and scan it again after adding files: photos that are already in the library are not added twice. A scan reads every file in the folder each time, even on a repeat run, but writes only the new ones.

A scan does not bring back photos that the owner moved to the trash or deleted permanently. To get one back, the owner restores it from the trash or uploads the file through the browser.

Hidden files, symbolic links and folders whose name starts with `@` or `#`, such as `@eaDir` and `#recycle`, are left out, and so are the `.aae` files in which Apple Photos keeps its edits next to an exported original.

A scan first lists the files, waits for `IMPORT_STABLE_SECONDS`, and skips any file whose size or modification time has changed since. The report lists these files, and the next scan picks them up, so it is safe to start a scan while files are still being copied.

A scan continues after a restart, from where it stopped. After a clean stop, this happens shortly after the next start. After a crash or power loss, it can take up to 30 minutes.

### Google Takeout

To move a library from Google Photos, export it at [takeout.google.com](https://takeout.google.com) and copy the parts, `.zip` or `.tgz`, into the import folder as they are: there is no need to unpack them. An export that was unpacked already works too. Then choose **Google Takeout exports** under **Contents** in the scan form.

The import reads the JSON files Google puts next to the photos. A photo without a date of its own, or with one without a time zone, takes the date from Google, and a photo without a location takes the place added in Google Photos. Folders with an album's file become albums of the owner, found by their title, so importing again fills the same albums. Descriptions, favorites and the archive come across, and photos Google had in the trash go to the trash. An edited copy is shown over its original, and the owner can switch to the original in the viewer. **Album photos in the trash** chooses what happens to a photo of an album that the owner has already moved to the trash.

Several exports may sit in the folder together, such as an export of the whole library and later ones: a photo is added once, and the newest export's details win. The scan's summary counts the JSON files, the edited copies, the photos without JSON and the albums.

## Library folders

An import copies the photos into the library. A library folder is read where it is: the files stay in their folder, nothing is copied, and the application follows the folder as files are added, edited, moved and deleted there. This suits a share that another program works in, such as the folder of a Lightroom Classic catalog's originals and exports, where copying would double the space the files take.

Put the folders under `FOLDERS_DIR`, which is mounted read-only at `/data/folders`, and as an administrator open **Library folders** in the account menu. Choose the owner, a folder and how often to scan it, which is every 10 minutes by default and at least a minute. The first scan reads every file once and can take a long time for a large share; later scans read only the folders and the files that changed, by size and modification time. **Scan now** starts one at once. The photos belong to the owner like any other: the library folder only says where their files are, and other users see them through albums and shares.

- **Edits.** A file that changes at the same path stays the same photo, with its albums, tags and shares, and its previews are made again. A file whose new content is another photo of the owner is not merged into it: the page lists it under **Problems**.
- **Moves.** A file that appears at a new path when the old one is gone, in this folder or in another of the owner's, is the same photo moved. Nothing is processed again.
- **Missing files.** A file that two scans in a row do not find sends its photo to the trash, where it shows "the file is not available" and keeps its previews. When the file comes back, the photo comes out of the trash, unless its owner put it there. If an identical copy of the file lies elsewhere in a library folder, the photo is read from the copy and goes nowhere. A folder that cannot be read or lists no files, as an unmounted share does, stops the scan without marking anything.
- **Deleting.** The folder is mounted read-only. Moving a photo to the trash and deleting it permanently never touch its file, and a photo deleted permanently stays deleted even if its file is edited later. A copy of a file the owner already has is skipped, and **Files** lists these and the files that are not photos or videos, such as PSD and TIFF, which are read again only when they change.
- **Links.** Symbolic links are not followed: a link left in the share is skipped when scanning, and a file swapped for a link stops being readable. This is not a defense against somebody who can write to the share and means harm, who could still race the programs that read a file. A library folder's share may be written only by its owner and by administrators.
- **Removing a folder.** A folder with photos cannot be removed. Delete the photos first; their files are not touched.

On Linux, the application reads the folder as the user it runs as, UID 1000 or `apps` (568), so it needs read access there. On TrueNAS, files that a Mac writes to the share over SMB get their permissions from the dataset's ACL: give `apps` an inheritable entry that can read and traverse, or every new file shows up under **Problems** as unreadable. The originals in a library folder are not in the nightly dump or in the library's snapshots: back them up with the share. The dump holds only which file is which photo.

## Uploading from a phone over WebDAV

Sync apps such as PhotoSync and FolderSync can upload photos to `https://<address>/webdav/`, or `https://<address>:10304/webdav/` when [running from an image](#running-from-an-image).

FolderSync needs HTTPS: given an `http://` address, it still tries TLS and reports "unable to parse TLS packet header". With the certificate from Caddy's local CA, turn on "Allow self-signed certificate" in the account settings.

1. In the web interface, open **Profile**. Under **Upload from a phone**, choose **New app password** and name the device.
2. In the app, add a WebDAV server with the address, your login and the app password. The account password does not work there.
3. Set the app to upload only. Photos and videos are accepted, other files are refused. The video of an iPhone Live Photo is paired with its photo and plays as its motion, as long as the app keeps Apple's `ContentIdentifier` tag in both files.
4. To sort uploads into albums, give each folder pair in the app its own server folder, such as `/Camera` and `/Telegram`. Then, under **Albums for folders** in the profile, choose **Link a folder** and pick the folder and the album. A folder can be linked before the app creates it, and `/` links the whole tree.

Files deleted in the app, or by the app's sync, disappear from the WebDAV folder and stay in the library. Photos in the trash stay there when the app sends them again. A photo can be up to 512 MiB and a video up to 4 GiB.

## Live Photos

A Live Photo plays its motion only when both of its files reach the library, the photo and the MOV video. They pair in either order, from any of the upload paths.

- **From an iPhone,** upload over [WebDAV](#uploading-from-a-phone-over-webdav) with an app that sends a Live Photo as its photo and its video.
- **From a Mac,** select the Live Photos in Photos and choose **File → Export → Export Unmodified Original**. This gives `IMG_0001.HEIC` and `IMG_0001.MOV`. Upload both through the browser, or import the folder.
- **Uploading through the browser's file picker** brings only the photo. The picker on macOS and on the iPhone hands a web page the still image alone, often converted to JPEG. Such a photo pairs later if its MOV is uploaded too.

The picker also exports a video anew each time it is chosen, with the export time written into the file. So the same video chosen twice from Photos is stored twice, as the two files differ. Videos chosen from the Finder, or exported once, are recognized as duplicates.

Revoke a password in the profile if a device is lost. Whoever has it can upload photos as that user and download the originals uploaded over WebDAV, but not log in to the web interface.

The server logs every WebDAV request with its method, path, status and client, to see how real clients behave.

## RAW photos

Upload NEF, NRW, CR2, CR3, ARW, DNG (including Apple ProRAW), RAF, ORF, RW2, PEF and SRW through the browser, WebDAV or an import. Run a new folder or Takeout import to pick up RAW files counted as unsupported by an earlier version.

For the viewer, the server uses the camera's largest embedded JPEG if it reaches the configured viewer size. Otherwise the server uses LibRaw 0.22.3. Mosaic sensors above 120 MP and up to 240 MP use half-size processing. Larger files and files with unsupported formats or missing camera colour data can still show an embedded JPEG. If neither source works, the photo reports a processing error. The download button saves the original.

A full decode holds memory in proportion to the sensor: about 1.4 GB for 61 MP and 2.4 GB for 102 MP (measured on 2026-11-21). Only one runs at a time, whatever `WORKERS` is. The application container is limited to 4 GB (`MEMORY_LIMIT`), so a decode that goes wrong cannot take the NAS's memory. If the application goes over the limit, the kernel stops it, Docker starts it again, and the job queue processes the photo again. To see whether that happened, run `docker inspect <container> --format '{{.State.OOMKilled}}'`; `docker stats` shows the current use and the limit. Its figure includes the cache of recently read files, which the kernel gives back when needed; the process's own memory is the `anon` line of `docker exec <container> grep -E '^(anon|file) ' /sys/fs/cgroup/memory.stat`. Memory freed after a decode often stays with the process, so the use seen after an import is not a leak. The image sets `MALLOC_ARENA_MAX=2`, which keeps glibc from holding freed memory in one arena per thread: on 2026-11-28 the process held 680 MiB after hashing the previews of a whole library without it, and 120 MiB with it while an upload of many photos was processed.

A RAW and JPEG from the same owner are paired if the base name, capture time including sub-seconds, and camera make and model match. The files can come from different directories or uploads. The JPEG stays in the library view. Its owner can switch to RAW in the viewer. Both files go into and out of the trash together.

## Search and people

Search by description, similar photos and People need the ML service, which runs on another machine of the home network, such as a Mac or a computer with a graphics card, as described in the [development guide](development.md#ml-service). Without it the library works as before.

Start the service on that machine with a token, and with `ML_FACES=1` for People:

```sh
ML_TOKEN=<token> ML_FACES=1 make ml
```

The first start downloads the models from Hugging Face, about 1.5 GB for search and 40 MB for faces. Then set `ML_URL` to the service's address, such as `http://mac.local:8090`, and `ML_TOKEN` to the same token for the application, in `.env` or in the TrueNAS YAML under `app` → `environment`, and restart it. The application finds the service within 30 seconds. The administration page shows the progress of search, and People fill in from the photos already in the library. Old and faded photos where no face is found at once are checked again with stretched contrast, which also finds faces in flowers and leaves; such faces join People only from the confidence set under People on the administration page (0.75 by default, lower it for a library of film scans). Update the ML service together with the application: People pause until it reports face scores.

The token travels over plain HTTP, and face vectors are biometric data, so keep the service on the home network.

## Signing in with a provider

Besides a login and a password, people can sign in through an OpenID Connect provider: Google, or one you run yourself, such as Authelia, Authentik or Keycloak. A button for each provider appears on the sign-in page. The password form stays, and it is the way in for an administrator when a provider is down.

Register the application with the provider as a web application, with this redirect address, built from the address you open the application at:

```
https://photos.example.org/api/v1/auth/oidc/1/callback
```

The number is the provider's own: `1` for `OIDC_1_*`, `2` for `OIDC_2_*`. Then set these in `.env` next to the Compose file:

| Variable | Purpose |
|---|---|
| `PUBLIC_URL` | The address people open the application at, without a path. The redirect address is built from it, so it must match what the provider has registered, character for character |
| `SESSION_SECRET` | At least 32 random characters, such as the output of `openssl rand -hex 32`. It signs the cookie that carries a sign-in between the application and the provider. Changing it only cancels the sign-ins in progress |
| `OIDC_1_ISSUER` | The provider's issuer address. For Google it is `https://accounts.google.com`; for the others, the address its documentation gives, where `/.well-known/openid-configuration` answers |
| `OIDC_1_CLIENT_ID`, `OIDC_1_CLIENT_SECRET` | From the provider's registration of the application |
| `OIDC_1_NAME` | The label of the button. Defaults to "Single sign-on" |

`OIDC_2_*` sets a second provider in the same way. The application refuses to start when a provider is set without `PUBLIC_URL` and a long enough `SESSION_SECRET`, and says which is missing.

Google accepts only redirect addresses on a public domain over HTTPS, apart from `localhost` for trying it out, so an installation reached by an IP address or `truenas.local` cannot use it. A provider you run yourself has no such limit, but the application must trust its certificate: a provider with a certificate from a private authority is not accepted yet.

On the administration page, **Sign-in** decides who may use a provider:

- **A first sign-in creates an account** is off by default. Without it only people whose accounts already exist can sign in, after an administrator or they themselves linked the provider to the account in the profile. With it on, anyone the provider admits gets an account with the role user, which makes the installation open to everyone who has a Google account unless the list below narrows it. It is meant for a public demo.
- **Addresses and domains** narrow account creation to people whose provider-verified email is on the list, such as `family.example` or `anna@gmail.com`.
- **Passwords only for administrators** closes the password form to everyone else.

An account made through a provider has no password; its login comes from the provider's username, else from the part of its email before the at sign, else from its name, with a number added when the login is taken. Linking a provider to an account that exists is done by the account's own user, signed in with the password, in the profile, so that a provider which lets anyone claim an address cannot take an account over.

An account made through a provider can set a first password in its profile, so that it has a way in when the provider is down and can turn on two-factor sign-in. The profile sends the user to the provider to sign in again, and a password is accepted for five minutes after the provider confirms it is the same account that is linked here. Providers that ignore the request to ask again (the application sends `prompt=login` and `max_age=0`, and checks `auth_time` when the provider sends it) count the existing sign-in, so for them this proves only that the session and the provider's own session belong to the same person.

## Public links to albums

The owner of an album makes a link in the album's menu, **Public links**. Anyone with the link sees the album with no account: its name and its photos as previews, and its videos play. They do not get the originals, the file names, camera details or places: the previews carry no EXIF, and the page is given only the date taken. A link may expire after 7 to 365 days and may have a password, which visitors enter once (it holds for 12 hours). An album has at most 10 links. Revoking a link closes it at once, as does deleting the album or disabling its owner. Photos in the trash or still being processed are not shown.

Set `PUBLIC_URL` in `.env`, as for [signing in with a provider](#signing-in-with-a-provider), to the address visitors reach the application at, such as `https://photos.example.org`. The link shown to the owner is built from it, never from the headers of a request. Without it the link is built from the address the owner has the page open at, which a home address (`truenas.local`, an IP address) makes useless outside the house, and the dialog says so.

**What a tunnel or proxy should open.** Visitors need only three groups of paths, so a tunnel or proxy that faces the internet can pass those and refuse the rest, and the rest of the application, the sign-in page included, stays reachable from home only:

| Path | What it is |
|---|---|
| `/s/<token>` | The page |
| `/api/v1/public/<token>/...` | What the page reads: the album, its photos, previews and videos |
| `/assets/...` | The application's scripts and styles |

For a Cloudflare Tunnel set by a configuration file, give the public name only those paths and everything else a 404. `path` is a regular expression:

```yaml
ingress:
  - hostname: photos.example.org
    path: ^/(s/|api/v1/public/|assets/)
    service: http://127.0.0.1:10303
  - service: http_status:404
```

You then open the application itself from home, at its home address, and the public name serves only links. Without such a filter the whole application, with its sign-in page, is reachable by the public name, as it is for the public demo.

**Limits.** A link's token is 43 random characters, and an unknown, expired or revoked link all answer 404, so one cannot be told from another. Wrong passwords of one link slow down as passwords of a login do (after 5 wrong, 30 seconds and then longer), counted for the link and not for an address, as the application behind a proxy sees only the proxy. Nothing else limits the requests of visitors, so put a rate limit in front when the public name is open to everyone: Cloudflare's rules do it without changes here. A visitor can save a photo's preview and the video that plays, as with any page; what a link withholds is the original file and its metadata, not the picture.

## Two-factor sign-in

A user can ask for a code from an authenticator app (Google Authenticator, Aegis, 1Password and the like) after the password. It is off until the administrator sets `TOTP_KEY` in `.env` next to the Compose file:

```sh
openssl rand -base64 32
```

`TOTP_KEY` is at least 32 characters. The application keeps each user's secret encrypted under it, so it cannot be turned on without it, and the application refuses to start when it is set to something shorter.

**Keep `TOTP_KEY` with the database backups.** A dump restored on another machine without the key cannot open the secrets. At start the application finds the users whose secret it cannot open, turns two-factor sign-in off for them, so that they are not locked out, and writes a warning with their logins. Starting without `TOTP_KEY` at all does the same for everyone, so a key lost from `.env` by mistake leaves passwords alone protecting the accounts until each user sets it up again. Changing the key has the same effect.

Each user turns it on in the profile: scan the picture or type the key into the app, enter a code to confirm, and save the ten recovery codes, which are shown once. A recovery code signs in once when the app is lost. Turning it off asks for the password and a code. If both the app and the codes are lost, an administrator resets it on the users page, and the user signs in with the password alone and sets it up again.

It applies to the password form only. A sign-in through a provider is not asked for a code, as the provider answers for the second factor, and WebDAV app passwords are random and can be revoked per device. After 5 wrong codes for one user the code step waits 30 seconds, then longer, as the password form does. The wait is kept in memory, so a restart clears it. An authenticator code works once, and the code of the time step that turned it on is spent, so wait for the next code when signing in right after.

## Backups

| What | Where | How |
|---|---|---|
| Database | `BACKUP_DIR` | Dumped every night by the `backup` container |
| Originals | `LIBRARY_DIR` | Snapshots of the folder, for example ZFS snapshots on TrueNAS |
| Originals in [library folders](#library-folders) | `FOLDERS_DIR` | With the share they live in, not with the library: the dump knows only which file is which photo |
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
   docker compose -f deploy/compose.yaml exec app magellanica-photos render-previews
   ```

Logins and passwords are those of the dump. The previews are made in the background, after any new photos, and the **Queue** section of the administration page shows how far they are. Until a photo has them, its tile stays empty. A photo whose original is missing shows as failed.

On TrueNAS, run the commands in the shell with `sudo docker exec` and the container name in place of `docker compose -f deploy/compose.yaml exec` and the service name, as in [Running on TrueNAS](#running-on-truenas). Stop and start the application container with `sudo docker stop` and `sudo docker start`.

## Building on another machine

Instead of building on the NAS, you can build the image elsewhere and copy it over. `make image` builds for `linux/amd64`; for an ARM NAS, run `make image PLATFORM=linux/arm64`:

```sh
make image
docker save magellanica-photos:latest | ssh <nas> docker load
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

It uses the image that CI publishes to `ghcr.io/magellanica-app/magellanica-photos`, for `linux/amd64` and `linux/arm64`; Docker picks the one for the NAS. The `arm64` image has not been tried on an ARM NAS yet. The tags are:

- `latest`, the newest build of `master`;
- `sha-<commit>`, for example `sha-6593702`, which never changes;
- `X.Y.Z`, for example `0.1.0`, a release, which never changes;
- `X.Y`, the newest release of that minor version.

The transcoder, which makes H.264 copies of videos on the graphics as described in [Video copies](#video-copies), has an image of its own for each maker of graphics, `ghcr.io/magellanica-app/magellanica-photos-transcoder`, built by hand for `linux/amd64` only. The tags are `latest-intel` and `latest-amd`, moved when they are built from `master`, and `X.Y.Z-intel` and `X.Y.Z-amd` for a release.

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
docker compose exec app magellanica-photos create-admin admin
```

To update:

```sh
docker compose pull
docker compose up -d
```

To run a specific release or build, set `IMAGE_TAG=0.1.0` or `IMAGE_TAG=sha-<commit>` in `.env`. Going back to an older build is safe only if no migrations were added since: migrations run forward only, so after a new one an older build may not start against the database.

### Video copies

Phones record HEVC, which some browsers cannot play: the viewer then shows the poster and offers the download. With graphics that encode H.264, the application can make an H.264 copy that every browser plays, with the shorter side at 1080 and HDR tone mapped to SDR. The copy is made on the graphics, leaves the CPU almost free, and goes into the cache next to the previews, at about a seventh of the original's size. On the N150 a minute of 4K HDR from an iPhone takes about 32 seconds, as measured in the [benchmark](benchmark.md#video-results). Copies are made for HEVC, VP9 and AV1 videos, for HDR videos in H.264, which Firefox shows grey, and for the motion of motion photos and Live Photos; other H.264 videos need none.

The copies are made by the transcoder, a service next to the application whose image carries the drivers for the graphics:

| Image | Graphics |
|---|---|
| `ghcr.io/magellanica-app/magellanica-photos-transcoder:latest-intel` | Intel, with Intel's iHD driver and OpenCL runtime |
| `ghcr.io/magellanica-app/magellanica-photos-transcoder:latest-amd` | AMD, with Mesa's radeonsi driver. It makes no copies of HDR videos yet: they keep the poster and the download |

The transcoder reads the videos from the library and the cache, and writes the copies into the cache, at the same paths as the application. It runs as the same user, as it writes into files the application made, and it needs the render node of the graphics and the group that may open it:

1. Find the group of the render node on the NAS: `stat -c %g /dev/dri/renderD128`.
2. Choose a token, such as the output of `openssl rand -hex 32`.
3. Add the transcoder to the compose file, with the image for your graphics, the group from step 1 and the token, and give the application its address and the same token:

   ```yaml
   services:
     app:
       environment:
         TRANSCODER_URL: http://transcoder:8091
         TRANSCODER_TOKEN: change-me

     transcoder:
       image: ghcr.io/magellanica-app/magellanica-photos-transcoder:latest-intel
       restart: unless-stopped
       environment:
         TRANSCODER_TOKEN: change-me
       devices:
         - /dev/dri/renderD128:/dev/dri/renderD128
       group_add:
         - '107'
       volumes:
         - ${LIBRARY_DIR:-./data/library}:/data/library:ro
         - ${FOLDERS_DIR:-./data/folders}:/data/folders:ro
         - ${CACHE_DIR:-./data/cache}:/data/cache
   ```

   Give the transcoder the same `user:` as the application, if it has one, and the same folders. Its port is not published: only the application calls it.

4. After `docker compose up -d`, the transcoder's log says `graphics can make H.264 copies`, with the driver, or why they cannot. The application asks it every 30 seconds; its log and the administration page, under **Videos**, say whether copies can be made.

The setting there decides when copies are made. **When someone plays the video**, the default, makes the copy of a video the first time someone starts it and the browser asks for it; until then the original plays where the browser can, and otherwise the viewer waits for the copy. **After upload** makes copies in the background as videos come in, and of the videos uploaded before. Either way, copies are made one at a time, and a video someone is waiting for goes first. Turning copies off keeps the ones already made. While the transcoder is stopped, no copies are made and none fail; those that waited for it are made when it is back, on upload, or the next time someone plays the video.

The transcoder's settings:

| Variable | Default | Purpose |
|---|---|---|
| `TRANSCODER_TOKEN` | required | Bearer token the application sends |
| `VAAPI_DEVICE` | `/dev/dri/renderD128` | Render node of the graphics |
| `LIBRARY_DIR`, `FOLDERS_DIR`, `CACHE_DIR` | `/data/library`, `/data/folders`, `/data/cache` | The application's folders, at the same paths as in its container. The library folders are read only |
| `TRANSCODER_ADDR` | `:8091` | Address it listens on |
| `LOG_LEVEL`, `LOG_FORMAT` | `info`, `text` | As for the application |

## Running on TrueNAS

On TrueNAS, the setup from [Running from an image](#running-from-an-image) runs as a custom app. TrueNAS does not read a `.env` file, so the values go into the YAML itself.

Both containers run as the TrueNAS `apps` user and group, UID and GID 568. For H.264 copies of videos, add the transcoder as described [below](#video-copies-on-truenas). Create a dataset for the application, and inside it a dataset or folder for each of `library`, `cache`, `import`, `postgres`, `backups` and `caddy`, and, to read a share where it is, [library folders](#library-folders) under `folders` or a dataset you mount there. Separate datasets for `library` and `backups` can be snapshotted on their own, as described in [Backups](#backups). In **Datasets**, select the parent dataset and choose **Edit** on the **Permissions** card. Set the owner and the owning group to `apps`, give `owner@` full control and `group@` modify, and apply the permissions to the child datasets. The application only reads `import` and `folders`, so files copied there by other users need to be readable by `apps`.

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
      - /mnt/pool/magellanica/postgres:/var/lib/postgresql/data
    healthcheck:
      test: [CMD-SHELL, pg_isready -U photos -d photos]
      interval: 15s
      timeout: 30s
      retries: 20
      start_period: 60s

  app:
    image: ghcr.io/magellanica-app/magellanica-photos:latest
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
      - /mnt/pool/magellanica/library:/data/library
      - /mnt/pool/magellanica/cache:/data/cache
      - /mnt/pool/magellanica/import:/data/import:ro
      - /mnt/pool/magellanica/folders:/data/folders:ro
    deploy:
      resources:
        limits:
          memory: 4G

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
      - /mnt/pool/magellanica/backups:/backups
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
      - /mnt/pool/magellanica/caddy:/data

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

### Video copies on TrueNAS

To make H.264 copies as described in [Video copies](#video-copies), add two lines to the `environment` of `app` in the YAML:

```yaml
      TRANSCODER_URL: http://transcoder:8091
      TRANSCODER_TOKEN: change-me-too
```

and the transcoder under `services`, with the image for your graphics, the group of the render node from `stat -c %g /dev/dri/renderD128`, the same token and the datasets of the application:

```yaml
  transcoder:
    image: ghcr.io/magellanica-app/magellanica-photos-transcoder:latest-intel
    restart: unless-stopped
    user: '568:568'
    group_add:
      - '107'
    devices:
      - /dev/dri/renderD128:/dev/dri/renderD128
    environment:
      TRANSCODER_TOKEN: change-me-too
    volumes:
      - /mnt/pool/magellanica/library:/data/library:ro
      - /mnt/pool/magellanica/folders:/data/folders:ro
      - /mnt/pool/magellanica/cache:/data/cache
```

If the transcoder cannot open the files, because a path differs from the application's or it runs as another user, no copies are made and none fail: the administration page shows the reason under **Videos**.

Create the first administrator from the TrueNAS shell. Docker names the container after the app, so look it up first:

```sh
sudo docker ps --format '{{.Names}}' | grep app
sudo docker exec -it <container> magellanica-photos create-admin admin
```

### Updates

TrueNAS compares the `latest` image on the NAS with the one in the registry and shows **Update** on the app when they differ. Updating pulls the new image and recreates the containers, and migrations run on start. A pinned tag such as `0.1.0` or `sha-<commit>` never shows an update.

The check needs **Check for image updates** turned on in the **Apps** settings. It runs once a day and when the Docker service starts. To check right away, run this in the TrueNAS shell and then reload the Apps page:

```sh
sudo midclt call app.image.op.check_update
```

On newer TrueNAS releases the method is `app.image.check_update`.

Since stage 7 the database needs the `vector` extension, and the `postgres` service uses `pgvector/pgvector:pg18-trixie` instead of `postgres:18`. Before updating an installation from an older YAML, change that line in **Edit**. Both images are PostgreSQL 18 on the same Debian release, so the existing database starts as it is. Without the change the application stops at start with an error about the `vector` extension. The `backup` service stays on `postgres:18`.
