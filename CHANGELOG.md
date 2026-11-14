# Changelog

Changes in each release. Add changes under "Unreleased" as they are made; `make release` turns that section into the version, and CI takes the release notes from it. See [Releases](docs/development.md#releases).

## Unreleased

The first release.

- Upload from the browser, with previews made in the background and progress shown as it goes
- Upload from phones over WebDAV, with app passwords and folders linked to albums. Checked with PhotoSync and FolderSync
- Import from a folder on the NAS, run by the administrator, which leaves the folder untouched and skips photos already imported or permanently deleted
- A timeline loaded month by month, with a year scrubber and filters by date, tag, camera and lens, and a "Mine / All" switch for the photos shared with the user
- A viewer with zoom and pinch, camera details and the location, and the original on zoom when the user chooses it
- Albums with covers, personal tags, bulk actions and a trash
- Sharing of photos and albums with all users or by login, read-only for the recipient, and a "Shared with me" section
- Users managed by the administrator, with administrator and user roles and an administration page with all photos, preview settings and the job queue
- Search by description and similar photos, with an optional ML service on SigLIP 2
- Nightly database dumps with a tested restore, request logs in text or JSON
- English and Russian interface, light, dark and high contrast themes and a glass mode
- JPEG, PNG, WebP and HEIC photos, and MP4 and MOV videos up to 4 GiB, which play in the viewer, with a download when the browser cannot play the codec
- The motion of Google and Samsung motion photos, played in the viewer by a button, including for photos uploaded before
- iPhone Live Photos as one item: the video, uploaded before or after its photo, plays as the photo's motion and leaves the lists
- H.264 copies of HEVC, VP9 and AV1 videos and of the motion of photos, made on Intel graphics by a transcoder service so that every browser plays them, at 1080p with HDR tone mapped, when the video is first played or after upload, as the administrator chooses
- Images for `linux/amd64` and `linux/arm64`, and `amd64` images of the transcoder, `fuchsia-photos-transcoder:latest-intel` with VAAPI and OpenCL, and `latest-amd` with Mesa's driver for SDR videos, built by hand
