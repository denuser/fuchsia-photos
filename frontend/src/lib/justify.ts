export interface JustifyItem {
  aspect: number
}

export interface Tile {
  index: number
  x: number
  width: number
}

export interface Row {
  y: number
  height: number
  tiles: Tile[]
}

export interface Layout {
  rows: Row[]
  height: number
}

// A row that ends before the photo that would overflow it grows taller than
// the target, at most this many times, so that a panorama after a single
// photo does not leave that photo a huge row of its own.
const MAX_STRETCH = 1.4

// Lays items out in rows of equal height that fill the width. When an item
// overflows a row at the target height, the row ends with it or before it,
// whichever brings the row's height closer to the target by ratio, so rows are
// a little shorter or taller than the target. The last row keeps the target
// height instead of stretching a few photos across the whole width.
export function justify(items: JustifyItem[], width: number, targetHeight: number, gap: number): Layout {
  const rows: Row[] = []
  if (width <= 0) {
    return { rows, height: 0 }
  }

  let y = 0
  let start = 0
  let aspectSum = 0

  const closeRow = (end: number, height: number) => {
    const tiles: Tile[] = []
    let x = 0
    for (let index = start; index < end; index++) {
      const tileWidth = items[index]!.aspect * height
      tiles.push({ index, x, width: tileWidth })
      x += tileWidth + gap
    }
    rows.push({ y, height, tiles })
    y += height + gap
    start = end
    aspectSum = 0
  }

  for (let i = 0; i < items.length; i++) {
    aspectSum += items[i]!.aspect
    const gaps = gap * (i - start)
    if (aspectSum * targetHeight + gaps < width) {
      continue
    }
    const withIt = (width - gaps) / aspectSum
    if (i > start) {
      const without = (width - gaps + gap) / (aspectSum - items[i]!.aspect)
      if (without <= targetHeight * MAX_STRETCH && without / targetHeight < targetHeight / withIt) {
        closeRow(i, without)
        // The item starts the next row, and may overflow it alone.
        i--
        continue
      }
    }
    closeRow(i + 1, withIt)
  }
  if (start < items.length) {
    closeRow(items.length, targetHeight)
  }

  return { rows, height: Math.max(0, y - gap) }
}

// The number of squares in a row: the whole number that brings them closest
// to side when they fill the width.
export function squareColumns(width: number, side: number, gap: number) {
  return Math.max(1, Math.round((width + gap) / (side + gap)))
}

// Lays count items out as squares in rows that fill the width. The squares are
// about side wide, a little wider or narrower so that a whole number of them
// fills a row.
export function squares(count: number, width: number, side: number, gap: number): Layout {
  const rows: Row[] = []
  if (width <= 0) {
    return { rows, height: 0 }
  }
  const columns = squareColumns(width, side, gap)
  const size = (width - gap * (columns - 1)) / columns
  for (let start = 0; start < count; start += columns) {
    const tiles: Tile[] = []
    for (let index = start; index < Math.min(count, start + columns); index++) {
      tiles.push({ index, x: (index - start) * (size + gap), width: size })
    }
    rows.push({ y: rows.length * (size + gap), height: size, tiles })
  }
  return { rows, height: Math.max(0, rows.length * (size + gap) - gap) }
}

// Returns the index range of the rows that intersect [top, bottom].
export function visibleRows(rows: Row[], top: number, bottom: number): [number, number] {
  let low = 0
  let high = rows.length
  while (low < high) {
    const mid = (low + high) >> 1
    const row = rows[mid]!
    if (row.y + row.height < top) {
      low = mid + 1
    } else {
      high = mid
    }
  }
  let end = low
  while (end < rows.length && rows[end]!.y <= bottom) {
    end++
  }
  return [low, end]
}
