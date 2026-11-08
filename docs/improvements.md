# Improvements

The current list of improvements, in order of priority. Stage 5 of the [roadmap](../README.md#roadmap) is on hold while this list is being worked on.

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

The timeline loads month by month from counts the server gives first, with month titles and a scrubber with the years. A month near the screen loads all of its pages; months far from it are only counted. The trash keeps loading page by page, with a spinner at the end of the list and a retry button when a page fails. Decisions are in the [README](../README.md#decisions).

Loaded months stay in memory. Unloading the months far from the screen is left for when a library makes it necessary.

## 3. Camera and lens filters

**Status:** done.

The timeline filters by camera and by lens, next to the tags and dates. On a phone they sit behind one button with the dates. The counts in each list take the other filters into account. Decisions are in the [README](../README.md#decisions).

The lists offer the photos the timeline shows, including other users' photos when it shows them. Other pages, such as albums and "Shared with me", have no such filters yet.

## 4. Shared photos in the timeline

**Status:** done.

A user who had uploaded nothing saw an empty timeline, even with photos and albums shared with them. The timeline now also shows the other users' photos that the user can see, with a "Mine / All" switch that is saved for the user and on by default. Decisions are in the [README](../README.md#decisions).

## 5. PatternFly look

**Status:** in progress.

The interface follows [PatternFly 6](https://www.patternfly.org/) on top of Vuetify, with fuchsia in place of the PatternFly blue. Decisions are in the [README](../README.md#decisions).

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

Left, in order of priority:

1. **Empty states.** The PatternFly layout: a large icon, a title, the text and a primary action button.
2. **Focus rings.** A 2px accent outline on keyboard focus, which PatternFly draws on every control. Vuetify shows an overlay instead.
3. **Icons.** PatternFly uses its own icons and Font Awesome; the application uses Material Design Icons. Changing them is a new dependency and touches every page, so it comes last.

A second visual style like PatternFly's Project Felt is out of scope for now.
