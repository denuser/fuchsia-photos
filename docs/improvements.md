# Improvements

The current list of improvements, in order of priority. All of them are done except 6, 8, 9 and 10, which are planned. Stages 0 to 6 of the [roadmap](roadmap.md) are done too.

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

**Status:** planned, to be done separately.

The user selects an object in a photo, such as a house, and gets the other photos of the same object or place. The SigLIP 2 vector describes the whole frame by its meaning, so it finds similar houses rather than the same one, and misses photos where the object takes only a corner of the frame. The experiment checks how far the current vectors go before anything is added to the API or the interface.

Plan:

1. A script in `ml/`, with no changes to the API or the interface. Test data: two or three photos of the same house or place from different angles, added to the development library if `samples` has none.
2. Candidates: crop the selected region from the viewer preview, embed it with the existing image embedding and take the nearest photos. When the photo has a location, also try a filter by distance from it: for the same place it is often the strongest signal and needs no ML.
3. If the results are similar but not the same object: re-rank the first 100–200 candidates by local features with geometric verification (LightGlue with ALIKED or DISK features, RANSAC). Not SuperPoint, whose weights have a non-commercial license. In the service this would be a new stateless endpoint, such as `POST /match`, taking the crop and the candidates' previews and returning the number of matched points for each.
4. If objects in a corner of the frame are still missed: try several vectors per photo for parts of the frame, such as a 2×2 grid and the center, or DINOv2 (Apache 2.0) as a second vector type, which tells specific objects apart better than SigLIP.

Record the results here: what each step found on the test photos and how long it took. Unlike similar photos, a search by a selected region needs the ML service to be running. Any new model or dependency is chosen with the user before it is added.

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

**Status:** steps 0 to 4 done on the branch `takeout-import`, not merged into `master` yet; step 5, versions, is next. Steps 1 to 3 were checked on 2026-11-15 on the development stack with the user's real exports: 2333 files, 1135 JSON files, 70 edited copies, no file without JSON and no JSON without a file, and dates and places applied as the rules say. Google Takeout comes first, iCloud after it.

People leaving Google Photos or iCloud take their library with them as an export. The files are there, but part of what the service knew about them is not in the files: dates and locations of photos without EXIF, places the owner corrected, descriptions, favorites, the archive and albums. An import reads these next to the files.

Settled with the user on 2026-11-15:

- Both services are imported from their exports only. Google removed reading a whole library from the Photos Library API on 2025-03-31, so a token gives nothing to import. iCloud Photos has no public API, and the private web API of icloud.com, which icloudpd uses, needs the Apple ID session to be renewed with a 2FA code every two months, does not work with Advanced Data Protection and breaks when Apple changes the login. icloudpd is not used, neither inside the application nor next to it.
- An import is a kind of folder scan, run by an administrator for an owner in the same scan root. It reads an unpacked export, `.zip` parts and `.tgz` parts, so the parts do not need unpacking by hand.
- Metadata from the export fills what the file lacks or got wrong. An EXIF date with an offset stays. A date without an offset, or no date, gives way to `photoTakenTime` from the JSON: the application reads a date without an offset as UTC and the interface shows dates in the browser's time zone, so such photos already appear shifted by it (by 3 hours in Moscow), while the Takeout time is the real moment, with the time zone and the owner's corrections that Google applied. Google Photos lets the owner add a location only to a photo whose file has none; it cannot change a location from the file. So a `geoData` that is not 0,0 fills the location of a file without one, and a file's own location stays. A `geoData` of 0,0 means "unknown" and never clears a location the file has. The shift of dates without an offset outside imports is a problem of its own, not solved here.
- An edited copy (`IMG_1-edited.jpg`, or the word in the language of the account) is linked to its original as a version of it, the way a Live Photo video is linked to its photo. Both use one link, "hidden under", with the kind of link in a field of its own, in place of `live_photo_id`, so that the lists keep one condition; pairs of RAW and JPEG could use it later. Until the versions step, edited copies are imported as photos of their own, and the import remembers each pair, so that a job links the pairs already imported with no new import. The grid and the viewer show the edited copy, as Google Photos did, with a switch to the original in the viewer. The date, location and favorite come from the original's JSON. The hidden version follows the shown one into the trash, albums and shares.
- Photos that other people added to a shared album are imported for the owner of the import, so that the album is whole.
- Descriptions, favorites and the archive are imported, and each becomes a feature of the application:
  - A description is shown in the viewer's details and its owner can edit it there. Searching by description comes later.
  - A favorite is each user's own mark, on any photo they can see. Others do not see it.
  - The archive is each user's own too: an archived photo leaves that user's timeline and stays in albums, search and an "Archive" page.

Plan for Google Takeout:

0. **A real export first.** The naming rules below come from reports by other projects, and Google has changed them before. Before the matcher is written, the user makes a small Takeout of one or two albums with an edited photo, a screenshot and an iPhone Live Photo in them, and `testdata` is built from its real names. It answers three questions: whether an edited copy has no JSON of its own and shares the original's, which would find it by "no JSON, same folder and extension, the original's name and a suffix" with no list of words in each language; whether the photo and the video of a Live Photo still share `ContentIdentifier` after Google, as pairing by name is ruled out in the [architecture](architecture.md#decisions); and what the year folders and `metadata.json` of albums look like.

   What a real export showed on 2026-11-15: two one-part exports of an account in Russian, a `.zip` of the whole library (837 files: JPEG and one `.3gp`) and a `.tgz` of one album, whose entries are the same as that album's folder in the `.zip`. The `.zip` sets the UTF-8 flag on its names; macOS `unzip -l` still shows them garbled.
   - **Edited copies: answered, yes.** All 68 files without a JSON are edited copies, `IMG_1-отредактировано.jpg` next to `IMG_1.jpg`; every JSON has its file. The word is the last `-` segment (`20190921-_MG_5983-отредактировано.jpg`), and all 68 pairs have the same EXIF `DateTimeOriginal`, a check that needs no list of words.
   - **Live Photos: answered in a second export, yes.** See below.
   - **Folders.** Year folders are localized too (`Фото 2013 г.`). The album file is `метаданные.json`, not `metadata.json`, while the sidecar suffix `.supplemental-metadata.json` stays English. An album's file has `title`, and sometimes `description`, `access` and `date`. The root of `Google Фото` also holds `названия-воспоминаний-созданные-пользователем.json`, a list of titles, which is neither an album nor a sidecar.
   - **Albums and year folders.** A photo in an album is also in its year folder, byte for byte (356 such files). The two JSONs differ in `url`, `creationTime`, `imageViews` and `sharedAlbumComments`, which only the album's copy has; `photoTakenTime`, `description` and `favorited` agree.
   - **Dates.** A Lenovo photo has EXIF `13:58:10` with no offset and `photoTakenTime` `10:58:10Z`, three hours apart; a Canon photo with `+03:00` matches its `photoTakenTime` exactly. This confirms the date rule.
   - **Not seen.** `geoDataExif` is in no JSON and every `geoData` is 0,0, so the location rule is unchecked. `archived` and `trashed` do not appear, and `favorited` appears only when true. No name has a `(1)` or a cut suffix, and the longest name is 52 characters, so those rules remain reports from other projects.

   A second export on 2026-11-15: one album of the same account, with an iPhone Live Photo (`IMG_0022.HEIC` and `IMG_0022.mov`), a phone JPEG with GPS and three 4K MP4 videos, uploaded through the web; the user added a location to the Live Photo in Google Photos.
   - **Live Photos.** The photo and the video each have a JSON of their own, with the same `photoTakenTime`; only the photo got the location. Both still carry the same `ContentIdentifier`, so pairing works as for any upload.
   - **Files are not always the originals.** The MOV and the MP4 files are byte for byte the uploaded ones. The HEIC came back as a JPEG under the name `IMG_0022.HEIC`, at the same size, with EXIF and the Apple maker note kept, and the JPEG came back recompressed (1.5 MB instead of 3.9 MB, same 4000x3000 and EXIF). The account uploads in "Storage saver" quality, which explains it; an account on "Original quality" should get its photos back unchanged. Ingest detects types by content, so the name does not matter, but such a photo does not match a copy of the original already in the library by hash.
   - **Locations.** A location added in Google Photos to a photo without GPS is in `geoData`, with a non-zero `latitudeSpan` and no `geoDataExif`. A photo with GPS has `geoDataExif` equal to `geoData`. A video whose file has GPS (QuickTime `GPSCoordinates`) got `geoData` 0,0 and no `geoDataExif`, so Google did not read it, and 0,0 must not clear the file's location.
   - **Dates.** A JPEG with EXIF `08:30:14` and no offset has `photoTakenTime` `04:30:14Z`, four hours apart (Tbilisi): the same rule again.
   - **Still not seen.** A screenshot, `(1)` and cut names. The album's file has `access: protected`.

   Two more exports on 2026-11-15, after the user archived one photo of an album and moved another to the trash:
   - **Archive.** The archived photo stays in its album's folder, its JSON there has `"archived": true`, and it is also in a folder of its own, `Отправить в архив` (the account's word for "Archive"), with the same flag. The folder has no album file, so the album rule leaves it out.
   - **Trash.** The trashed photo left its album's folder and is only in `Корзина` ("Trash"), with `"trashed": true` and no `googlePhotosOrigin`.
   - Both flags, like `favorited`, appear only when true. The import reads the flags and not the names of these folders, which depend on the language.
1. **Archives and resuming.** A Takeout import is a kind of scan the administrator chooses in the scan form (settled on 2026-11-15); a plain folder scan opens no archives and skips `*.supplemental-metadata.json` without a report, as it does `.aae`. The job lists the parts (`.zip`, `.tgz`, `.tar.gz`) and the loose files of an unpacked export, waits for them to stop changing as a folder scan does, and reads them in order. A folder may hold several exports, as a Downloads folder does: parts are grouped by the time in their name (`takeout-20261115T073434Z-1-001.zip`), an unpacked `Takeout` folder is an export of its own, and an entry is known by its export and its path in it, as the same path can hold different JSON in two exports. Every JSON file is kept in the import's tables with its path, and every photo with the path it came from and its photo, duplicates included, so that albums in step 4 find each copy.
   A `.zip` entry is opened twice, once to hash it and once to store it, so a file the library has is not written again; a `.tar` entry has no random access and is written to a temporary file while it is hashed. A `.zip` part continues from the next entry after a pause or a restart. A `.tgz` part is read in long slices, as finding an entry means unpacking everything before it: a slice that ends in the middle reads the part again from its start, skipping the entries done.
2. **Matching.** Once every part is read, the run goes to `matching` and matches each JSON to its file, within one export and one folder. A JSON of a photo is told from that of an album by `photoTakenTime`. The name of the JSON varies: `IMG_1.jpg.supplemental-metadata.json`, the suffix cut short (`.supplemental.json`, `.supplementa.json` and so on) when the name is long, file names themselves cut at 47 characters, and a `(1)` for repeated names in more than one place in the name. So the match tries, in order: the whole name; the `title` inside the JSON with the `(n)` of both names; a `title` the file's name was cut from; the file's name followed by a cut suffix or none, for JSON without a title. A candidate that is not the only one is not taken. Edited copies are matched to their originals in the same folder, among the files still without a JSON after all other matching: the last `-` part of the name is dropped, before or after its `(n)`, and the original must be in the same folder with the same extension. No list of words is needed. The check of the same EXIF date moves to step 3, the first to use the pair: an edited copy takes the original's metadata only when processing has read the same date from both, and the matching, which runs before processing, records the pair as a candidate. Each file keeps the path of its JSON or its original in `takeout_entries`, and the run counts edited copies, files without JSON and JSON without files.
3. **Metadata.** `imported_metadata` keeps the whole matched JSON of each photo, with `photoTakenTime` and `geoData` read out of it. When a photo is in several exports, as when the owner exports again after a change, the newest export's JSON wins, by the time in the names of its parts; an unpacked folder counts as older than any export (settled with the user on 2026-11-15). A photo's own JSON comes before its original's. One function in `jobs` works out the date and the location by the rules above, from the stored EXIF and this table: processing calls it after each photo and the matching after it has saved the metadata, so it does not matter which comes first. The date goes into `taken_at` and `sort_at`; a location goes into `imported_latitude` and `imported_longitude` of `photos`, which the viewer shows when the EXIF has none. An edited copy takes its original's metadata only when processing has read the same EXIF date from both. A photo already in the library, matched by its hash, gets the metadata too. Keeping the JSON lets the later steps fill descriptions, favorites, the archive and versions by a job, as `backfill_motion` does, after the export is deleted.
4. **Albums.** An album of one owner is known by its title, so an album that is in several exports, or imported again, is one album (settled with the user on 2026-11-15). A folder that holds a JSON other than a sidecar, with a `title`, becomes an album with that title. The file's name depends on the language of the account (`metadata.json`, `метаданные.json`), so it is not looked up by name. Year folders such as "Photos from 2019", whose name depends on the language too, have no such file and are not albums. Other JSON files outside albums, such as the list of memory titles in the root, are skipped and not counted as JSON without files. A file in both places is stored once and added to the album; comments in `sharedAlbumComments` are on the album's copy only.

   How it is built (reviewed and settled with the user on 2026-11-15):
   - The matching makes the albums, in the transaction that records the matches, so a run that stops and resumes makes them again with the same result. An album file is a JSON entry with no `photoTakenTime` and a `title` that is a non-empty string, in a folder right under the product folder (`Takeout/Google Фото/<album>/метаданные.json`). In the real exports the list of memory titles lies one level higher and its `title` is a list, so either check leaves it out. An unpacked export keeps `Takeout/` at the start of its paths, so the depth is the same there. A folder with more than one such file is not an album, as a candidate that is not the only one is not taken elsewhere.
   - The title is cleaned as a name typed in the interface is: spaces trimmed, control characters replaced by spaces, at most 200 characters. Titles are gathered from every export of the run first, so two exports with one title fill one album. The album is the owner's album whose name is the title, the oldest one if there are several, or a new one, made one title at a time in Go. The JSON's `description`, `access` and `date` are not used: albums have no description, and Google's sharing says nothing about who may see the album here, so it stays private to its owner.
   - Every media file of the album's folder whose outcome is `added`, `duplicate` or `in_trash`, with a photo of the owner that is not purged, is added, with nothing added twice. The scan form has a choice for photos in the owner's trash (settled with the user on 2026-11-15): leave them out of the album, which is the default, add them, or add them and restore them; only photos of albums are restored, and the run counts them. An album with nothing to add is made empty, as in Google Photos. Both an edited copy and its original are added until the versions step hides the original. A Live Photo video is added like any file; the album hides it as it hides every paired video.
   - The run counts the albums it filled, created or found, and the scan's summary shows the number.
   - Album titles are compared exactly, so "Trip" and "trip" are two albums. A photo the owner removed from an album comes back when the export is imported again, as the album is found by its title.
   - Checked on 2026-11-15 on the development stack with the user's six real exports, after one photo of an album was put in the owner's trash, with "add and restore": 4 albums, with 194, 160, 2 and 6 photos, the numbers of distinct files in their folders across all exports; the photo came back from the trash; the archived photo is in its album; year folders, the archive and the trash made no albums. The photo the owner moved to the trash in Google is in its album too, as older exports still had it there: albums are the union of every export. Step 6 moves such a photo to the trash, where the album hides it.
   - Tests: the matching of a small export with an album, the year folder and the list of memory titles; an album already in the library with the same title; a second run that adds nothing; a photo in the trash with each of the three choices.
5. **Versions.** A link from the original to the edited copy, the lists and counts that leave the hidden version out, the trash, albums and shares that move both, and the switch in the viewer.
6. **Descriptions, favorites, archive.** Schema, API and interface for each, filled from `description`, `favorited` and `archived` in the JSON. Items with `trashed` go to the trash, with the date of the import as the date of deletion.
7. **Tests.** A small Takeout export in `testdata`, made from the real names of step 0, with cut names, repeated names, an edited copy, an album, a photo in two parts and a restart in the middle of a `.tgz`, read as a folder, a `.zip` and a `.tgz`. Then a whole real export, recording here what it showed.

Steps 0 to 4 make a working import, released on their own. Versions come next, and descriptions, favorites and the archive each as a change of its own.

iCloud later: the parts of a copy from privacy.apple.com hold the files, `Photo Details.csv` (`imgName`, `fileChecksum`, `favorite`, `hidden`, `deleted`, `originalCreationDate`, `importDate`) and `Albums/*.csv`. Rows are matched to files by name, and by `fileChecksum` when a name repeats in a part. Live Photos are paired by `ContentIdentifier` as they are now.

## 11. Grid tile size

**Status:** done.

Before this improvement tiles were 200 CSS pixels in both grid modes, a constant in `lib/timeline.ts`. Rows that keep each photo's shape closed as soon as they overflowed the width, so a row was never taller than 200 and usually a little shorter, on a phone too. Squares aimed at 200, or 130 on a grid narrower than 600. The grid preview size on the administration page sets only the resolution of the preview files, which decides how sharp the tiles are, not their size.

Settled with the user on 2026-11-15:

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
