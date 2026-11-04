import type { Photo } from '../api/client'
import type { Section } from '../stores/photos'
import { justify, type Row } from './justify'

export const TARGET_HEIGHT = 200
export const GAP = 4
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

// Rows of a section are reused while its photos and the width stay the same,
// so loading one month does not lay out all the others again.
const rowCache = new WeakMap<Photo[], { width: number; rows: Row[]; height: number }>()

function rowsOf(photos: Photo[], width: number) {
  const cached = rowCache.get(photos)
  if (cached && cached.width === width) {
    return cached
  }
  const { rows, height } = justify(
    photos.map((p) => ({ aspect: p.width && p.height ? p.width / p.height : 1 })),
    width,
    TARGET_HEIGHT,
    GAP,
  )
  const entry = { width, rows, height }
  rowCache.set(photos, entry)
  return entry
}

// The height that n photos not loaded yet will take, roughly.
function estimate(n: number, width: number) {
  const perRow = Math.max(1, Math.floor((width + GAP) / (ASPECT_GUESS * TARGET_HEIGHT + GAP)))
  return Math.ceil(n / perRow) * (TARGET_HEIGHT + GAP) - GAP
}

function pendingHeight(section: Section, width: number) {
  if (section.complete) {
    return 0
  }
  const missing = section.count - section.photos.length
  if (section.month && missing > 0) {
    return Math.max(STATUS_HEIGHT, estimate(missing, width))
  }
  return STATUS_HEIGHT
}

export function layoutTimeline(sections: Section[], width: number, withHeaders: boolean): TimelineLayout {
  const out: SectionLayout[] = []
  if (width <= 0) {
    return { sections: out, height: 0 }
  }
  let y = 0
  for (const section of sections) {
    const header = withHeaders ? HEADER_HEIGHT : 0
    const rows = rowsOf(section.photos, width)
    const pending = pendingHeight(section, width)
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

export function monthTitle(section: Section) {
  return section.month?.start.toLocaleDateString(undefined, { month: 'long', year: 'numeric' }) ?? ''
}
