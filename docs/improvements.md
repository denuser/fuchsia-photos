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

Left, in order of priority:

1. **Form fields.** PatternFly puts the label above the field, with helper text below and a mark on required fields. Vuetify floats the label inside the outline. A wrapper like `AppAutocomplete`, or Vuetify defaults with a separate label, would cover the text fields, selects and the date filters.
2. **Dialog buttons.** A PatternFly modal ends with a filled primary button and a link-style cancel button on the left. The dialogs now have two text buttons on the right.
3. **Text size.** PatternFly body text is 14px and headings follow its own scale (h1 1.5rem, h2 1.25rem, h3 1.125rem). Vuetify text is 16px, and pages use `text-h5` and `text-h6`.
4. **Alerts and notifications.** PatternFly inline alerts have a status icon, a title and a colored edge, and short notices appear as toast alerts in the top right corner. The application uses tonal Vuetify alerts and a snackbar at the bottom.
5. **Applied filters.** PatternFly shows the chosen filters as chip groups under the toolbar, with "Clear all filters". The timeline keeps them inside the fields. The [PatternFly filter guidelines](https://www.patternfly.org/patterns/filters/design-guidelines/) are the model.
6. **Page headers.** A title, a short description and, where it helps, breadcrumbs, such as "Albums / Trip" in an album, in a padded header section.
7. **Tables and tabs.** The users, tags and import tables with PatternFly row borders, header style and compact density. Tabs with the PatternFly underline on the "Shared with me" page.
8. **Empty states.** The PatternFly layout: a large icon, a title, the text and a primary action button.
9. **Focus rings.** A 2px accent outline on keyboard focus, which PatternFly draws on every control. Vuetify shows an overlay instead.
10. **Icons.** PatternFly uses its own icons and Font Awesome; the application uses Material Design Icons. Changing them is a new dependency and touches every page, so it comes last.
11. **Cyrillic text.** The Red Hat fonts have no Cyrillic, so dates in Russian fall back to Helvetica or Arial. PatternFly falls back to Noto Sans for other scripts. Bundling Noto Sans Cyrillic would match it.

A second visual style like PatternFly's Project Felt is out of scope for now.
