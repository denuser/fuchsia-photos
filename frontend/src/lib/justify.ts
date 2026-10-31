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

// Lays items out in rows of equal height that fill the width. A row is closed as
// soon as it is wide enough at the target height. The last row keeps the target
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
    if (aspectSum * targetHeight + gaps >= width) {
      closeRow(i + 1, (width - gaps) / aspectSum)
    }
  }
  if (start < items.length) {
    closeRow(items.length, targetHeight)
  }

  return { rows, height: Math.max(0, y - gap) }
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
