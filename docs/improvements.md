# Improvements

The current list of improvements, in order of priority. All of them are done except 6, 8 and 9, which are planned. Stages 0 to 6 of the [roadmap](roadmap.md) are done too.

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
