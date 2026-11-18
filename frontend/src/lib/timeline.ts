import type { GridSize, Photo } from '../api/client'
import { intlLocale } from '../i18n'
import type { Section } from '../stores/photos'
import { justify, squareColumns, squares, type Row } from './justify'

// PatternFly's small spacer between photos.
export const GAP = 8

// On a wide grid each size aims at a number of CSS pixels, on any screen
// density: the height of a row of photos that keep their shape, and the side
// of a square. The largest tiles stay within what the largest grid preview
// covers on a 2x screen.
const WIDE_SIDES: Record<GridSize, number> = { small: 150, medium: 200, large: 280 }
// A grid narrower than a tablet fits a number of squares in a row instead,
// and rows of photos aim at the side of those squares.
const NARROW_COLUMNS: Record<GridSize, number> = { small: 4, medium: 3, large: 2 }
const NARROW_WIDTH = 600

// The sizes in the order the profile offers them.
export const gridSizes: GridSize[] = ['small', 'medium', 'large']

// The side a tile aims at for a grid this wide, in CSS pixels.
export function tileSide(width: number, size: GridSize) {
  if (width >= NARROW_WIDTH) {
    return WIDE_SIDES[size]
  }
  const columns = NARROW_COLUMNS[size]
  return Math.max(1, (width - GAP * (columns - 1)) / columns)
}
// The month title above the photos of a month.
export const HEADER_HEIGHT = 48
// The row that shows a list loading its next page, or failing to.
export const STATUS_HEIGHT = 64
// Photos are assumed to be this wide on average until they load.
const ASPECT_GUESS = 4 / 3

export interface SectionLayout {
  section: Section
  // The top of the section in the list, and its height.
  y: number
  height: number
  header: number
  // Rows of the loaded photos, with y relative to the top of the section.
  rows: Row[]
  // The part of the section that has not loaded yet: an estimate of the
  // photos it will hold, or a status row. Relative to the top of the section.
  pendingY: number
  pendingHeight: number
}

export interface TimelineLayout {
  sections: SectionLayout[]
  height: number
}

// Rows of a section are reused while its photos, the width and the tile size
// stay the same, so loading one month does not lay out all the others again.
const rowCache = new WeakMap<Photo[], { width: number; target: number; square: number | null; rows: Row[]; height: number }>()

function rowsOf(photos: Photo[], width: number, target: number, square: number | null) {
  const cached = rowCache.get(photos)
  if (cached && cached.width === width && cached.target === target && cached.square === square) {
    return cached
  }
  const { rows, height } = square
    ? squares(photos.length, width, square, GAP)
    : justify(
        photos.map((p) => ({ aspect: p.width && p.height ? p.width / p.height : 1 })),
        width,
        target,
        GAP,
      )
  const entry = { width, target, square, rows, height }
  rowCache.set(photos, entry)
  return entry
}

// The height of a row of photos, roughly when they are not square.
function rowHeight(width: number, target: number, square: number | null) {
  if (!square) {
    return target
  }
  const columns = squareColumns(width, square, GAP)
  return (width - GAP * (columns - 1)) / columns
}

// The height that n photos not loaded yet will take, roughly.
function estimate(n: number, width: number, target: number, square: number | null) {
  const perRow = square
    ? squareColumns(width, square, GAP)
    : Math.max(1, Math.floor((width + GAP) / (ASPECT_GUESS * target + GAP)))
  return Math.ceil(n / perRow) * (rowHeight(width, target, square) + GAP) - GAP
}

function pendingHeight(section: Section, width: number, target: number, square: number | null) {
  if (section.complete) {
    return 0
  }
  const missing = section.count - section.photos.length
  if (section.month && missing > 0) {
    return Math.max(STATUS_HEIGHT, estimate(missing, width, target, square))
  }
  return STATUS_HEIGHT
}

// Lays the sections out in rows that keep each photo's shape and aim at the
// target height, or, if square is set, in squares of about that many pixels.
export function layoutTimeline(
  sections: Section[],
  width: number,
  withHeaders: boolean,
  target: number,
  square: number | null = null,
): TimelineLayout {
  const out: SectionLayout[] = []
  if (width <= 0) {
    return { sections: out, height: 0 }
  }
  let y = 0
  for (const section of sections) {
    const header = withHeaders ? HEADER_HEIGHT : 0
    const rows = rowsOf(section.photos, width, target, square)
    const pending = pendingHeight(section, width, target, square)
    const pendingY = header + rows.height + (rows.height > 0 && pending > 0 ? GAP : 0)
    const height = pendingY + pending
    out.push({ section, y, height, header, rows: rows.rows, pendingY, pendingHeight: pending })
    y += height + GAP
  }
  return { sections: out, height: Math.max(0, y - GAP) }
}

// The sections that intersect [top, bottom], as an index range.
export function visibleSections(layout: TimelineLayout, top: number, bottom: number): [number, number] {
  const list = layout.sections
  let low = 0
  let high = list.length
  while (low < high) {
    const mid = (low + high) >> 1
    const s = list[mid]!
    if (s.y + s.height < top) {
      low = mid + 1
    } else {
      high = mid
    }
  }
  let end = low
  while (end < list.length && list[end]!.y <= bottom) {
    end++
  }
  return [low, end]
}

// Such as "November 2026" or "Ноябрь 2026 г.": Russian writes the month in lower case, which a title capitalizes.
export function monthTitle(section: Section) {
  const title = section.month?.start.toLocaleDateString(intlLocale.value, { month: 'long', year: 'numeric' }) ?? ''
  return title.charAt(0).toUpperCase() + title.slice(1)
}
