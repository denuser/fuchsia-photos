# Changelog

Changes in each release. Add changes under "Unreleased" as they are made; `make release` turns that section into the version, and CI takes the release notes from it. See [Releases](docs/development.md#releases).

## Unreleased

- A private People page groups faces in the owner’s photos. Owners can name or merge groups, move a face and ignore an incorrect detection. Background indexing resumes when the ML host is available. The optional self-hosted ML service can detect faces and return face vectors with YuNet and SFace. It requires explicit local model paths and an ML token. See [Face inference](docs/development.md#face-inference).

- Similar photos: a page that lists copies, edited versions and nearly identical frames (Duplicates), and other shots of the same scene (Similar shots), found from the vectors of the ML service and a hash of each preview. For a group you keep one photo and the others go to the trash with their albums, tags, favorites and shares moved to the one you keep, or you mark the group as not the same. Nothing changes without your choice, and a pair you marked is not offered again. Photos are looked at in the background after the ML service has made their vectors, and the previews of the photos already in the library are hashed in the first run. See [Similar photos](docs/architecture.md).
- Fixes found while checking sign-in with Google: the administration page no longer fails to draw once account creation through a provider is on, and unlinking a provider that is the only way in now says so near the button.
- Library folders: an administrator adds a folder of a new read-only mount, `FOLDERS_DIR` (`/data/folders`), with an owner and a scan interval, and its photos are read where they are, without a copy in the library. Scans run on a schedule and read only the folders and the files that changed. A file edited in another program, such as Lightroom, stays the same photo with its albums, tags and shares, and its previews are made again; a moved file follows its photo; a file that stays away for two scans sends its photo to the trash, where it shows that the file is not available, and the photo comes back with the file. Files that are not photos, that could not be read or that hold another photo's content are listed on the page. Links in the share are never followed. See [Library folders](docs/deployment.md#library-folders).
- To use it on an existing installation, add the mount to the application and to the transcoder: `${FOLDERS_DIR:-./data/folders}:/data/folders:ro` in a Compose file, or a dataset mounted at `/data/folders` on TrueNAS, and read [Library folders](docs/deployment.md#library-folders) for the permissions. Nothing changes without it. Until the transcoder is rebuilt and says it reads the folders, videos of library folders get no H.264 copy, and none fails. Three migrations run forward only on the first start.

## 0.4.1 - 2026-11-24

- Six tints for the Glass background (Fuchsia, Orchid, Raspberry, Twilight, Rose, Plum), chosen in the appearance menu once Glass is on and kept per device. Tables no longer draw a solid sheet over the glass panel.

## 0.4.0 - 2026-11-24

- An optional activity journal for the public demo: with `ACTIVITY_LOG=true`, web sign-ins and uploads are recorded in the `activity` schema of PostgreSQL, with the outcome, the browser and system family and a random session ID, and without IP addresses or credentials. Administrators read it on a new Activity journal page. It is off by default. See [Demo activity journal](docs/development.md#demo-activity-journal).

## 0.3.0 - 2026-11-22

- The project is renamed from Fuchsia Photos to Magellanica Photos. The command is now `magellanica-photos`, the images are `ghcr.io/magellanica-app/magellanica-photos` and `magellanica-photos-transcoder` (the old images stay but are no longer updated), and the session cookie is `magellanica_session`, so everyone signs in once more. To move an existing installation, change the `image:` lines of your Compose file or TrueNAS app and use `magellanica-photos` in `docker compose exec`; the data and settings are unchanged. Old GitHub links redirect to the new repository.

## 0.2.0 - 2026-11-22

- A Map page with the photos that have a location, in clusters that zoom in and open their photos, drawn with MapLibre GL JS on OpenStreetMap tiles loaded by the browser. The viewer's coordinates link to it. It follows the administrator's setting that hides locations.
- Memories: a strip of "N years ago" cards above the timeline and a Memories page with the photos taken on this day in earlier years, or on the closest day within three days.

## 0.1.1 - 2026-11-22

- RAW previews tolerate warnings in the camera JPEG, use LibRaw 0.22.3 for sensor decoding and reduce memory use on very large mosaic sensors.
- Very large RAW files no longer go over the default 4 GB memory limit: sensors above 120 MP are decoded at half size, and larger files fall back to the camera's embedded JPEG. This lifts the first known limitation of 0.1.0. All 20 difficult RAW samples of the [NAS comparison](https://github.com/denuser/fuchsia-photos/blob/master/docs/benchmark.md#difficult-raw-files-with-default-viewer-settings) got a preview on the NAS, with the application and PostgreSQL together at a peak of 1.4 GB.

## 0.1.0 - 2026-11-21

The first release.

- Upload from the browser, with previews made in the background and progress shown as it goes
- Upload from phones over WebDAV, with app passwords and folders linked to albums. Checked with PhotoSync and FolderSync
- Import from a folder on the NAS, run by the administrator, which leaves the folder untouched and skips photos already imported or permanently deleted
- Import from Google Takeout, reading `.zip` and `.tgz` parts or an unpacked export: the dates and places Google kept, albums, descriptions, favorites, the archive and the trash. An edited copy is shown over its original, which the owner can switch to in the viewer. Several exports, and the same export imported again, give one library
- A timeline loaded month by month, with a year scrubber and filters by date, tag, camera and lens, and a "Mine / All" switch for the photos shared with the user
- A viewer with zoom and pinch, camera details and the location, and the original on zoom when the user chooses it
- Albums with covers, personal tags, bulk actions and a trash
- Favorites and an archive, each user's own, and descriptions in the viewer, which the owner edits
- Sharing of photos and albums with all users or by login, read-only for the recipient, and a "Shared with me" section
- Users managed by the administrator, with administrator and user roles and an administration page with all photos, preview settings and the job queue
- Search by description and similar photos, with an optional ML service on SigLIP 2
- Nightly database dumps with a tested restore, request logs in text or JSON
- The interface in ten languages (English, Russian, Simplified Chinese, Spanish, Brazilian Portuguese, German, French, Japanese, Korean and Italian), light, dark and high contrast themes and a glass mode
- JPEG, PNG, WebP and HEIC photos, and MP4 and MOV videos up to 4 GiB, which play in the viewer, with a download when the browser cannot play the codec
- RAW photos from cameras and phones, with browser previews, original downloads and a viewer switch between matched JPEG and RAW files. A RAW badge marks them and RAW + JPEG pairs in the grid, and the type filter finds them
- The motion of Google and Samsung motion photos, played in the viewer by a button, including for photos uploaded before
- iPhone Live Photos as one item: the video, uploaded before or after its photo, plays as the photo's motion and leaves the lists
- H.264 copies of HEVC, VP9 and AV1 videos, of HDR H.264 videos and of the motion of photos, made on Intel graphics by a transcoder service so that every browser plays them, at 1080p with HDR tone mapped, when the video is first played or after upload, as the administrator chooses
- Images for `linux/amd64` and `linux/arm64`, and `amd64` images of the transcoder, `fuchsia-photos-transcoder:latest-intel` with VAAPI and OpenCL, and `latest-amd` with Mesa's driver for SDR videos, built by hand

### Known limitations

- Very large RAW files that need a full decode can exceed the container's default 4 GB memory limit and trigger repeated restarts as the processing job retries. Try importing a small sample from your camera first. See [RAW photos](https://github.com/denuser/fuchsia-photos/blob/master/docs/deployment.md#raw-photos).
- Folder imports copy originals into the managed library. Lightroom XMP develop settings are not applied to previews.
