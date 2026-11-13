# Roadmap

**Status:** stage 6, with the stage 7 spike in progress: search by description and similar photos, with the ML service on a Mac. Stage 8 has started: videos, motion photos and Live Photos upload and play. Login, user management, upload, processing, the timeline, albums, tags, search, bulk actions, the trash, sharing, folder scans and the preview settings work, with nightly database dumps, a tested restore, request logs and the job queue on the administration page. Phones upload over WebDAV, checked with PhotoSync and FolderSync on the NAS. The stage 0 benchmark set the preview format and worker count for JPEG photos.

Decisions are in the [architecture](architecture.md).

## Stages

- **Stage 0: skeleton and benchmarks.**
  - [x] Monorepo structure, compose, migrations, CI
  - [x] Choose the libvips binding
  - [x] Benchmark on the real NAS: JPEG photos, previews on the SSD, WebP and JPEG, 1 to 3 workers. HEIC was left out as not currently relevant
- [x] **Stage 1: core.** Login, user management, roles. Browser upload (3–4 files in parallel, retrying failed files). Ingest, queue, previews, virtual-scrolling timeline, viewer, processing progress.
- [x] **Stage 2: organization.** Albums, tags, search by date and tag, bulk actions, trash with soft delete.
- [x] **Stage 3: sharing.** Visibility for all users, sharing photos and albums by login, a "Shared with me" section, the admin view of everything. Test suite for the permission predicate.
- [x] **Stage 4: folder scan.** Admin only: fixed root, owner selection, safe re-runs thanks to hashing. Skips files still being copied (size and mtime unchanged for N seconds). Report in `import_runs`. A re-run must not restore photos the owner moved to the trash, unlike a browser upload.
- [x] **Stage 5: operations.** Nightly `pg_dump`, backup and tested restore instructions, logs and queue metrics.
  - Restore checked on 2026-11-10 with `deploy/compose.yaml`: a nightly dump from one installation restored on a clean one, with the originals copied over and the previews made again by `render-previews`
- [x] **Stage 6: WebDAV (hypothesis, starts as a spike).** Per-user upload folder, separate app password, files go through ingest. Tested with real mobile clients such as PhotoSync and FolderSync. If they work poorly, the feature is dropped and the outcome recorded here.
  - [x] Spike: the WebDAV tree at `/webdav/`, app passwords in the profile, uploads through ingest
  - [x] Tests with PhotoSync and FolderSync on the NAS, with the outcome recorded here
    - Checked on the NAS on 2026-11-10: uploads from both apps work, so WebDAV stays
    - FolderSync on Android connects only over HTTPS. With `http://` in the address it still starts a TLS handshake and fails with "unable to parse TLS packet header". A self-signed certificate works once "Allow self-signed certificate" is on. Checked against the development server on 2026-11-09, where uploads over HTTPS work
  - [x] Albums for WebDAV folders, as described under "WebDAV albums" above
- [ ] **Stage 7: ML (starts as a spike).** Text search and similar photos from one image embedding, as described under [Machine learning](architecture.md#machine-learning). The service runs on a Mac for the spike.
  - [x] Model: SigLIP 2 base (`google/siglip2-base-patch16-384`), chosen over so400m
    - Compared on 2026-11-10 on the development library of 117 photos with 8 Russian queries. Base ranked the matches first in more queries: so400m put the only dog third, after a black frame and a screenshot, and lost the fuchsia photos for "flowers". It understood whole scenes better, finding selfies against mountains for "mountains". So400m embedded 3.5 times slower, weighs 4.5 GB against 1.5 GB, and its text encoder would be much harder to move to the NAS
    - Text search needs a similarity threshold, or a query with no match returns the nearest photos anyway. With base, matches came down to a cosine similarity of 0.068 and unrelated photos up to 0.072, so no value separates them. 0.070 keeps nearly all matches and lets a few unrelated photos in at the end of the list. Query templates such as "a photo of …" and a threshold relative to each query's distances separated them worse
  - [x] The ML service in `ml/`, with the health check and both embedding endpoints
  - [x] `pgvector` image in every compose file, the embedding tables, the availability monitor and the embedding job
  - [x] Text search on a page of its own and similar photos in the viewer, in the API and the interface
  - [ ] Measure embedding speed on the Mac and search time on the NAS
    - On a Mac with an M5 Pro, the application embedded the 117 photos of the development library in 4 seconds with base and 14 seconds with so400m, reading viewer previews. The NAS is not measured yet
  - Later candidates, on the same embedding: near-duplicates and bursts, tag suggestions, separating screenshots and documents. Faces need a model of their own and are processed at home only, as face vectors are biometric data
- [ ] **Stage 8: video.** Upload and playback of videos from phones, through the same ingest path as photos. Transcoding is left out of the first step and decided after a benchmark on the NAS.
  - Decisions, settled on 2026-11-11:
    - Live Photos and motion photos: step 1 plays the video embedded in Android motion photos as the motion of the photo. The video file of an iPhone Live Photo comes in as a video of its own until a later step pairs it with its photo, the way Immich and Nextcloud Memories hide it behind the photo. Pairing needs both files in any order, from three sources, with the trash, sharing and every photo list taking the pair into account, which is most of the cost
    - A video may be up to 4 GB per upload, a photo stays at 512 MB. A failed upload starts again from the beginning, with no resuming
    - ffmpeg and ffprobe inside the application image, run as separate processes. A separate service would need the files sent over the network or a shared volume. On 2026-11-12 Debian's package gave way to a minimal build of the same release, which took the arm64 image from 932 MB to 448 MB on disk
    - Transcoding waits for the benchmark: no VAAPI driver and no `/dev/dri` in step 1
  - [ ] Step 1: accept and play
    - [x] Detect MP4 and MOV in ingest, next to HEIC, which shares the `ftyp` container, and accept them from the browser, WebDAV and folder scans
    - [x] A migration for the duration and whether an item is a video
    - [x] Processing: ffprobe for the size, duration and rotation, a frame from ffmpeg as the poster, and the grid and viewer previews made from it with libvips. The ML service embeds the poster, so search by description and similar items cover videos
    - [x] Dates: QuickTime `CreateDate` is in UTC by the standard, unlike EXIF dates. The application already reads a date without an offset as UTC, so it reads `CreationDate`, which an iPhone writes with the offset, then `CreateDate`, without exiftool's `QuickTimeUTC`
    - [x] Playback of the original with `<video>`, which the existing range support for originals allows to seek. When the browser cannot play the codec, such as HEVC in some browsers, show the poster and a download button
    - [x] Interface: a mark and the duration on grid tiles, the viewer showing the video in place of the zoomable photo, and swipes and keys that do not get in the way of the video controls
    - [x] Motion photos: the video embedded in a Google or Samsung motion photo is extracted during processing, kept next to the previews and played in the viewer. Photos uploaded earlier get it from a job of their own, without reprocessing the library
    - [x] Backend tests with a small sample video, and a video in the browser test
    - [x] Motion photo extraction checked on 2026-11-11 with the real files of [MotionPhotoSamples](https://github.com/NightMean/MotionPhotoSamples): a Pixel 9 Pro XL and Samsung phones and a tablet, in JPEG and HEIC, each giving a whole MP4. Most of them are HEVC
    - [x] Playback checked on 2026-11-12 in Safari on an iPhone and in Firefox
    - [x] Google's older MicroVideo motion photos, which give only `MicroVideoOffset`, checked on 2026-11-12 with `MVIMG_` files from [Wikimedia Commons](https://commons.wikimedia.org/w/index.php?search=MVIMG_&ns6=1): a Pixel 2, a Pixel 3 XL, a Nokia 3.1 Plus and an ASUS phone each gave a whole MP4. Two files kept the tag after an editor dropped the video and stayed still photos
    - [x] Huawei and Honor moving pictures, found by the `LIVE_` trailer at the end of the file, checked on 2026-11-12 with a Huawei MediaPad M5 file. Photos processed before are not backfilled
    - [ ] Check on real devices: the poster of an HDR video from an iPhone
      - A PQ video in H.264, not from an iPhone, got a tone-mapped poster with normal colours in the application image on 2026-11-12
  - [x] Live Photos: the iPhone photo and its video as one item, pairing files that come in either order, including those imported before, as described under [Live Photos](architecture.md)
    - Pairing checked on 2026-11-12 with the iPhone 15 pair of [MotionPhotoSamples](https://github.com/NightMean/MotionPhotoSamples): the HEIC and the HEVC MOV share the tag, and the video paired with the photo whichever came first
    - The motion checked on 2026-11-12 in Chrome on a Mac, with Live Photos from an iPad uploaded to the development stack. The browser's file picker gave only their photos, so their videos were uploaded too
    - The motion checked on 2026-11-12 in Safari and in Firefox
    - [ ] Check on real devices: an iPhone library uploaded over WebDAV
  - [ ] Step 2, if needed: transcoding
    - Planned on 2026-11-13: hardware encoding with `h264_vaapi` only, with no libx264 fallback on the CPU. An administrator turns it on in the administration page, where the graphics allow it. Without `/dev/dri`, or with the setting off, videos the browser cannot play keep the poster and the download, as in step 1
    - [x] Benchmark on the NAS with `scripts/video-bench.sh`, as described under [Video transcoding](benchmark.md#video-transcoding): 1080p and 4K HEVC to H.264 through VAAPI, with libx264 for comparison
      - Measured on 2026-11-13 with Jellyfin's test files: `h264_vaapi` makes a 1080p copy of 4K HLG video, as an iPhone records it, at 1.9 times real time with about one core busy, and of 4K SDR at 2.9 times. HLG needs `tonemap_opencl`, as `tonemap_vaapi` fails on it. libx264 ran at about 0.14 times real time on HDR
    - [x] Check the tone mapped copies by eye: on 2026-11-13 the copies of the HLG and PQ files looked right next to the originals
    - [ ] Check with an iPhone video that has rotation and sound
    - Decided on 2026-11-13: the hardware encoding ships in a separate amd64 image for Intel graphics, with a moving tag of its own next to `latest`, so the main image stays small and arm64 needs nothing. It carries libva, Debian's non-free iHD driver and Intel's OpenCL runtime, about 450 MB more. On arm64 videos stay as in step 1: no hardware encoder there works with upstream ffmpeg in a container
    - [ ] Depending on the result, a playable copy for videos the browser cannot play, made on a queue of its own with one worker and low priority, as folder scans are

## Later

Features people tend to expect before moving their photos to a new library. None is planned in detail or ordered yet.

- **Import from Google Takeout and iCloud.** Both exports keep dates, descriptions and locations in JSON sidecars or CSV files rather than in the photos, and leaving these services is a common reason to look for a self-hosted library.
- **Map.** Photos on a map by their GPS locations, next to the place names in the viewer from [improvement 8](improvements.md#8-place-names-in-the-viewer).
- **Duplicates.** Near-duplicates and bursts, from the stage 7 embedding.
- **Faces.** A model of their own, processed at home only, as face vectors are biometric data.

## Acceptance criteria

- **Benchmark (stage 0):** a benchmark report with real speeds on the NAS gives the figures under [Performance](../README.md#performance).
- **Backend:** `go test ./...` passes against a real PostgreSQL. Required scenarios: the permission matrix, duplicates, and a worker failure followed by a job retry.
- **End to end on the NAS:** after `docker compose up`, upload 1,000 photos from the browser. The UI stays responsive, a restart during processing loses no jobs, and every photo ends up with previews or a clear error.
- **Permissions (stage 3):** a second user gets a 404 for another user's original and previews by direct URL, and gets the files once they are shared.
- **Folder scan (stage 4):** a second run on the same folder creates no new records, and the source folder stays unchanged.
- **Backup (stage 5):** a dump restores on a clean installation.
