# Improvements

Improvements after stages 0 to 6 of the [roadmap](roadmap.md). Numbers are stable, as other documents refer to them, and do not give an order of priority. The status in this table and in each section are updated together.

| # | Improvement | Status | Left to do |
|---:|---|---|---|
| 1 | [Administration page](#1-administration-page) | Done | |
| 2 | [Timeline for large libraries](#2-timeline-for-large-libraries) | Done | |
| 3 | [Camera and lens filters](#3-camera-and-lens-filters) | Done | |
| 4 | [Shared photos in the timeline](#4-shared-photos-in-the-timeline) | Done | |
| 5 | [PatternFly look](#5-patternfly-look) | Done | |
| 6 | [Finding the same object or place](#6-finding-the-same-object-or-place-experiment) | Planned | The whole experiment |
| 7 | [Interface language](#7-interface-language) | Done | |
| 8 | [Place names in the viewer](#8-place-names-in-the-viewer) | Planned | Everything |
| 9 | [OPPO motion photos and older Huawei moving pictures](#9-oppo-motion-photos-and-older-huawei-moving-pictures) | Planned | A real OPPO file; a backfill for old Huawei photos |
| 10 | [Import from Google Takeout and iCloud](#10-import-from-google-takeout-and-icloud) | Takeout done, iCloud plan drafted | iCloud: a real export from the library's owner (step 0), then the build |
| 11 | [Grid tile size](#11-grid-tile-size) | Done | |
| 12 | [Upload into an album, and whole folders](#12-upload-into-an-album-and-whole-folders) | Done | Check "Choose folder" on iPhone and Android |
| 13 | [Public links to albums](#13-public-links-to-albums) | Planned | Open questions, then a plan |
| 14 | [Memories: on this day](#14-memories-on-this-day) | Done, released in 0.2.0 | |
| 15 | [Wait for the copy of an HDR video](#15-wait-for-the-copy-of-an-hdr-video) | Done | Check in Firefox on the NAS |
| 16 | [Ten interface languages](#16-ten-interface-languages) | Done | Check date fields and fonts in Chinese, Japanese and Korean |
| 17 | [RAW photos from cameras](#17-raw-photos-from-cameras) | Done, released in 0.1.0 | NAS checks 2, 3, 4, 6 and 7 |
| 18 | [Lightroom XMP sidecars](#18-lightroom-xmp-sidecars) | Draft plan | Open questions 2 to 4; builds on 19 |
| 19 | [Library folders indexed in place](#19-library-folders-indexed-in-place) | Planned | Everything |
| 20 | [Safer RAW decoding](#20-safer-raw-decoding) | Done, released in 0.1.1 | |
| 21 | [Map](#21-map) | Done, released in 0.2.0 | Check the map on a phone and with a larger library |
| 22 | [Demo activity log](#22-demo-activity-log) | Implemented | Deploy and enable on the public demo |

## 1. Administration page

**Status:** done.

A page for administrators at `/admin`, with the image processing settings. They are stored in the database and read for each photo, so a change applies to the next photo processed.

| Setting | Default | Range |
|---|---:|---|
| Preview format | WebP | WebP, JPEG |
| WebP quality | 75 | 1–100 |
| WebP compression effort | 2 | 0–6 |
| JPEG quality | 80 | 1–100 |
| Grid preview size, shortest side | 400 px | 200–800 |
| Viewer preview size | 2048 px | 1024–4096 |

Purely technical settings stay in the environment, such as the number of workers. Stripping metadata, auto-rotation and sRGB conversion are not configurable. Users and folder imports remain separate pages.

Settings apply to new photos only. The `render-grid` command makes the grid previews of all photos again, as described in the [deployment guide](deployment.md#making-grid-previews-again). Regenerating all previews from the administration page would be a separate improvement, since it is a background task that takes hours and needs progress and protection from running twice.

## 2. Timeline for large libraries

**Status:** done.

The timeline loads month by month from counts the server gives first, with month titles and a scrubber with the years. A month near the screen loads all of its pages; months far from it are only counted. The trash keeps loading page by page, with a spinner at the end of the list and a retry button when a page fails. Decisions are in the [architecture](architecture.md#decisions).

Loaded months stay in memory. Unloading the months far from the screen is left for when a library makes it necessary.

## 3. Camera and lens filters

**Status:** done.

The timeline filters by camera and by lens, next to the tags and dates. On a phone they sit behind one button with the dates. The counts in each list take the other filters into account. Decisions are in the [architecture](architecture.md#decisions).

The lists offer the photos the timeline shows, including other users' photos when it shows them. Other pages, such as albums and "Shared with me", have no such filters yet.

## 4. Shared photos in the timeline

**Status:** done.

A user who had uploaded nothing saw an empty timeline, even with photos and albums shared with them. The timeline now also shows the other users' photos that the user can see, with a "Mine / All" switch that is saved for the user and on by default. Decisions are in the [architecture](architecture.md#decisions).

## 5. PatternFly look

**Status:** done.

The interface follows [PatternFly 6](https://www.patternfly.org/) on top of Vuetify, with fuchsia in place of the PatternFly blue. Decisions are in the [architecture](architecture.md#decisions).

Done:

- Colors from the PatternFly 6 tokens in light, dark and high contrast themes, and a glass mode with translucent surfaces.
- Red Hat Text and Red Hat Display fonts.
- Pill buttons, 6px form controls, 16px cards and menus, 24px dialogs, joined toggle groups.
- The app bar and navigation on the page background, the content in a rounded panel, the current navigation item on a light plate.
- A theme menu in the app bar with the color scheme and the contrast mode, kept in the browser.
- Form fields with the label above, a mark on required fields and the helper text below, through the `AppFormGroup` wrapper. Filters in toolbars show a placeholder inside instead of a label, and the timeline button that resets them reads "Clear all filters".
- Dialogs on the `AppDialog` wrapper, with the filled primary button on the left, a link-style Cancel after it and a red button for destructive actions.
- The PatternFly type scale: 14px body text, fields and lists, and headings sized by level (h1 1.5rem, h2 1.25rem, h3 1.125rem).
- PatternFly font weights and spacing: regular button text, 600 for headings, form labels and badges, no letter spacing, 12px tooltips.
- Cyrillic in Inter. The Red Hat fonts have none, and PatternFly leaves it to Helvetica or Arial, so dates in Russian looked out of place. Only the Cyrillic file of Inter is bundled, scaled to 92% to sit with Red Hat Text, and the browser loads it only for pages with Cyrillic text. Headings take it at most at weight 500, which matches Red Hat Display at 600.
- PatternFly alerts: a border, an icon and a title in the status color. Notices about finished actions are toast alerts stacked in the top right corner, under the app bar so that they leave the Upload button visible; errors stay until closed.
- Applied filters on the timeline as chip groups under the toolbar, one per category with its own clear button, and "Clear all filters". The tag, camera and lens fields show only a count of chosen values.
- Page headers on the `AppPageHeader` wrapper: breadcrumbs, the title, a short description where it helps and the page actions on the right, with the PatternFly section padding of 16px and 24px. Every page has one except the timeline, which keeps its height for photos. An album shows "Albums / Trip" or "Shared with me / Trip" instead of a back arrow. Pages without a photo grid scroll as a whole, with their content aligned to the header.
- PatternFly tables and tabs. Tables are compact, with a line under every row including the last, column titles at 12px in bold, and the outer columns lined up with the page title. Tabs have a line under the whole row, the current tab underlined in fuchsia with rounded ends, the others in the subtle text color, and 8px of padding at the sides.
- PatternFly empty states on the `EmptyState` component: a 56px icon in a subtle color, red for a failure, the title, a line of text and the action that leads out. The empty timeline offers "Upload photos", no matching photos "Clear all filters", no albums "Create an album", an empty album and no tags "Open the timeline", and a failed load "Retry".
- Date filters on Vuetify's `VDateInput` instead of the browser's own date field: an outlined field with "From" or "To" and the format hint inside, a calendar button, and a calendar popover in the application's theme. Dates are typed and shown the way the browser's language writes them, in the fields, the calendar and the filter chips, while the address keeps YYYY-MM-DD.
- Focus rings: a 2px outline in fuchsia on keyboard focus, in place of Vuetify's tinted overlay. It sits 2px outside buttons, links, chips and cards, inside list items, tabs, joined buttons and the logo link where the container would cut it off, and around the track of a switch and the box of a checkbox. A focused field draws its outline in fuchsia too.
- Font Awesome Free icons in place of Material Design Icons, as in PatternFly. Each SVG is bundled on its own, so only the icons in use reach the browser, and no icon font is loaded. The glyph takes two thirds of Vuetify's icon box, which gives PatternFly's 16px icons in lists and 14px in buttons; empty states keep 56px. Vuetify's own icons, such as the dropdown caret, the checkbox and the calendar, come from the same set.

A second visual style like PatternFly's Project Felt is out of scope for now.

## 6. Finding the same object or place: experiment

**Status:** planned, not started.

The user selects an object in a photo, such as a house, and gets the other photos of the same object or place. The SigLIP 2 vector describes the whole frame by its meaning, so it finds similar houses rather than the same one, and misses photos where the object takes only a corner of the frame. The experiment checks how far the current vectors go before anything is added to the API or the interface.

Plan:

1. A script in `ml/`, with no changes to the API or the interface. Test data: two or three photos of the same house or place from different angles, added to the development library if `samples` has none.
2. Candidates: crop the selected region from the viewer preview, embed it with the existing image embedding and take the nearest photos. When the photo has a location, also try a filter by distance from it: for the same place it is often the strongest signal and needs no ML.
3. If the results are similar but not the same object: re-rank the first 100–200 candidates by local features with geometric verification (LightGlue with ALIKED or DISK features, RANSAC). Not SuperPoint, whose weights have a non-commercial license. In the service this would be a new stateless endpoint, such as `POST /match`, taking the crop and the candidates' previews and returning the number of matched points for each.
4. If objects in a corner of the frame are still missed: try several vectors per photo for parts of the frame, such as a 2×2 grid and the center, or DINOv2 (Apache 2.0) as a second vector type, which tells specific objects apart better than SigLIP.

Record the results here: what each step found on the test photos and how long it took. Unlike similar photos, a search by a selected region needs the ML service to be running. Any new model or dependency is agreed on before it is added.

## 7. Interface language

**Status:** done.

The interface in English and Russian, with the language chosen in the profile. Decisions are in the [architecture](architecture.md#decisions), and how to add a string is in the [development guide](development.md#interface-text).

Done:

- vue-i18n, with the messages in `frontend/src/i18n/en.ts` and `ru.ts`. Every string of the interface moved there, including page titles, empty states, toast alerts, form hints, accessible names and the errors the interface shows. Counts use plural forms, three in Russian.
- A "Language" field in the profile: Automatic, English, Русский, each language named in itself. Automatic, the default, takes the first of the browser's languages that the interface has, or English. The choice is stored with the user (`language` on `/me`), so it applies on every device, right after login. Before login the browser's language applies.
- Vuetify's own texts follow the chosen language through its vue-i18n adapter. Dates and numbers do too: the date filters, month titles, the viewer, tables and the shooting details. English keeps the browser's English region, such as en-GB, and is en-US otherwise.
- API errors stay in English. The interface matches the messages it knows, such as a wrong password or a taken login, and shows its own text; others are shown as the server wrote them. Processing and import errors on the administration and import pages stay as the server wrote them.
- `make e2e` switches the profile to Russian, checks a translated page after a reload, and switches back.

## 8. Place names in the viewer

**Status:** planned, not started.

The viewer shows the name of the place where a photo was taken in place of its coordinates. Decisions are in the [architecture](architecture.md#decisions).

Plan:

1. A table of populated places from the GeoNames `cities1000` dump, with their names in English and Russian from the alternate names, loaded by a command or a migration. GeoNames asks for attribution under CC BY 4.0, so the documentation and the interface credit it.
2. Processing finds the nearest place to the photo's location and keeps its identifier with the photo. A periodic job does the same for photos processed before, as the embedding job does.
3. The API returns the place with the location, in the language of the request, under the same administrator setting. The interface shows "Town, region, country" with the map link on it, and hides it with the location.
4. Backend tests: the nearest place, the language fallback, and no place name when locations are hidden.

The user's interface language is stored with the user (improvement 7), so the API can pick the language of a name from it, and from the browser's `Accept-Language` when it is automatic.

## 9. OPPO motion photos and older Huawei moving pictures

**Status:** planned, not started.

Motion photos play for Google's formats (Motion Photo and the older MicroVideo, Xiaomi included), Samsung's, and Huawei and Honor moving pictures. Two gaps are left:

1. **OPPO, OnePlus and realme.** Descriptions of the format say it is a Google Motion Photo with OPPO's own XMP fields (`OLivePhotoVersion`, `MotionPhotoOwner`), so `MotionPhotoVideo` should already find the video. No real file has confirmed it. Get an original from a phone with Live Photo on, sent as a file rather than as a photo in a messenger, which drops the video, and check it with `exiftool -b -MotionPhotoVideo` and `ffprobe`. If no phone is at hand, a file from an OPPO converter checks the parsing but not what a phone writes. Scanning Wikimedia Commons originals in bulk does not work: upload.wikimedia.org answers 429 even at one request a second.
2. **Huawei and Honor photos processed before 2026-11-12.** Their stored tags do not tell a moving picture apart, so `backfill_motion` does not find them. It could read the end of the file of every Huawei and Honor photo by `Make`, once rather than daily, for example with a mark that the check was done.

## 10. Import from Google Takeout and iCloud

**Status:** Google Takeout done: steps 0 to 6 merged into `master` by 2026-11-17, step 7 (tests) done on 2026-11-18. iCloud: plan drafted on 2026-11-22, waiting for a real export.

Steps 0 to 6 were checked on the development stack with real exports, except 6d, which backend tests cover. Step 7 added a backend test of a whole export, imported a whole real export on the NAS and measured the timeline on a large synthetic library.

People leaving Google Photos or iCloud take their library with them as an export. The files are there, but part of what the service knew about them is not in the files: dates and locations of photos without EXIF, places the owner corrected, descriptions, favorites, the archive and albums. An import reads these next to the files.

Decided on 2026-11-15:

- Both services are imported from their exports only. Google removed reading a whole library from the Photos Library API on 2025-03-31, so a token gives nothing to import. iCloud Photos has no public API, and the private web API of icloud.com, which icloudpd uses, needs the Apple ID session to be renewed with a 2FA code every two months, does not work with Advanced Data Protection and breaks when Apple changes the login. icloudpd is not used, neither inside the application nor next to it.
- An import is a kind of folder scan, run by an administrator for an owner in the same scan root. It reads an unpacked export, `.zip` parts and `.tgz` parts, so the parts do not need unpacking by hand.
- Metadata from the export fills what the file lacks or got wrong. An EXIF date with an offset stays. A date without an offset, or no date, gives way to `photoTakenTime` from the JSON: the application reads a date without an offset as UTC and the interface shows dates in the browser's time zone, so such photos already appear shifted by it (by 3 hours in Moscow), while the Takeout time is the real moment, with the time zone and the owner's corrections that Google applied. Google Photos lets the owner add a location only to a photo whose file has none; it cannot change a location from the file. So a `geoData` that is not 0,0 fills the location of a file without one, and a file's own location stays. A `geoData` of 0,0 means "unknown" and never clears a location the file has. The shift of dates without an offset outside imports is a problem of its own, not solved here.
- An edited copy (`IMG_1-edited.jpg`, or the word in the language of the account) is linked to its original as a version of it, the way a Live Photo video is linked to its photo. Both use one link, "hidden under", kept in a table of its own with the kind of link, in place of `live_photo_id` (decided on 2026-11-17, see step 5); pairs of RAW and JPEG, bursts and duplicates could use it later. Until the versions step, edited copies are imported as photos of their own, and the import remembers each pair, so that a job links the pairs already imported with no new import. The grid and the viewer show the edited copy, as Google Photos did, with a switch to the original in the viewer. The date, location and favorite come from the original's JSON. The hidden version follows the shown one into the trash, albums and shares.
- Photos that other people added to a shared album are imported for the owner of the import, so that the album is whole.
- Descriptions, favorites and the archive are imported, and each becomes a feature of the application:
  - A description is shown in the viewer's details and its owner can edit it there. Searching by description comes later.
  - A favorite is each user's own mark, on any photo they can see. Others do not see it.
  - The archive is each user's own too: an archived photo leaves that user's timeline and stays in albums, search and an "Archive" page.

Plan for Google Takeout:

0. **A real export first.** The naming rules below come from reports by other projects, and Google has changed them before. Before the matcher is written, a small Takeout is made of one or two albums with an edited photo, a screenshot and an iPhone Live Photo in them, and `testdata` is built from its real names. It answers three questions: whether an edited copy has no JSON of its own and shares the original's, which would find it by "no JSON, same folder and extension, the original's name and a suffix" with no list of words in each language; whether the photo and the video of a Live Photo still share `ContentIdentifier` after Google, as pairing by name is ruled out in the [architecture](architecture.md#decisions); and what the year folders and `metadata.json` of albums look like.

   What a real export showed on 2026-11-15: two one-part exports of an account in Russian, a `.zip` of the whole library (837 files: JPEG and one `.3gp`) and a `.tgz` of one album, whose entries are the same as that album's folder in the `.zip`. The `.zip` sets the UTF-8 flag on its names; macOS `unzip -l` still shows them garbled.
   - **Edited copies: answered, yes.** All 68 files without a JSON are edited copies, `IMG_1-отредактировано.jpg` next to `IMG_1.jpg`; every JSON has its file. The word is the last `-` segment (`20190921-_MG_5983-отредактировано.jpg`), and all 68 pairs have the same EXIF `DateTimeOriginal`, a check that needs no list of words.
   - **Live Photos: answered in a second export, yes.** See below.
   - **Folders.** Year folders are localized too (`Фото 2013 г.`). The album file is `метаданные.json`, not `metadata.json`, while the sidecar suffix `.supplemental-metadata.json` stays English. An album's file has `title`, and sometimes `description`, `access` and `date`. The root of `Google Фото` also holds `названия-воспоминаний-созданные-пользователем.json`, a list of titles, which is neither an album nor a sidecar.
   - **Albums and year folders.** A photo in an album is also in its year folder, byte for byte (356 such files). The two JSONs differ in `url`, `creationTime`, `imageViews` and `sharedAlbumComments`, which only the album's copy has; `photoTakenTime`, `description` and `favorited` agree.
   - **Dates.** A Lenovo photo has EXIF `13:58:10` with no offset and `photoTakenTime` `10:58:10Z`, three hours apart; a Canon photo with `+03:00` matches its `photoTakenTime` exactly. This confirms the date rule.
   - **Not seen.** `geoDataExif` is in no JSON and every `geoData` is 0,0, so the location rule is unchecked. `archived` and `trashed` do not appear, and `favorited` appears only when true. No name has a `(1)` or a cut suffix, and the longest name is 52 characters, so those rules remain reports from other projects.

   A second export on 2026-11-15: one album of the same account, with an iPhone Live Photo (`IMG_0022.HEIC` and `IMG_0022.mov`), a phone JPEG with GPS and three 4K MP4 videos, uploaded through the web, with a location added to the Live Photo in Google Photos.
   - **Live Photos.** The photo and the video each have a JSON of their own, with the same `photoTakenTime`; only the photo got the location. Both still carry the same `ContentIdentifier`, so pairing works as for any upload.
   - **Files are not always the originals.** The MOV and the MP4 files are byte for byte the uploaded ones. The HEIC came back as a JPEG under the name `IMG_0022.HEIC`, at the same size, with EXIF and the Apple maker note kept, and the JPEG came back recompressed (1.5 MB instead of 3.9 MB, same 4000x3000 and EXIF). The account uploads in "Storage saver" quality, which explains it; an account on "Original quality" should get its photos back unchanged. Ingest detects types by content, so the name does not matter, but such a photo does not match a copy of the original already in the library by hash.
   - **Locations.** A location added in Google Photos to a photo without GPS is in `geoData`, with a non-zero `latitudeSpan` and no `geoDataExif`. A photo with GPS has `geoDataExif` equal to `geoData`. A video whose file has GPS (QuickTime `GPSCoordinates`) got `geoData` 0,0 and no `geoDataExif`, so Google did not read it, and 0,0 must not clear the file's location.
   - **Dates.** A JPEG with EXIF `08:30:14` and no offset has `photoTakenTime` `04:30:14Z`, four hours apart (Tbilisi): the same rule again.
   - **Still not seen.** A screenshot, `(1)` and cut names. The album's file has `access: protected`.

   Two more exports on 2026-11-15, after one photo of an album was archived and another moved to the trash:
   - **Archive.** The archived photo stays in its album's folder, its JSON there has `"archived": true`, and it is also in a folder of its own, `Отправить в архив` (the account's word for "Archive"), with the same flag. The folder has no album file, so the album rule leaves it out.
   - **Trash.** The trashed photo left its album's folder and is only in `Корзина` ("Trash"), with `"trashed": true` and no `googlePhotosOrigin`.
   - Both flags, like `favorited`, appear only when true. The import reads the flags and not the names of these folders, which depend on the language.
1. **Archives and resuming.** A Takeout import is a kind of scan the administrator chooses in the scan form (decided on 2026-11-15); a plain folder scan opens no archives and skips `*.supplemental-metadata.json` without a report, as it does `.aae`. The job lists the parts (`.zip`, `.tgz`, `.tar.gz`) and the loose files of an unpacked export, waits for them to stop changing as a folder scan does, and reads them in order. A folder may hold several exports, as a Downloads folder does: parts are grouped by the time in their name (`takeout-20261115T073434Z-1-001.zip`), an unpacked `Takeout` folder is an export of its own, and an entry is known by its export and its path in it, as the same path can hold different JSON in two exports. Every JSON file is kept in the import's tables with its path, and every photo with the path it came from and its photo, duplicates included, so that albums in step 4 find each copy.
   A `.zip` entry is opened twice, once to hash it and once to store it, so a file the library has is not written again; a `.tar` entry has no random access and is written to a temporary file while it is hashed. A `.zip` part continues from the next entry after a pause or a restart. A `.tgz` part is read in long slices, as finding an entry means unpacking everything before it: a slice that ends in the middle reads the part again from its start, skipping the entries done.
2. **Matching.** Once every part is read, the run goes to `matching` and matches each JSON to its file, within one export and one folder. A JSON of a photo is told from that of an album by `photoTakenTime`. The name of the JSON varies: `IMG_1.jpg.supplemental-metadata.json`, the suffix cut short (`.supplemental.json`, `.supplementa.json` and so on) when the name is long, file names themselves cut at 47 characters, and a `(1)` for repeated names in more than one place in the name. So the match tries, in order: the whole name; the `title` inside the JSON with the `(n)` of both names; a `title` the file's name was cut from; the file's name followed by a cut suffix or none, for JSON without a title. A candidate that is not the only one is not taken. Edited copies are matched to their originals in the same folder, among the files still without a JSON after all other matching: the last `-` part of the name is dropped, before or after its `(n)`, and the original must be in the same folder with the same extension. No list of words is needed. The check of the same EXIF date moves to step 3, the first to use the pair: an edited copy takes the original's metadata only when processing has read the same date from both, and the matching, which runs before processing, records the pair as a candidate. Each file keeps the path of its JSON or its original in `takeout_entries`, and the run counts edited copies, files without JSON and JSON without files.
3. **Metadata.** `imported_metadata` keeps the whole matched JSON of each photo, with `photoTakenTime` and `geoData` read out of it. When a photo is in several exports, as when the owner exports again after a change, the newest export's JSON wins, by the time in the names of its parts; an unpacked folder counts as older than any export (decided on 2026-11-15). A photo's own JSON comes before its original's. One function in `jobs` works out the date and the location by the rules above, from the stored EXIF and this table: processing calls it after each photo and the matching after it has saved the metadata, so it does not matter which comes first. The date goes into `taken_at` and `sort_at`; a location goes into `imported_latitude` and `imported_longitude` of `photos`, which the viewer shows when the EXIF has none. An edited copy takes its original's metadata only when processing has read the same EXIF date from both. A photo already in the library, matched by its hash, gets the metadata too. Keeping the JSON lets the later steps fill descriptions, favorites, the archive and versions by a job, as `backfill_motion` does, after the export is deleted.
4. **Albums.** An album of one owner is known by its title, so an album that is in several exports, or imported again, is one album (decided on 2026-11-15). A folder that holds a JSON other than a sidecar, with a `title`, becomes an album with that title. The file's name depends on the language of the account (`metadata.json`, `метаданные.json`), so it is not looked up by name. Year folders such as "Photos from 2019", whose name depends on the language too, have no such file and are not albums. Other JSON files outside albums, such as the list of memory titles in the root, are skipped and not counted as JSON without files. A file in both places is stored once and added to the album; comments in `sharedAlbumComments` are on the album's copy only.

   How it is built (reviewed and decided on 2026-11-15):
   - The matching makes the albums, in the transaction that records the matches, so a run that stops and resumes makes them again with the same result. An album file is a JSON entry with no `photoTakenTime` and a `title` that is a non-empty string, in a folder right under the product folder (`Takeout/Google Фото/<album>/метаданные.json`). In the real exports the list of memory titles lies one level higher and its `title` is a list, so either check leaves it out. An unpacked export keeps `Takeout/` at the start of its paths, so the depth is the same there. A folder with more than one such file is not an album, as a candidate that is not the only one is not taken elsewhere.
   - The title is cleaned as a name typed in the interface is: spaces trimmed, control characters replaced by spaces, at most 200 characters. Titles are gathered from every export of the run first, so two exports with one title fill one album. The album is the owner's album whose name is the title, the oldest one if there are several, or a new one, made one title at a time in Go. The JSON's `description`, `access` and `date` are not used: albums have no description, and Google's sharing says nothing about who may see the album here, so it stays private to its owner.
   - Every media file of the album's folder whose outcome is `added`, `duplicate` or `in_trash`, with a photo of the owner that is not purged, is added, with nothing added twice. The scan form has a choice for photos in the owner's trash (decided on 2026-11-15): leave them out of the album, which is the default, add them, or add them and restore them; only photos of albums are restored, and the run counts them. An album with nothing to add is made empty, as in Google Photos. Both an edited copy and its original are added until the versions step hides the original. A Live Photo video is added like any file; the album hides it as it hides every paired video.
   - The run counts the albums it filled, created or found, and the scan's summary shows the number.
   - Album titles are compared exactly, so "Trip" and "trip" are two albums. A photo the owner removed from an album comes back when the export is imported again, as the album is found by its title.
   - Checked on 2026-11-15 on the development stack with six real exports of one account, after one photo of an album was put in the owner's trash, with "add and restore": 4 albums, with 194, 160, 2 and 6 photos, the numbers of distinct files in their folders across all exports; the photo came back from the trash; the archived photo is in its album; year folders, the archive and the trash made no albums. The photo the owner moved to the trash in Google is in its album too, as older exports still had it there: albums are the union of every export. Step 6d moves such a photo to the trash, where the album hides it, when the same run added it to the library; these photos were added by earlier runs, so they stay.
   - Tests: the matching of a small export with an album, the year folder and the list of memory titles; an album already in the library with the same title; a second run that adds nothing; a photo in the trash with each of the three choices.
5. **Versions.** A link from the original to the edited copy, the lists and counts that leave the hidden version out, the trash, albums and shares that move both, and the switch in the viewer.

   How it is built (decided on 2026-11-17):
   - **A table of links.** `photo_links (hidden_id, shown_id, kind)`: `hidden_id` is the primary key, so a file is hidden under one photo only; both ends reference `photos` with `ON DELETE CASCADE`; `shown_id` has an index, as the viewer asks for the links of a photo. `kind` is `text` with a `CHECK` (`live`, `edited`), an enumeration in Go and TypeScript; a PostgreSQL `enum` is not used, as a value added to it cannot be used in the transaction that adds it. A photo has at most one `live` link under it, by a partial unique index. A separate table was chosen over a column on `photos` because later links, such as duplicates, need not hide anything, and a photo may have several versions of different kinds: a RAW, an edited copy and a Live Photo video. The migration moves the pairs of `live_photo_id` into the table and drops the column.
   - **Chains.** Links may chain: the video of a Live Photo stays under its photo when that photo, an original, is hidden under its edited copy. The trash, restoring and purging follow the links down, with a recursive query; a link never points up to a photo above it.
   - **Lists.** Every list, count, album cover, search and embedding leaves out a hidden photo with `NOT EXISTS (SELECT 1 FROM photo_links l WHERE l.hidden_id = p.id)` in place of `live_photo_id IS NULL`. It is a lookup by primary key for each row; whether the timeline stays as fast is to be checked with `EXPLAIN ANALYZE` on a large library.
   - **Access.** Only the owner, and an administrator, open a hidden photo itself: an edit is often made to crop or hide something, so the original of an edited copy is not shown to those the copy is shared with (decided on 2026-11-17). Others see a version only through the shown photo, as they play the video of a Live Photo through its photo. The check stays in `GetVisiblePhoto`; `photo_visible_to` is not changed, as search calls it for every row. The trash and restoring move the whole chain, and a purge removes it.
   - **Edited copies.** `jobs.LinkVersions` links the edited copies an import recorded in `imported_metadata.original_id`: when both photos are processed, of one owner, out of the trash, and have the same EXIF date, as for their metadata in step 3; the edited copy is not hidden under anything and the original has no edited copy yet. It runs after the matching of a Takeout scan, after the processing of each photo, after restoring photos, and in the `pair_live_photos` job at every start and once a day, rather than as a job of its own. When a link is made, the shown photo takes over what the original had, so that nobody loses the photo (decided on 2026-11-17): its albums, the album covers it was, its direct shares, `visibility = 'all'` and its tags. Both move `updated_at`, so that open pages drop the original.
   - **Live Photos and edited copies.** The video of a Live Photo belongs to the original, and the edited copy shown above it plays that video as its motion, as Apple Photos does (decided on 2026-11-17). An edited copy may keep the `ContentIdentifier` of its original, and pairing may run before the matching records the copy, so the rule does not depend on the order: pairing does not choose a recorded edited copy as the photo of a video, and linking moves a video already paired with the copy to the original when the original has none. The motion of the shown copy, its `has_motion` and its H.264 copy come from the original's video, or from the original's own motion if it is a motion photo.
   - **Uploads.** A file the owner has that is hidden, uploaded again, is answered with the photo shown at the top of its chain, as the video of a Live Photo is now.
   - **API.** `GET /photos/{id}`, which the viewer loads, carries `versions`, the links right under the photo with their kind, only for its owner; list items stay as they are. Events carry `hiddenUnder` with the kind, so that pages drop a photo that became hidden; `livePhotoId` goes. The previews, original and details of a hidden photo come from its own address.
   - **Naming.** The query `DeletePhotoLinks`, which removes a purged photo from albums, tags and shares, becomes `DeletePhotoMemberships`, so that it is not confused with the table.
   - **Viewer.** Each kind the interface knows has a control of its own: `live` keeps the motion button, `edited` gets "Show original", for the owner only, which switches the image, the details and the download to the original and back. Kinds the interface does not know are not shown.
   - Not in this step: unlinking a version by hand, and choosing which version is shown.
6. **Descriptions, favorites, archive.** Schema, API and interface for each, filled from `description`, `favorited` and `archived` in the JSON. Items with `trashed` go to the trash, with the date of the import as the date of deletion.

   Plan (decided on 2026-11-17), four changes in this order, each released on its own:
   - **6a. Descriptions.** `photos.description`, text, empty by default, at most 2000 characters, with spaces trimmed at both ends, `\r\n` turned into `\n`, line breaks kept and other control characters replaced by spaces. Everyone who can see the photo sees its description, as in Google Photos, in the viewer's details; only the owner edits it, with a pencil button that opens an `AppDialog` with a field of several lines, through `PATCH /photos/{id}`, which moves `updated_at` so that open pages get it. An import fills the description of a photo that has none from a non-empty `description` string in its newest JSON; it never replaces a description, so the owner's edits stay. When an original is hidden under its edited copy, the copy takes the original's description if it has none. The migration fills the descriptions of photos imported before it from `imported_metadata`, so no export is needed again. Searching by description stays for later.
   - **6b. Favorites.** `photo_favorites (user_id, photo_id, created_at)`, one row per user and photo, removed with either and when the photo is purged (`DeletePhotoMemberships`). A user marks any photo they can see, and only that user sees the mark. The viewer has a star button; the selection bar has "Add to favorites" and "Remove from favorites"; a "Favorites" page in the menu lists the user's favorites, their own photos and others' they can see, newest first, through a `favorite` filter of the timeline queries. `GET /photos/{id}` tells whether the photo is the viewer's favorite, and the viewer loads it for every user, not only the owner; list items do not carry it, so the lists stay as they are. A mark is the user's own, so it moves no `updated_at` and sends no event: the page where the user acted updates itself, as trashing removes a photo, and other open tabs catch up on reload. An import marks the photos whose newest JSON has `"favorited": true` as favorites of the import's owner, on the photo shown at the top of their chain. Linking a version moves every user's favorite from the original to the copy. The migration marks those imported before it, on the photo at the top of their chain.
   - **6c. Archive.** `photo_archive (user_id, photo_id, created_at)`, each user's own, on any photo the user can see, removed with the photo as favorites are. An archived photo leaves that user's timeline, its month counts and facets, and stays in albums, search, "Shared with me" and similar photos. An "Archive" page in the menu lists it through an `archived` filter of the timeline queries. The condition does not go into the WHERE block the timeline queries share as it is: `ListTimeline`, `CountTimelineMonths`, `ListCameras` and `ListLenses` leave out the user's archived photos only when there is no `album`, `favorite` or `archived` filter, so album pages and the Favorites page keep archived photos, as Google does, while the timeline filtered by a tag leaves them out like the plain timeline. Archiving, like a favorite, sends no event. The viewer and the selection bar archive and unarchive. An import archives the photos whose newest JSON has `"archived": true` for the import's owner; linking moves archive rows to the copy, as for favorites; the migration archives those imported before it, on the photo at the top of their chain.
   - **6d. Trash.** At the end of the matching, after `makeAlbums`, the photos that the run added to the library, with an entry whose outcome is `added`, and whose newest JSON has `"trashed": true`, go to the trash with the time the run started as their date of deletion, with their chain, as `TrashPhotos` moves it. A hidden file acts on the photo shown at the top of its chain, and a photo already in the trash keeps its date. So a photo that "add and restore" brought back for an album but that Google trashed ends in the trash, where its album hides it. A photo the library already had is never moved to the trash by an import, so a photo the owner restored stays restored when the export is imported again. The migration does nothing for earlier imports.
   - Importing an export again marks favorites and the archive again, as it adds photos to albums again: a mark the owner took off comes back.
   - The real exports have 5 descriptions (the longest 16 characters, none on several lines), 3 favorites, 1 archived and 1 trashed photo among 486 imported photos: the numbers to check each change against on the development stack.
   - The browser test gets a short step for the Favorites and the Archive pages in 6b and 6c, as they extend the main path.
7. **Tests.** A small Takeout export in `testdata`, made from the real names of step 0, with cut names, repeated names, an edited copy, an album, a photo in two parts and a restart in the middle of a `.tgz`, read as a folder, a `.zip` and a `.tgz`. Then a whole real export, recording here what it showed.
   - [x] `TestTakeoutWholeExport` in `internal/scan/export_test.go`, added on 2026-11-18. The export is built in memory, as in the other Takeout tests, rather than kept as files in `testdata` (decided on 2026-11-18): two parts of one export, with the photos in the first and most of their JSON in the second, a suffix and a file name cut short, two files of one name told apart by `(1)`, an edited copy, and an album whose file and copy of a photo are in the second part. It is read as two `.zip` parts, as two `.tgz` parts with a new service after the first batch, as after a restart, and unpacked into one folder, and each form must give the same matches, metadata and album. Giving the second part the time of another export makes the test fail, as its JSON are then matched apart from their photos.
   - [x] A whole real export imported on the NAS for a new owner, on 2026-11-18: the same six exports, the same part downloaded twice among them, with the defaults. The first run read nothing, as the copied parts were not readable by `apps`, and said only "Finished" with 6 problems; the import page now shows such a run as failed, and one with some failed files as finished with errors, and the details of a run with "permission denied" explain the permissions. The second run took under 2 minutes for 2,333 entries: 486 photos added, 711 duplicates, 1 page that is not a photo, 1,135 JSON files with no photo without JSON and no JSON without photo, 70 entries of edited copies, counted once per entry, of 65 distinct ones, 4 albums and 1 photo moved to the trash, with no problems. Once processed, the timeline showed 418 photos: 486 less the photo in the trash, the archived one, the video of the Live Photo and the 65 originals hidden under their edited copies, every one linked. Descriptions (3), favorites (2), the archive (1) and the trash (1) match the distinct photos of the exports; the 5 descriptions and 3 favorites noted in step 6 counted a photo once per JSON. The albums have 5, 194, 157 and 1 photos, against 6, 194, 160 and 2 on the development stack before versions: the Live Photo video, 3 originals under their edited copies and the photo Google trashed are hidden
   - While the NAS processed the import, the administrator's "All photos" with the new owner chosen showed the processed photos and empty places for the rest, with no progress, until the page was loaded again: the photos still being processed were left out of the timeline but counted in "All photos". They are left out there too now, and from the photo counts of albums. As the events carry the user's own photos only, the page reads its month counts again every 10 seconds and loads the months that changed, so another owner's photos appear as they are processed
   - [x] The timeline speed on a large library, checked on 2026-11-18 with `EXPLAIN (ANALYZE, BUFFERS)` of the generated `ListTimeline` and `CountTimelineMonths` in a throwaway test on a Mac with an M5 Pro, in the test container: 120,000 synthetic photos over ten years, 100,000 of one owner and 20,000 of another shared with all, 20,000 hidden under another photo, 2,000 archived and 1,000 being processed. The first page of 200 takes 0.4 ms, and 0.5 ms with shared photos: the index on the timeline order is read until the page is full, with the `photo_links` check a lookup by primary key for each of about 250 rows. The month counts take 160 ms, and 125 ms with shared photos, reading every photo of the owner; the anti join on `photo_links` adds about 1 ms to them. The NAS has no library this large, so its own time is not measured

Steps 0 to 4 make a working import, released on their own. Versions come next, and descriptions, favorites and the archive each as a change of its own.

### iCloud

**Status:** plan reviewed and drafted on 2026-11-22; the hidden and deleted choices are decided. Waiting for a real export (step 0): the owner of the library requests the copy at privacy.apple.com, which Apple prepares in a few days.

The source is the copy of the data that Apple prepares at privacy.apple.com (decided on 2026-11-15, see above). The WebDAV upload from an iPhone already brings new photos in (checked on 2026-11-22), so the import is for the history: what a library of files does not say, such as favorites, hidden and deleted photos, albums and the dates Apple keeps. If only the files are needed, a plain folder scan of the unpacked parts already imports them.

What the format is said to hold, from reports of other projects and not yet seen by us: parts of the copy are `.zip` files holding the photos and videos, `Photo Details.csv` (`imgName`, `fileChecksum`, `favorite`, `hidden`, `deleted`, `originalCreationDate`, `importDate`) and `Albums/*.csv`. The names of files and parts, the format and time zone of `originalCreationDate`, how edited copies and Live Photos are named, what `fileChecksum` is, and whether captions and locations survive are all unknown until step 0.

Plan, to be corrected by step 0:

0. **A real export first**, as for Takeout. Whether privacy.apple.com can export a part of a library at all is not known: it may offer only the whole iCloud Photos category, with the size of the parts as the one choice, and then a small library of a test Apple ID with a Live Photo, an edited photo, a hidden one, a deleted one and a favorite is the sample. Whether shared albums are a category of their own is checked too. Written down here: the names of the parts, and whether they carry a time like Takeout's, since grouping parts into exports and "the newest export wins" both depend on it; where the CSV files are, and whether every part has its own; the date format and zone; whether an edited copy has a row of its own; whether the Live Photo's photo and video share a name and `ContentIdentifier`; what `fileChecksum` is (compared with the SHA-256 of the file); whether `hidden` and `deleted` photos are in the parts; whether `originalCreationDate` is a true instant or a local clock time labelled GMT, checked against files whose EXIF date has an offset; whether the files are the originals; and what the Albums files hold.
1. **A second kind of scan.** The scan form already chooses a Takeout import; it gets "iCloud" next to it. `import_runs.kind` has a `CHECK` of `folder` and `takeout` (migration 00023), so a migration widens it and `scan.go` gets a `KindICloud`. Reading parts, resuming, hashing, duplicates and `takeout_entries` stay as they are; what changes is how parts are grouped into exports and which entries are metadata.
2. **Matching.** Each row of `Photo Details.csv` is matched to its file by name within one export, and by `fileChecksum` where a name repeats. The result goes into the tables Takeout fills (`imported_metadata`: date, favorite, archived, trashed), so dates, favorites, the archive, the trash and linking edited copies work from one place for both services, and the "newest export wins" rule stays if the parts' names say when they were made; if they carry only a part number, a folder holds one export, and a second one is told apart some other way.
3. **Rules to decide on the real data:**
   - `favorite` marks a favorite of the import's owner, as Takeout does.
   - `hidden` goes to the archive (decided on 2026-11-22), the nearest thing to Apple's Hidden album, though not as private: an archived photo here still shows in albums, favorites, search and shares, so a hidden photo that must stay unseen by others is not shared by hand. The other choices were the trash, leaving hidden photos out, and importing them as ordinary photos.
   - `deleted` goes to the trash (decided on 2026-11-22), with the date of the run as the date of deletion, only for photos the run added, as in 6d.
   - `originalCreationDate` fills a date the file lacks or has without an offset by the Takeout rule, only if step 0 shows that it is a true instant.
   - Photos other people added to a shared album: Takeout imports them for the owner of the import; iCloud needs its own decision once step 0 shows whether they are in the copy.
   - Albums from `Albums/*.csv` by their name, as Takeout albums are made by their title.
4. **Edited copies and Live Photos.** If an edited copy is a file of its own, it is linked to its original as `edited`, with the same EXIF date check; Live Photos are paired by `ContentIdentifier` as now.
5. **Tests.** A small export built in memory from the real names, read as `.zip` parts and unpacked, as `TestTakeoutWholeExport` does; a whole real export on the NAS.

## 11. Grid tile size

**Status:** done.

Before this improvement tiles were 200 CSS pixels in both grid modes, a constant in `lib/timeline.ts`. Rows that keep each photo's shape closed as soon as they overflowed the width, so a row was never taller than 200 and usually a little shorter, on a phone too. Squares aimed at 200, or 130 on a grid narrower than 600. The grid preview size on the administration page sets only the resolution of the preview files, which decides how sharp the tiles are, not their size.

Decided on 2026-11-15:

- One setting drives both modes: a size of small, medium or large, chosen in the profile and stored per user in the database, next to the switch for squares. The default is medium.
- On a grid 600 pixels wide or more the levels aim at 150, 200 and 280 CSS pixels: the height of a row that keeps each photo's shape, and the side of a square. Medium is what a wide grid shows now.
- On a narrower grid the levels give 4, 3 and 2 squares in a row, and rows that keep each photo's shape aim at the side of those squares. This changes the phone grid: medium rows become about 120 pixels tall instead of up to 200, the same as the squares, and a grid between 500 and 600 pixels gets 3 squares instead of 4.
- A row that keeps each photo's shape breaks where its height comes closest to the target, before or after the photo that overflows, so rows are a little taller or shorter than the target instead of never taller.
- The range is limited so that tiles stay sharp, rather than warning about blurry ones: the largest tile is about 400 CSS pixels, which the largest grid preview, 800, covers on a 2× screen.
- `GET /settings/grid` goes: nothing has read it since squares stopped depending on the grid preview size.

Work:

1. Backend. A migration adds `users.grid_size` (`small`, `medium`, `large`, default `medium`, with a check). `PATCH /me` takes `gridSize` and checks it before any write, as it does `language`, so an invalid request changes nothing. The OpenAPI user gets it as an enum in its required fields. The comment in `00012_grid_squares.sql` that says the size comes from the grid preview size is edited. `GET /settings/grid` goes with its handler, route, OpenAPI path and test.
2. `lib/justify.ts`. When a photo overflows the row, compare the height with it and without it and take the one closer to the target by ratio. There is no choice when the photo is the first in its row, and a photo pushed to the next row is checked again at once, since a panorama may overflow alone. A row is at most 1.4 times the target, so a panorama after a single photo does not make a row a thousand pixels tall; the last row of a month keeps the target height.
3. `lib/timeline.ts` and `PhotoGrid.vue`. The target comes from the level and the width instead of `TARGET_HEIGHT` and `squareSide`: in `rowsOf` and its cache key, `rowHeight`, `estimate` and `--row-height`.
4. Profile. A three-button toggle inside `AppFormGroup`, labelled above, under the squares switch, with English and Russian strings. The administration hint of the grid preview size says that it controls sharpness, not tile size: about 200 × the screen density for medium tiles and 280 × the density for large ones; after raising it, `render-grid` remakes the previews of existing photos.
5. Checks. The API tests cover the `gridSize` round trip and the rejection of an unknown value. The browser flow checks that large squares are wider than medium ones. The break choice of `justify` is checked with a throwaway script in the scratchpad, not a committed test. `docs/architecture.md` updates the "Grid tiles" and "Grid preview size" rows and the `users` table.

## 12. Upload into an album, and whole folders

**Status:** done and merged into `master` on 2026-11-18, checked on the development stack with folders dropped and chosen. Still to check: the hidden "Choose folder" on an iPhone and an Android phone.

Uploading from the browser takes single files and puts them in the library only; adding them to an album is a second step. Decided on 2026-11-18: the album is chosen when uploading, and whole folders are uploaded from the browser, by the picker and by dragging. WebDAV already takes folders, and a WebDAV folder can be linked to an album.

Plan, three changes in this order, merged in one commit:

1. **An album for an upload.** `POST /photos` takes an optional `album` query parameter, as the multipart body streams the file and a form field would have to come before it. The album must be the uploader's own, checked before the file is read, with 404 otherwise. After ingest the photo is added with `AddPhotosToAlbum`, duplicates and photos restored from the trash included, as a WebDAV album does. A duplicate of a hidden file already answers with the photo shown at the top of its chain, so that photo goes into the album. A Live Photo video uploaded before its photo is paired later, in processing, so it enters the album and the album hides it once paired, as for WebDAV. OpenAPI and backend tests: an upload into an own album, a duplicate added to it, someone else's album refused without a photo created.
2. **The upload dialog.** Upload in the app bar, and `pickUploadsKey` from the empty timeline, open an `AppDialog` instead of the file picker. It has an album field inside `AppFormGroup`, empty on every opening, which means the library only. The field lists the user's own albums and also takes the name of a new album, as "Add to album" does (decided on 2026-11-18): the album is created once the files are chosen, before they are sent, so that the picker opens within the user's click. The field is an outlined `v-combobox`, as `AppAutocomplete` takes no new values. The dialog has the buttons "Choose files" and "Choose folder", which click the hidden inputs synchronously so that the browser keeps the user's gesture. The album is taken when the button is clicked and travels with the files; files that come any other way have no album. Choosing closes the dialog and starts the upload. The album page gets "Upload to album" in its ⋮ menu, and its empty state an action of the same kind, opening the same dialog with the album filled in. The upload queue carries the album of each file, so uploads into different albums can run together, and the panel names the album. A 404 for an album, deleted while its files wait, is not retried, and the rest of that album's queue fails with it without being sent, so that "Retry failed" can send them again. `merge` places an uploaded photo in the open list when it is the album the photo went into with no other filter, as it does for the whole library, and the album page reads the album again, for its count and cover, at most once a second while files land in it. Dropping files on the timeline still uploads them into the library with no dialog (decided on 2026-11-18); dropping them on an album page the user owns uploads them into that album. The hidden inputs stay in `App.vue` and get a `data-testid` each, as a second input would make `input[type=file]` in the browser test match two; the test uploads into an album through the dialog.
3. **Folders.** "Choose folder" clicks a second hidden input with `webkitdirectory`, which Chrome, Firefox and desktop Safari support; on iOS and Android, where it does not pick folders, the button is hidden, judged by the user agent, which is to be checked on real phones. A folder dropped on the timeline or an album page is read through `webkitGetAsEntry`, called for every item inside the `drop` handler before anything is awaited, walking subfolders and calling `readEntries` until it returns nothing, as it gives at most 100 entries at a time. When the drop has entries, `dataTransfer.files` is not used, as a browser may list a folder there as an empty file. Files inside a folder, chosen or dropped, are filtered by extension, ignoring case, so `.aae` and Takeout JSON do not show up as failures; the panel says how many files were skipped and lists their paths on request (added on 2026-11-18, as a count alone did not say what was left out). Each line of the panel is a line of its own, so that none is cut off. Hidden files and folders, such as `.DS_Store` and the `._` files macOS writes next to each photo on other volumes, are left out without being counted, as listing them was only noise (2026-11-18). The extensions in `lib/uploadFiles.ts` also make the picker's `accept`, and cover what `ingest` detects. Files chosen or dropped one by one go to the server unfiltered, as it tells a file by its content. Folder names do not make albums, as for WebDAV.

## 13. Public links to albums

**Status:** planned, not started.

The owner of an album makes a link that opens it without an account, to show photos to people who do not use the server. Today sharing reaches only users of the server, as described under [sharing](architecture.md#decisions). People moving from other photo libraries tend to expect such links.

Open questions before a plan:

- Albums only, or single photos too.
- Whether a link may have an expiry date and a password, and whether visitors may download originals, which keep their EXIF and GPS.
- Whether the album's location data is shown to visitors, given that locations are shown only when an administrator allows them.
- How the page is served to visitors without a session, and how its requests are limited, as the NAS would then be reached from outside the home network.

## 14. Memories: on this day

**Status:** done on 2026-11-22, released in 0.2.0. The strip above the timeline and the Memories page work, with a backend test (`memories_test.go`) and a step in the browser test, whose page clock stands on 1 June 2026 so that the video taken on 1 June 2024 is "2 years ago".

Photos taken on the same day in earlier years, shown without the user asking. The date filter cannot do this: it selects one continuous range, while memories take one day from every earlier year. It needs no ML, only `sort_at`. Immich and Nextcloud Memories put a strip of "N years ago" cards above the timeline; Google Photos adds curated trips and people on top, which needs ML and is left out.

Decided on 2026-11-22:

- **Where:** a strip of cards above the timeline, one per earlier year, and a page of its own (`/memories`) that the strip and the menu open. The strip is hidden when there is nothing to show, so an empty library or a day with nothing does not take height from the photos.
- **Whose photos:** the user's own photos and those shared with them, as the timeline with "include shared" does, without the user's archived photos, the trash and hidden versions (the same WHERE block as `ListTimeline`).
- **A day with nothing:** per year, only the exact day if it has photos; otherwise the closest day within 3 days before or after that has some, and its card says "around 22 Nov 2023" instead of "22 Nov 2023". A year with nothing in the window has no card.
- **Which day a photo is on:** its `sort_at` in the viewer's time zone, as the timeline's month counts do, so a card's date always matches the day the photo shows under on the timeline. The database does not record whether a date came with an offset, and a date without one is read as UTC, so such a photo taken late in the evening can land on the next day in a time zone east of UTC. Fixing that is a matter for the dates themselves, not for memories.

Plan:

1. API: `GET /photos/memories?tz=<IANA zone>&date=YYYY-MM-DD` (`date` optional, today in `tz` by default, so tests can fix it). It returns years, newest first, each with its day and its photos, at most 60 photos per year. The current year is left out.
2. SQL: `ListMemories` in `photos.sql`, with the same WHERE block as `ListTimeline` (`with_shared` always on, archived left out), except `status = 'ready'`, as a card needs a preview. A condition on the local date of each row would scan the whole library and could not use `photos_timeline_idx`, so Go computes the windows: for each year from the earliest photo (a cheap `MIN(sort_at)`) to the year before the current one, the days `date−3 … date+3` in the viewer's zone as instants, and passes them as `from[]` and `to[]` arrays joined with `unnest`. Windows across New Year need no special case. Go's `time.Date` moves 29 February of a non-leap year to 1 March, so the day is clamped to 28 February explicitly. Go then picks, per year, the exact day, or else the closest day that has photos.
3. OpenAPI, then `make sqlc` and `make api-types`; a store `memories` (Pinia) next to `photos.ts`.
4. Strip: `PhotoBrowser`'s `toolbar` and `header` slots sit above `.content`, which holds the scroller, so the strip goes outside the scroller as a row of fixed height and the grid's computed coordinates, scroll bar and scrubber are untouched (the grid measures its own container). It shows only when no filter is set and there is something to show. Cards have a cover photo, "N years ago" and the date. The page `MemoriesView.vue` has an `AppPageHeader` and a section per year; a photo opens the viewer with the year's photos as its sequence.
5. Strings in all ten language files; a backend test (exact day, nearest days, year boundary, 29 February, archived and shared photos, other users' private photos, a year with nothing) and a step in the browser test with photos dated on the fixed day.

Built as planned, with one change: the page reuses `PhotoBrowser` and `PhotoGrid` (so the viewer, selection and bulk actions work), through a `memories` query of the photos store that gives one titled section per year (`Section.title`) and has no scrubber. The browser sends its own date (`lib/today.ts`) with the request.

## 15. Wait for the copy of an HDR video

**Status:** done on 2026-11-19. Still to check: the first play of an HDR video in Firefox on the NAS.

On 2026-11-19 an HDR10 video in H.264 played grey in Firefox on its first play: the browser plays the original, the copy is asked for alongside it, and the viewer does not switch to a copy that arrives while the original plays. The copy, tone mapped, played once the page was reloaded. Decided the same day: the viewer shows the poster and waits for the copy of an HDR video, as it does when the browser cannot play the original.

The interface does not know which videos are HDR: processing finds it with ffprobe and throws it away.

Plan:

1. A migration adds `photos.hdr boolean`, NULL until the video is probed. Processing sets it where it already probes, and a job at every start probes the videos with no value yet, as the motion backfill does; ffprobe reads only the headers.
2. The API returns `hdr` with every photo, in the lists too, as the viewer starts from the list item.
3. The viewer: when a video is HDR and has no copy yet, starting it asks for the copy first and shows the poster with "the copy is being made", then plays the copy. When no copy is coming (copies off, the transcoder unreachable) or it ends `failed` or `not_needed`, the original plays at once.
4. Backend tests for the column and the API field; the browser test is not changed.
5. The "Video copies" row in `docs/architecture.md`.

Decided on 2026-11-19: videos only, not the motion of motion photos and Live Photos, which lasts about 3 seconds and comes from another file. On AMD graphics, which make no HDR copies yet, nothing changes: the first play of an HDR video waits through the failed attempts and then plays the original, as AMD is a test machine only.

## 16. Ten interface languages

**Status:** done on 2026-11-20. Still to check: step 5, the date fields, and the fonts in Chinese, Japanese and Korean in a browser.

The interface in the ten languages most used in software: English, Russian, Simplified Chinese, Spanish, Brazilian Portuguese, German, French, Japanese, Korean and Italian (chosen on 2026-11-20 over the ten with the most speakers, which would bring Hindi, Bengali and Urdu, with no Vuetify locales, and right-to-left Arabic and Urdu).

Plan:

1. The codes are `en`, `ru`, `zh`, `es`, `pt`, `de`, `fr`, `ja`, `ko` and `it`, the primary subtags of browser languages, so the automatic choice keeps matching on them. `zh` is Simplified Chinese and `pt` Brazilian Portuguese; a browser set to Traditional Chinese gets Simplified rather than English.
2. Backend: a migration widens the check on `users.language`, `PATCH /me` accepts the new codes, the OpenAPI enum lists them; the language test sets one of them and refuses a code outside the list.
3. Frontend: a message file per language, typed by `en.ts`; Vuetify's locale and an `Intl` locale for each (`zh-CN`, `pt-BR`, ...). Plural forms come from `Intl.PluralRules` for every language: one form in Chinese, Japanese and Korean, "one | other" in the Western languages, three forms in Russian.
4. Fonts: system fonts for Chinese, Japanese and Korean, as PatternFly leaves them, named per language through `:lang()` so that Japanese text does not take Chinese glyph shapes (chosen over bundling Noto subsets).
5. Check the date fields' masks in Korean, Japanese and Chinese, whose short dates are written `2026. 12. 31.` and `2026/12/31`.
6. Docs: the architecture row, the development guide, the README and the changelog.

## 17. RAW photos from cameras

**Status:** done, released in 0.1.0 on 2026-11-21. Improvement 20 replaced its decode path. Still to check on the NAS: items 2, 3, 4, 6 and 7 of the list below; items 1 and 5 were covered by the run of 20 difficult files in improvement 20.

Raw files from cameras: Nikon NEF and NRW, Canon CR2 and CR3, Sony ARW, Adobe DNG (also from phones, such as Apple ProRAW), Fujifilm RAF, Olympus and OM System ORF, Panasonic RW2, Pentax PEF and Samsung SRW. Today ingest refuses them, and the README says RAW is not supported yet.

What the image already has, checked on 2026-11-20 in `debian:trixie-slim`: Debian's libvips 8.16.1 depends on ImageMagick 7 (`libmagickcore-7.q16-10`), which is built with LibRaw 0.21.4 and lists all the formats above, CR3 included, under its `dng` coder. libvips 8.16 has `magickload` and no LibRaw loader of its own. Debian's ImageMagick policy blocks no coder; it limits a decode to 1 GiB of memory, 2 GiB of disk and 256 megapixels. Nothing has been decoded yet: no RAW sample was at hand. exiftool reads the tags of every RAW format and extracts the JPEG previews cameras embed.

Where it is harder than HEIC:

- **Detection.** NEF, CR2, ARW, DNG, PEF and SRW are TIFF files; only the extension tells them from a plain TIFF, which stays unsupported. CR2 has `CR` at byte 8, ORF starts with `IIRO`, `IIRS` or `MMOR`, RW2 with `IIU`, RAF with `FUJIFILMCCD-RAW`, and CR3 is an `ftyp` file with the brand `crx `. Detection takes the content where it is enough and the TIFF header with the extension for the rest.
- **Decoding.** Opened by file, libvips would see a TIFF and load its first directory, a small preview or raw sensor data, instead of the image. `magickload` has to be called explicitly, which govips does not offer as is, and ImageMagick may sniff the TIFF header too, so a `DNG:` prefix on the path may be needed; LibRaw then finds the real format by content. `media.Dimensions` would quietly return the size of that first directory, so RAW types take another branch in `media`, which also covers the `render-previews` job and command and `imgbench`. Stored width and height must be those after rotation, like the preview's, or the grid distorts them.
- **Look and speed.** LibRaw gives a flat rendering with the camera's white balance and no lens corrections, unlike the camera's JPEG, and a 24 MP file takes seconds and some hundreds of MB of memory to decode. The embedded JPEG is the camera's own rendering and costs almost nothing, but its size depends on the camera and is to be measured in step 1; from what is known, some makers embed a full-size JPEG and others one of about 1600 px, and DNG from phones a small one or none. exiftool names it differently by maker (`JpgFromRaw`, `PreviewImage`, `OtherImage`), so the largest is taken. The extracted JPEG carries no orientation of its own; the RAW's `Orientation` tag applies, while LibRaw rotates the image itself, so the two paths must not rotate twice. LibRaw 0.21.4 may not read the newest encodings, such as Nikon's High Efficiency NEF.
- **The viewer.** No browser shows a RAW file, so the viewer never asks for the original and zooms the preview, as for HEIC outside Safari. Download gives the original.
- **RAW and JPEG pairs.** A camera set to RAW+JPEG writes `DSC_0001.NEF` and `DSC_0001.JPG`. `photo_links` was made with such pairs in mind (see the "Versions" row of the architecture).

Draft plan:

1. A trial on a few CC0 files from raw.pixls.us, one or two per maker and a portrait one, in the runtime image: an explicit `magickload` of a NEF, CR2 and DNG gives the full sensor size, not the first directory; the portrait comes out upright on both paths; the time and peak memory of decodes on the NAS's N150 with the configured number of workers; the sizes of the embedded previews. If `magickload` does not work, the alternative, such as the `libraw-bin` tools, is to be decided. The result decides step 3.
2. Ingest: MIME types such as `image/x-nikon-nef` with their extensions in `storage`, detection as above, the error message, the upload `ACCEPT` list and the extensions in `frontend/src/lib/uploadFiles.ts`, the OpenAPI description. Folder scans and WebDAV follow from ingest.
3. Previews, in this order: the embedded JPEG when its long side reaches the viewer preview size; a decode through `magickload`; the embedded JPEG of any size; otherwise the file is unreadable. A branch by MIME type inside `media`, used by every caller.
4. Viewer: no original for RAW types; the details panel names the format.
5. Pairs: a `raw` kind in `photo_links` (the `CHECK` widened, a partial unique index as for `live`) hides the RAW under its JPEG, matched by owner, base name, `DateTimeOriginal` with sub-seconds, and make and model, not by folder, since cameras and people often keep `RAW/` and `JPG/` apart; the viewer's switch to the original shows the RAW's preview. A job pairs the files already imported. As with other hidden files, the RAW goes into the trash with its JPEG.
6. RAW files already in scanned folders are picked up by the next scan, as scans only count unsupported files; Takeout entries recorded as `unsupported` are checked for a rerun.
7. Tests with small fixtures, the architecture rows, the README, the deployment guide and the changelog.

Decided on 2026-11-20: all the formats above; previews from the embedded JPEG when it is large enough and a LibRaw decode otherwise; RAW and JPEG pairs linked in this improvement, the JPEG shown and the RAW under it.

The implementation uses explicit `magickload` with a `DNG:` prefix. On Debian trixie, the ImageMagick coder handed the portrait DNG’s orientation to libvips as metadata. Both paths now apply the orientation when they make the thumbnail, and a synthetic portrait fixture covers the three preview choices. Direct decoding succeeded for one CC0 sample of every supported format from [raw.pixls.us](https://raw.pixls.us/). On a Ryzen AI 7 350, the trial of 12 files with WebP previews took 2.93 s with one worker, 1.60 s with two and 1.14 s with three. Peak process memory across the run was about 878 MB. The NAS N150 measurements are still pending.

After review on 2026-11-20, only one full decode runs at a time, whatever the number of workers; previews from the embedded JPEG do not wait. A decode that has started cannot be stopped: a job cancelled at shutdown or by its 2-minute timeout returns only when LibRaw finishes.

To check on the NAS (N150). Items 1 and 5 were covered on 2026-11-21 by the NAS run in improvement 20; the others are still open:

1. Memory: import a folder of large RAW files that need a full decode, with the default number of workers. Watch the container's peak memory with `docker stats`; it should stay near one decode, not one per worker. Most current cameras embed a preview at or above the default viewer size of 2048 (Sony since about 2024 a full-size JPEG, Fujifilm GFX 4000x3000), so they take the embedded JPEG path. To force full decodes, set the viewer preview size to its maximum of 4096 in the settings, or strip the previews from a copy with `exiftool -JpgFromRaw= -PreviewImage= -OtherImage=` (this works for ARW, not for RAF, whose preview sits in the RAF header). Measured on 2026-11-21 in the test image (LibRaw 0.21.4, Apple silicon): a 102 MP GFX100S II RAF peaks at about 2.4 GB in 1.3 s, a 61 MP RX1R III ARW without previews at 1.4 GB in 1.0 s, the same ARW from its embedded JPEG at 0.3 GB. Samples come from raw.pixls.us (CC0; downloads redirect, so use `curl -L`).
2. Time: the duration of one full decode of the largest file. It must stay well under the 2-minute processing timeout, and ideally under the 30-second grace period of a shutdown, which a running decode delays.
3. Restart during a decode: stop the app while a large RAW is decoding and check how long it takes to stop, and that the photo is processed again after the start.
4. Embedded JPEG path: the time to process a folder of NEF, CR2, CR3 and RAF files whose preview reaches the viewer size, against a folder of JPEGs of the same count.
5. Formats: one file of each supported format from a real camera, if at hand. Files that LibRaw in the image cannot decode, such as Nikon High Efficiency NEF, should still get a preview from the embedded JPEG, and the log should give LibRaw's reason rather than an empty error.
6. Pairs: a folder scan of a camera card with RAW + JPEG shows only the JPEGs, the viewer switches to the RAW, and the pair goes into the trash and back together. A rescan of a folder scanned by an earlier version picks up the RAW files it skipped.
7. Bulk import of mixed files (photos, videos, RAW): the processing queue should drain at about the same rate as before RAW support, now that photos without a pair no longer take the linking lock.

Found on 2026-11-21 with 20 hard CC0 files from raw.pixls.us, rendered at the default viewer size of 2048 in the test image (LibRaw 0.21.4, Apple silicon, one container per file limited to 4 GB):

- **Memory.** A 177 MP high-resolution shot of a Panasonic S1R II (16288x10848, embedded preview 1920 px) takes a full decode and peaks at 3.9 GB, at the container's limit before the application and the other workers count. With 4 GB the NAS would likely stop the application on such a file, and the job would run it again after the restart. Other full decodes: Sony A7R IV 1.4 GB, Samsung S23 Ultra DNG 1.5 GB, Panasonic S1R 1.1 GB, Sony A7R III 1.0 GB, Pentax K-1 II pixel shift DNG 0.8 GB.
- **Wrong colours without an error.** A Canon R5 II CR3 shot with HDR PQ on has its previews in Canon's own HEIF-like container (`CISZ`), which neither exiftool nor libvips reads, and LibRaw 0.21.4 does not know the camera: the preview comes out with a purple and green cast and the masked border of the sensor (8480x5650 instead of 8192x5464). LibRaw 0.22 knows it.
- **Embedded JPEG refused.** The S23 Ultra's full-size preview fails in libvips on a warning ("2 extraneous bytes before marker"), so the file takes a full decode it did not need.
- **Dimensions.** When the embedded JPEG is the source, the stored size is the JPEG's: 4000x3000 for a 102 MP GFX 100, 3200x2400 for an 80 MP OM-3 high-resolution shot, 4416x2944 for a 40 MP X-T5, as the architecture row says; the details panel shows these sizes.
- **Fine:** Nikon Z6III and Z5II High Efficiency NEF (LibRaw cannot decode them, the full-size embedded JPEG is used), Canon R5 dual pixel and R6 III CR3, 5DS R CR2, Sigma fp L and iPhone 12 Pro DNG, Pixel 8 Pro DNG, Pentax PEF, Samsung NX1 SRW, Fujifilm X-T5 and GFX 100 RAF.

The fixes are improvement 20.

## 18. Lightroom XMP sidecars

**Status:** draft plan of 2026-11-21, not started. Open questions 2 to 4 below are not decided yet. It builds on improvement 19.

Lightroom Classic, with "Automatically write changes into XMP" on, keeps the edits of a proprietary RAW file in a sidecar next to it: `DSC_0001.NEF` gets `DSC_0001.xmp`. DNG and JPEG files get the XMP inside the file instead (other formats unchecked), and virtual copies live only in the catalog. The goal is to read the sidecar and make the RAW's previews from its settings, so the library shows the edited photo.

What a sidecar holds:

- **Metadata:** `xmp:Rating` (0 to 5, −1 for rejected), `xmp:Label` (a color name), `dc:subject` (keywords, flat; `lr:hierarchicalSubject` keeps the hierarchy), `dc:title`, `dc:description`, `tiff:Orientation`.
- **Geometry:** `crs:HasCrop`, `crs:CropTop`, `CropLeft`, `CropBottom` and `CropRight` as fractions of the image before rotation, and `crs:CropAngle` in degrees.
- **Global tone and color, process version 2012 and later:** `Exposure2012` (EV), `WhiteBalance` with `Temperature` and `Tint`, `Contrast2012`, `Highlights2012`, `Shadows2012`, `Whites2012`, `Blacks2012`, `Texture`, `Clarity2012`, `Dehaze`, `Vibrance`, `Saturation`, the parametric and point tone curves, HSL, color grading, the camera profile (`CameraProfile`, such as Adobe Color), sharpening, noise reduction, lens profile corrections.
- **Local edits:** masks (`MaskGroupBasedCorrections`, including AI subject and sky masks), healing and cloning, AI denoise. They refer to Adobe's own processing.

Adobe's rendering is closed, and no open tool applies these settings the way Lightroom does. Darktable imports a part of them on its own import, not from a command line. Adobe offers no library that applies them, checked on 2026-11-21. The Lightroom API, a cloud service of Adobe's Firefly Services, could apply an XMP to an image; it reached end of life on 2026-07-31, and Adobe points its users to the Photoshop API v2. Any cloud service would send the library's originals to Adobe. The DNG SDK (C++) does not read `crs:` settings, but its renderer, `dng_render`, takes a white point (`SetWhiteXY`), an exposure, a shadows level and a tone curve, and applies Adobe's own colour pipeline with the camera profiles a DNG carries. It could run the approximate level below in a separate container, with the application mapping `Temperature`, `Tint` and `Exposure2012` to it. It reads DNG only, and Adobe DNG Converter, which turns a NEF or ARW into DNG, has no Linux version. So the levels are:

1. **Exact:** metadata, orientation, crop and its angle.
2. **Approximate:** exposure, white balance, the five tone sliders, vibrance and saturation, applied by the application to LibRaw's linear output with libvips. White balance from Kelvin and tint needs the camera's color matrix; LibRaw carries Adobe's matrices, so this may come close, to be checked. Profiles, curves, HSL, clarity, dehaze and lens corrections would not be applied, so a strongly edited photo would look visibly different from Lightroom.
3. **Not reproducible:** masks, healing, AI denoise.

The exact Lightroom look comes only from Lightroom itself:

- **Exported JPEG:** exported next to the RAW with the original's file name and all metadata, it pairs with the RAW today (owner, base name, capture time with sub-seconds, make and model). A later export after a new edit is a new file with a new hash, so today it would add a second photo; replacing the pair's JPEG would need a rule.
- **Publish Service:** Lightroom Classic's built-in Hard Drive publish service exports a collection to a folder, and after an edit lists the photo under "Modified Photos to Re-Publish"; one click on Publish exports the changed ones again over the old files. Pointed at the folder the application scans, with the original's file name and all metadata, it keeps the library's JPEG in step with Lightroom's exact look and needs no Adobe engine on the NAS. The application's part is the rule above: a file that replaces one already imported from the same path, with a new hash, replaces that photo's JPEG in its pair rather than adding a photo.
- **DNG with an updated preview:** Lightroom can convert to DNG and, on "Update DNG Previews & Metadata", write a full-size preview of the edit into the file, which the embedded-JPEG path already shows. This changes the Lightroom workflow and the original's hash on every update.

- **Darktable instead of Lightroom:** LinuxServer.io publishes a darktable image (`lscr.io/linuxserver/darktable`) that shows the whole desktop application in a browser: one user's remote desktop, not an editor the application could embed per user. Edits made there are darktable's own XMP (`DSC_0001.ARW.xmp`), which `darktable-cli` renders exactly, so previews could match the editor, at the cost of a darktable sidecar container and full renders on the NAS. It would replace Lightroom in the workflow; darktable reads only a part of Lightroom's settings, once, on import.

How a sidecar reaches the application and stays current:

- **Sources.** A folder scan leaves `.aae` and Takeout JSON out as sidecars and would count an `.xmp` as unsupported. WebDAV refuses it with 415, and the browser's folder upload filters it out by extension. All three would accept `.xmp`, matched to a RAW of the same owner by folder and base name, case-insensitive. When a RAW and a JPEG share the base name, the sidecar belongs to the RAW.
- **Storage.** The library keeps originals by hash, and a sidecar changes on every edit, so it is not a photo: a table keeps the XMP text, its hash and the photo, one current sidecar per RAW. A sidecar that comes before its RAW waits, by path, for the RAW to arrive.
- **Updates.** A folder scan reads a sidecar again when its size or modification time changes, even if the RAW is already imported, and a WebDAV PUT over it replaces it. A changed sidecar queues `render-previews` for its photo. A removed sidecar is not seen by a scan, so the last one stays.
- **Pairs.** A RAW with edits and the camera's JPEG: today the JPEG is shown and the RAW hidden under it. With a sidecar, the edited RAW is the photo worth showing, so the pair may need to show the RAW and hide the JPEG, or show the JPEG with the RAW's metadata.
- **Cost.** Edits apply to the sensor data, so a photo with a sidecar needs a full decode at every render, about 1.4 GB for 61 MP and 2.4 GB for 102 MP, one at a time; on the NAS's N150 likely several seconds each, to be measured. Crop and orientation alone could apply to the embedded JPEG, which costs almost nothing. A Lightroom library of thousands of edited RAW would take hours to process, and every change of a setting such as the viewer size would render them again.

Draft plan:

1. A trial: a few RAW files with sidecars from a real Lightroom catalog (light and heavy edits, a crop with an angle, a portrait, a rejected and a five-star one), and Lightroom's own export of each for comparison. Parse the XMP with exiftool, which the image has (`-xmp:all`, `-XMP-crs:all`). Measure a full decode with the approximate settings on the N150.
2. Storage and matching: the sidecar table, acceptance of `.xmp` in folder scans, WebDAV and the browser, waiting sidecars, rescans on change.
3. Metadata: rating, label, keywords, title and description into the application's fields, as chosen below.
4. Previews: crop and orientation, then the approximate tone and color if chosen, in `media` behind the RAW branch, with the sidecar's hash part of what decides that previews are current.
5. Pairs, the viewer (an "edited in Lightroom" mark, download of the RAW with its sidecar), tests with small fixtures, the architecture rows, the README, the deployment guide and the changelog.

Decided on 2026-11-21: the workflow stays on Lightroom Classic. The files reach the NAS as a share Lightroom works in: originals with their sidecars in one folder, exports in another, both under the import folder. The catalog itself stays on the Mac, as Lightroom Classic does not open a catalog on a network drive. Today a folder scan copies the originals into the library, doubling the space RAW files take, and runs only when an administrator starts it; the files are to stay where they are.

Open questions:

1. Decided on 2026-11-21: the share is indexed in place, without copying; see improvement 19, which this one builds on.
2. How far to go: the Publish Service as the shown photo, with the sidecar for metadata (exact look, a click in Lightroom after edits); metadata, crop and orientation only (exact, cheap); or those plus the approximate global settings, with libvips or the DNG SDK (closer, still visibly different from Lightroom, a full decode for every edited RAW).
3. What the application does with each metadata field: the rating (the application has favorites, not stars; five stars as a favorite, or a new rating field), rejected photos (hidden, into the archive, or nothing), keywords as tags, title and description into the description.
4. Which photo a RAW + JPEG pair shows once the RAW has edits.

Pairing keeps the file name (decided on 2026-11-21): a pair needs the same base name, capture time with sub-seconds, make and model, so a repeated name alone never pairs. Exports must keep the original's file name; matching by Lightroom document IDs or the body serial number was considered and left out.

## 19. Library folders indexed in place

**Status:** planned on 2026-11-21, not started.

RAW files that Lightroom works on are kept in a share on the NAS, and the application is to show them without copying them into its library (decided on 2026-11-21, see improvement 18). Today every source goes through `Ingest`, which moves the file to `originals/<owner>/<hash>` (`storage.Layout.Original`), and a folder scan copies, hashes every file on every run and is started by hand. This improvement adds a second kind of folder: a library folder, read where it is, rescanned on a schedule, with the files allowed to change, move and disappear. It is the base for improvement 18: the sidecars, the pairs with Lightroom's exports and their re-publishing all happen in such a folder.

What assumes today that the original is in the library:

- Eight places build the path from the hash: `ingest.store` (moves the file in) and `ingest.Purge` (removes it), `api/photos.go` (download and the original in the viewer), `webdav.go` (GET), `jobs.go` (processing, the motion and video HDR backfills), `transcode.go` (the source of a copy). The ML service reads the viewer preview, not the original, and is not affected.
- A photo's hash is its identity: `UNIQUE (owner_id, sha256)`, duplicates, the record a permanent delete leaves behind, the lock in `LockPhotoFile`. An original never changes.
- The transcoder reads originals at the same paths as the application, from the library mounted read-only.

Design:

1. **Configuration.** A mount for library folders, `/data/folders`, read-only, separate from `/data/import`. An administrator adds a library folder in the interface: a subfolder of the mount and its owner, one owner per folder. Photos from it are the owner's like any other; others see them through albums and shares.
2. **Where an original lives.** A table `photo_files` keeps, for each photo from a library folder, the folder, the relative path, size and modification time. A photo without a row has its original in the library as today. One function, used by the eight places above, gives a photo's original path. A file whose path is also in the library (the same hash uploaded earlier) keeps the library copy as its source.
3. **Scanning.** A scan lists the folder and compares each file with its row by size and modification time. Only new and changed files are hashed, so a rescan of a large folder reads directories, not files. A file whose size or time is still changing waits, as today. A scheduled job rescans each library folder at an interval set per folder, every 10 minutes by default, and the folder page has a button to scan now. A scan that is still running when the next is due is not started twice. A watcher for changes (inotify) is not part of this improvement. Hidden files, `@eaDir` and `#recycle` are left out as today; `.xmp` files are recorded for improvement 18 and not ingested as photos.
4. **Identity is the path.** A `photo_files` row is keyed by folder and path, and outlives a missing file and a permanent delete. A file at a path the folder already knows is always that row's file, unchanged or changed, whatever its hash; a hash is matched only for a path the folder has never seen. Lightroom with "Automatically write changes into XMP" rewrites JPEG, DNG and TIFF originals on every rating or keyword, and the Publish Service may delete a file before writing it again, so matching by hash would turn edits into new photos.
5. **New path.** Hashed in place, then `Ingest` without copying, with the same processing job. If the owner has no photo with that hash, it becomes a new photo with this file as its source. If the hash belongs to a photo that went missing in this folder, it is that file moved or renamed: the row takes the new path and nothing is processed again. Otherwise the file is a duplicate: its row is kept, marked as such, and never becomes a source; a photo in the trash or purged stays there, as with any scan. One photo has one source file, and a photo whose original is in the application's library is never re-pointed to a folder.
6. **Changed file at a known path.** The photo keeps its id, albums, tags, favorites, shares and its RAW pair, and takes the new hash; its previews, motion, copy, EXIF and embedding are made again. The change takes the file lock (`LockPhotoFile`) on the old and the new hash in a fixed order, as a purge or an upload may race it. If the new hash belongs to another photo of the owner, the two are not merged: the file is reported and left. A purged photo's row stays purged whatever its content, so an edit in Lightroom does not bring it back. Every rating or keyword set in Lightroom on a JPEG or DNG original changes the file, so it costs a full processing, for a DNG a full decode; RAW files with sidecars do not change.
7. **Missing file.** A file not found by two scans in a row (so a Publish Service that deletes before it writes again is not caught in between) is marked missing and its photo goes to the trash, with the reason kept. In the trash the photo shows a "file not available" notice, and download and the original zoom say the same; the previews stay. When the path comes back, with the same or another hash, the row is found again by its path, and a photo the scan moved to the trash comes out of it; one the owner moved there stays. The trash has no expiry, so nothing is purged while a file is away. A scan that finds the folder itself missing or empty stops without marking anything, as an unmounted share would otherwise send every photo to the trash.
8. **The application's own deletes.** The folder is mounted read-only, so the trash and a permanent delete never touch the file. A permanent delete removes previews and keeps the record with the hash and the path row, so the next scan skips the file; the folder page lists such skipped files.
9. **Unavailable originals in jobs.** Processing, `render-previews` (all photos, after a change of the preview settings), the motion and video HDR backfills and transcoding read the original. When it is missing or the share is down, they skip the photo and leave it for the next run instead of marking it failed, so a settings change during an outage does not fail a whole folder.
10. **Other services.** The transcoder mounts `/data/folders` read-only at the same path. The nightly backup covers the database only, as today; the originals in library folders are backed up with the share, which the deployment guide says.
11. **Interface.** An administration page for library folders: path, owner, last scan, counts of photos, missing, skipped and failed files, the scan button. The viewer's details name the file's path in the folder, and a missing file shows a notice instead of the download.

Draft plan:

1. Migration: `library_folders` (path, owner, scan interval, last scan) and `photo_files` (photo, folder, path, size, mtime, first scan that missed it, missing since, whether the scan put the photo in the trash). The original path function and its use in the eight places, with tests that a photo from a folder is read from there and never removed.
2. Scanning a library folder: listing, comparison with `photo_files`, hashing of new and changed files only, moves, missing files and the guard against an empty folder. Ingest without copying.
3. A changed file: the photo takes the new hash under both file locks and everything derived is made again, with the current processing and render jobs; the jobs that read originals skip an unavailable one rather than fail.
4. The scheduled job and the scan button; the administration page; the viewer's notice for missing files.
5. Deployment: the `/data/folders` mount in the three compose files and the TrueNAS YAML, the transcoder's mount, permissions for UID 568, a note on backups.
6. A check on the NAS with a copy of a real Lightroom folder: the first scan's time for the share's size, a rescan's time, a re-publish replacing a JPEG in a pair, a move in Lightroom, an unmounted share.
7. Tests, the architecture rows ("Folder scan", "Permanently deleted photos", a new "Library folders"), the README, the deployment guide and the changelog.

Decided on 2026-11-21:

1. A missing file sends its photo to the trash, where it shows with a notice; it comes back out when the file does, unless the owner put it there.
2. The scan interval is set per folder, every 10 minutes by default.
3. Subfolders do not become albums.

## 20. Safer RAW decoding

**Status:** done on 2026-11-21, released in 0.1.1. All 20 difficult files were imported on the NAS on 2026-11-21, each with a preview. On 2026-11-22 the Canon R5 II CR3 preview was checked by eye: its colours are right, and its size is 8222x5488. That is LibRaw's visible area, without the masked border of 8480x5650 and 30x24 pixels larger than the camera's own 8192x5464 crop, as LibRaw's inset crops are not applied.

From the trial of 20 hard files in improvement 17 ("Found on 2026-11-21"). Samples are the files named there, from raw.pixls.us; the check is a render of each at the default viewer size in a container limited to 4 GB, watching the peak memory.

1. **A ceiling on full decodes.** Above a pixel count, such as 120 MP, a full decode is not run: the largest embedded preview is used however small, or, if LibRaw's half-size mode can be reached through ImageMagick's `dng` coder (to check: its `-define` options), the file is decoded at half size, a quarter of the memory and still above the viewer size. A 177 MP Panasonic S1R II shot peaks at 3.9 GB today, at the container's 4 GB limit, and would stop the application on the NAS, then again after every restart, as the job runs again. The ceiling may be a setting next to the preview settings. Test: the S1R II file stays well under the limit and gets a preview.
2. **Embedded JPEGs that libvips warns about.** Load them without failing on warnings (libvips `fail` off, the default of `vips_jpegload`'s `fail_on` being checked in govips), so a Samsung S23 Ultra DNG takes its full-size embedded JPEG instead of a 1.5 GB decode. A JPEG that does not decode at all still falls through to LibRaw.
3. **No wrong image from a camera LibRaw does not know.** LibRaw 0.21.4 decodes a Canon R5 II CR3 with the colours and the masked border of an unknown camera, without an error. When LibRaw reports the camera as unknown, or ImageMagick's coder gives no colour matrix for it (to check which signal is reachable), use any embedded preview, and if there is none, mark the photo failed with a message that names the camera, rather than show a wrong picture. Canon's HDR PQ previews, in Canon's own container (`CISZ`), are not readable; this case then fails clearly.
4. **LibRaw 0.22 in the image.** Checked in the Debian archive on 2026-11-21: trixie has LibRaw 0.21.4 (`libraw23t64`), ImageMagick 7.1.1.43 and libvips 8.16.1; testing (forky) has LibRaw 0.22.3 (`libraw25`, a new soname), ImageMagick 7.1.2.35 and libvips 8.18.7, whose Debian build links ImageMagick, not LibRaw; trixie-backports has no LibRaw. Installing `libraw25` alone changes nothing, as trixie's ImageMagick links `libraw23t64`. The ways:
   - the image on `debian:forky-slim`: consistent, but testing moves under the build and has no security support to speak of;
   - testing's ImageMagick pinned into trixie: pulls much of testing with it, fragile;
   - **a backport of our own** (chosen on 2026-11-21): a build stage on trixie rebuilds Debian's `libraw` 0.22.3 source package, the runtime installs that `.deb`, and the application calls LibRaw through cgo instead of `magickload`. ImageMagick and libvips stay Debian's with their security updates; only LibRaw is ours to follow, until forky is stable. Calling LibRaw directly also gives fix 1 its half-size mode (`half_size`) and fix 3 its signal for a camera without colour data, neither of which ImageMagick's coder exposes. It replaces the decode path of improvement 17.
   Test: the R5 II file decodes with the right colours and size.

Order: 2 first, as a small change of its own. Then 1, 3 and 4 together: the LibRaw 0.22.3 backport stage in `deploy/Dockerfile`, a cgo binding to LibRaw in `media` in place of `magickload` for full decodes, with `half_size` above the pixel ceiling and a refusal for a camera without colour data. The embedded-preview path, the one-decode-at-a-time slot and the 4 GB limit stay. Tests run against the image's LibRaw with the fixtures of improvement 17; the 20 hard files are checked again as in "Found on 2026-11-21". When forky becomes Debian stable, the backport stage goes.


Implementation uses direct LibRaw 0.22.3 calls with the backported Debian package. Mosaic sensors over 120 MP use half size up to a 240 MP sensor ceiling. The unpacker also has a 1536 MiB allocation limit, and a large input that LibRaw cannot reduce to half size falls back to the embedded JPEG. Multi-colour sensors with no colour conversion are refused. The original, upright dimensions are kept for a successful half-size decode.

Local checks on 2026-11-21 used LibRaw 0.22.3 and host libvips, measuring peak process RSS outside Docker:

- Samsung Galaxy S23 Ultra 50 MP used the 8160×6120 embedded JPEG despite libjpeg warnings, at about 130 MiB.
- Panasonic DC-S1RM2 177 MP used its 1920×1280 embedded JPEG after LibRaw returned an I/O error during unpack, at about 65 MiB.
- A synthetic 123 MP Bayer DNG exercised the half-size sensor path and kept its 9600×12800 upright source dimensions, at about 593 MiB.

On 2026-11-21 the problematic RAW samples were uploaded to the NAS (Intel N150). All uploads succeeded, with no memory issues seen. No peak memory or timing figures were reported for this run.

A timed NAS run on 2026-11-21 imported all 20 difficult RAW samples in 102.1 s on the Intel N150 at 800 MHz with Turbo disabled. Magellanica Photos and PostgreSQL together peaked at 1,392 MiB of cgroup working set sampled every 250 ms. The test used three workers and the default 2048 px WebP viewer preview. The 177 MP Panasonic used its 1920×1280 embedded preview. See the [NAS comparison](benchmark.md#comparison-with-immich-and-photoprism) for the full results.

## 21. Map

**Status:** done on 2026-11-22, released in 0.2.0. The Map page, the clusters API, the `bbox` filter and the viewer's link work, with a backend test (`map_test.go`) and a step in the browser test. Not yet checked on a phone or with a large library.

A page with the user's photos on a map, by the GPS location of each, as Immich, Nextcloud Memories and Google Photos have. The viewer today links a photo's coordinates to openstreetmap.org in a new tab (`PhotoViewer.vue`); nothing shows many photos at once. Place names ([improvement 8](#8-place-names-in-the-viewer)) are a separate piece and do not block this one.

What the code gives: a location is read out of the EXIF JSON when a photo is sent (`metadata.Details`), or comes from an import (`imported_latitude`, `imported_longitude`, which apply only when the EXIF has none). The API returns it only when an administrator allows locations. There is no column to query by, so the first step is a place for the numbers.

Decided on 2026-11-22 (the options considered follow):

- **Where the map tiles come from.** This is the privacy decision: whoever serves the tiles learns what area the user looks at, though never which photos are there. The [place names](architecture.md#decisions) set the bar as "coordinates never leave the NAS, and no internet is needed".
  - OpenStreetMap's public tile servers from the browser: no setup, but the user's IP and viewed areas go to a third party, and its tile policy discourages heavy use.
  - The NAS fetches and caches tiles for the browser: IPs stay home, but the NAS still asks OpenStreetMap for every area viewed, and its policy expects a User-Agent that identifies the application with a contact, which this project has no place to put.
  - Vector tiles from a file on the NAS (Protomaps PMTiles): no outside requests at all, provided the style, the fonts (glyphs) and the sprites are served from the NAS too, as MapLibre's defaults fetch them elsewhere. Two dependencies: `maplibre-gl` and `pmtiles`. The Protomaps world build is about 120 GB; `pmtiles extract` cuts a region by bounding box and zoom, and an extract of a city at full zoom was 88 MB, with each zoom level roughly doubling the size. The administrator downloads the extract once.
- **The library** follows from that choice: Leaflet (raster tiles, small) for the first two, MapLibre GL JS for PMTiles.
- **What a click on a cluster does:** zooms in, or opens the photos of the cluster under the map at any zoom.
- **How it is entered.** A "Map" page in the menu, and a "Show on map" link in the viewer, replacing the external link or next to it.

The choices: tiles from OpenStreetMap's public servers, straight from the browser (OpenStreetMap sees the user's own IP, the viewed areas and the address of the site as Referer; the VPS in front of the NAS is not involved); a click on a cluster zooms in, and at the closest zoom or on a single photo lists the cluster's photos under the map; the viewer's link opens the Map page centred on the photo and replaces the link to openstreetmap.org, which sent the coordinates there. The library started as Leaflet and was replaced by MapLibre GL JS the same day, as Leaflet 1.9 prints a flag before its name in the credit line and the user did not want it; MapLibre also reads vector tiles, so PMTiles from the NAS can come later without another library. The map's chunk is loaded only on its page (about 300 kB gzipped).

Decided on 2026-11-22: whose photos are on the map, as on the memories page and the timeline with shared photos: the user's own and those shared with them, without the user's archive, the trash and hidden versions.

Plan, as built, with these differences: the `bbox` filter is on `/photos` and `/photos/months` only (the facet lists of `/photos/cameras` ignore it, and the box still answers 403 there where locations are hidden); the menu item is always shown, and the page says that locations are hidden when the API answers 403, as the setting is the administrator's and the API gives other users no way to read it; MapLibre's zoom counts 512 pixel tiles, so the API gets its zoom plus one; the browser test uploads the located `rotated.jpg` for the administrator and purges it at the end of its step, as the other users' photos are not on the administrator's map.

1. **Coordinates in the database.** Generated columns `latitude` and `longitude` on `photos`, as `camera` and `lens` are: `COALESCE` of the EXIF value and `imported_latitude`/`imported_longitude`, so a location an import records later is on the map with no job. The SQL expression is immutable and accepts what `location()` in `metadata.go` accepts: numbers (checked with `jsonb_typeof`), the range of 90 and 180, the sign from the reference letter, and 0,0 as no fix; the two are kept in step as `is_raw_mime` and `storage.IsRAW` are, with a test that runs both on the same tags. An index on them for bounding boxes.
2. **API.** `GET /photos/map` takes the bounding box and the zoom and returns clusters (the centroid of its photos, their number, a cover photo) computed by grouping into a grid whose cell follows the zoom, on the Mercator y rather than the latitude so that cells do not stretch near the poles. A `bbox` filter joins the timeline filters (`ListTimeline`, `CountTimelineMonths`, `ListCameras`, `ListLenses` and `timelineFilter`), so that the photos of a cluster open in `PhotoBrowser` with the viewer, selection and bulk actions. On the frontend it also goes into `listFilters`, `withoutFilters` in `stores/photos.ts` and `filtered` in `TimelineView.vue`. Both endpoints answer only when an administrator allows locations, and so does `bbox` on `/photos`, `/photos/months` and `/photos/cameras`: otherwise anyone could find the coordinates of shared photos by narrowing the box.
3. **Page.** `MapView.vue`, loaded lazily so that the map library stays out of the main bundle: the map with cluster markers showing a thumbnail and a count; a click as decided above. The viewer's "Show on map" opens the page centred on the photo. The menu item and the link are hidden when locations are off.
4. **Tiles.** As decided above, with the attribution the source asks for; the application sets no Content-Security-Policy today, so no header changes. With PMTiles, the administration page says where the file is and the route that serves it supports range requests, as originals do.
5. **Tests.** The backend: clusters at two zooms, the bounding box across the date line, an imported location, shared and archived photos, and no answer when locations are hidden. One step in the browser test after "an administrator shows where photos were taken", which already has a located photo; it checks the markers and counts and does not depend on tiles.

## 22. Demo activity log

**Status:** Implemented. Public demo rollout pending.

Decided on 2026-11-23: keep web login and upload events in PostgreSQL across demo resets. Start with operator-only SQL reports, 30 days of retention, a row cap, browser and OS, and a separate random session ID for grouping actions. Omit client IP addresses and defer an admin page for the first release. The shared demo account makes session counts more informative than account counts.

1. **Schema and activation.** Add a Goose migration for a dedicated activity schema and use sqlc for inserts and document read-only SQL reports. Enable recording explicitly on the demo and default it off elsewhere. Use a timestamp with time zone, action, outcome, HTTP status, optional account ID and session ID, browser and OS family, and optional media type, size in bytes, media ID, and reset generation. Use historical IDs without foreign keys to library tables. Use the reset generation with media IDs because the restore can rewind sequences. Omit raw user agents, submitted login names, credentials, cookies, authentication tokens and hashes, file names, and file contents.
2. **Web login.** Generate an independent random activity session ID when a web session is created and store it with the session so authenticated uploads can use it. Leave it empty for rejected attempts or sessions created before the migration. Record successful session creation, rejected credentials or disabled accounts, invalid requests, and internal errors with separate outcomes. Preserve the existing authentication responses.
3. **Web upload.** Record one event per request that reaches the authenticated upload handler. Distinguish newly accepted files, duplicates, restored files, rejections, and server errors. Keep the ingest outcome if album insertion or response preparation fails after ingest. Use the HTTP status to expose that failure in reports. An accepted upload can still fail background processing. Proxy rejections, WebDAV, folder imports, and background processing are outside this scope.
4. **Writes and service checks.** Use a short bounded write timeout and warn in the service log if the event cannot be stored. Preserve the operation result and avoid retrying the upload or login. Give the maintenance script a deployment-only secret marker to suppress its own login events after validating the marker on the server. Keep the marker independent of authentication and out of logs. A loopback peer alone is insufficient because the tunnel connector uses loopback.
5. **Reset and retention.** Explicitly exclude the activity schema when creating the demo baseline. Verify the real `pg_restore --clean` command against both a pre-migration baseline and a regenerated baseline in a disposable database. Keep the reset generation in the activity schema and advance it before accepting traffic after a restore. Prune old rows in batches under the maintenance lock before checking disk pressure. Start with a proposed target of 100,000 retained rows alongside the 30-day cutoff, and check table and index size before deployment. Periodic pruning makes this a retention target. Verify PostgreSQL can reuse space after pruning and account for the journal in the disk budget. Check whether ordinary backup jobs include it and apply the retention policy to any retained copies.
6. **Reports.** Provide read-only SQL examples grouped by day in Europe/Berlin for successful and rejected logins, upload outcomes, and distinct sessions with newly accepted or restored uploads. Count uploads from older sessions with no activity ID in upload totals. Treat the session count as an activity measure for a shared account.
7. **Verification and rollout.** Cover recording disabled, login outcomes, maintenance logins, session correlation, new files, duplicates, restores, media rejection, album failures, and write failures in backend tests. Verify age and row pruning, the reset generation, and continued recording after both reset triggers using a disposable demo database. Deploy the migration with recording disabled, verify the baseline and maintenance configuration, then enable recording and check a smoke-test login and upload in the SQL report.
