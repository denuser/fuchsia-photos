import { defineStore } from 'pinia'
import { computed, ref, shallowRef } from 'vue'

import { API_BASE, api, type Photo, type PhotoEvent } from '../api/client'

// The server takes this many photos in one bulk action.
const BULK_SIZE = 500

export interface PhotoQuery {
  trash?: boolean
  // Other users' photos shared with the current user.
  shared?: boolean
  // All users' photos, for an administrator, optionally of one owner.
  everyone?: boolean
  owner?: number
  album?: number
  // The user's photos together with other users' photos the user can see, in the timeline.
  withShared?: boolean
  tags?: number[]
  cameras?: string[]
  lenses?: string[]
  from?: string
  to?: string
}

function newestFirst(a: Photo, b: Photo) {
  return b.sortAt.localeCompare(a.sortAt) || b.id - a.id
}

function deletedFirst(a: Photo, b: Photo) {
  return (b.deletedAt ?? '').localeCompare(a.deletedAt ?? '') || b.id - a.id
}

// Runs a bulk request on as many batches as the selection needs.
export async function inBatches(ids: number[], send: (batch: number[]) => Promise<{ error?: { error: string } }>) {
  for (let i = 0; i < ids.length; i += BULK_SIZE) {
    const { error } = await send(ids.slice(i, i + BULK_SIZE))
    if (error) {
      throw new Error(error.error)
    }
  }
}

// A part of a list: a month of the timeline, or the whole list where photos
// are not grouped by month, as in the trash. A month starts as a count from the
// server and gets its photos once it comes near the screen.
export interface Section {
  key: string
  // The month the section covers, or null for a whole list.
  month: { start: Date; end: Date } | null
  // The number of photos in the section, including those not loaded yet. A
  // whole list does not know it, and counts its loaded photos.
  count: number
  // Loaded photos, newest first. A section loads page by page.
  photos: Photo[]
  cursor: string | null
  complete: boolean
  loading: boolean
  failed: boolean
}

// The year and month of a moment in the browser's time zone, as the server names months.
export function monthKey(at: string | Date) {
  const date = new Date(at)
  return `${date.getFullYear()}-${String(date.getMonth() + 1).padStart(2, '0')}`
}

function monthSection(key: string, count: number): Section {
  const [year, month] = key.split('-').map(Number) as [number, number]
  return {
    key,
    month: { start: new Date(year, month - 1, 1), end: new Date(year, month, 1) },
    count,
    photos: [],
    cursor: null,
    complete: count === 0,
    loading: false,
    failed: false,
  }
}

const WHOLE_LIST = 'all'

function wholeList(): Section {
  return { key: WHOLE_LIST, month: null, count: 0, photos: [], cursor: null, complete: false, loading: false, failed: false }
}

// The browser's time zone, which the server counts months in.
function timeZone() {
  return Intl.DateTimeFormat().resolvedOptions().timeZone || 'UTC'
}

// Months load at most this many at a time, nearest to the screen first.
const PARALLEL_LOADS = 3
// A month loads in pages of this size, the most the server gives.
const MONTH_PAGE = 500

function later(a: string | undefined, b: string | undefined) {
  return !a ? b : !b ? a : a > b ? a : b
}

function earlier(a: string | undefined, b: string | undefined) {
  return !a ? b : !b ? a : a < b ? a : b
}

function fetchPage(query: PhotoQuery, section: Section) {
  const cursor = section.cursor ?? undefined
  const { trash, shared, everyone, owner, album } = query
  if (trash) {
    return api.GET('/photos/trash', { params: { query: { cursor } } })
  }
  // A month narrows the filters of the list to its own dates.
  const from = later(query.from, section.month?.start.toISOString())
  const to = earlier(query.to, section.month?.end.toISOString())
  const limit = section.month ? MONTH_PAGE : undefined
  if (shared) {
    return api.GET('/photos/shared', { params: { query: { cursor, limit, from, to } } })
  }
  if (everyone) {
    return api.GET('/photos/all', { params: { query: { cursor, limit, owner, from, to } } })
  }
  return api.GET('/photos', { params: { query: { cursor, limit, album, ...listFilters(query), from, to } } })
}

function fetchMonths(query: PhotoQuery) {
  const { shared, everyone, owner, album, from, to } = query
  const tz = timeZone()
  if (shared) {
    return api.GET('/photos/shared/months', { params: { query: { tz } } })
  }
  if (everyone) {
    return api.GET('/photos/all/months', { params: { query: { tz, owner } } })
  }
  return api.GET('/photos/months', { params: { query: { tz, album, ...listFilters(query), from, to } } })
}

// The filters of the user's own list, other than the album and the dates. An empty one is left out.
export function listFilters({ withShared, tags, cameras, lenses }: PhotoQuery) {
  return {
    withShared: withShared || undefined,
    tag: tags?.length ? tags : undefined,
    camera: cameras?.length ? cameras : undefined,
    lens: lenses?.length ? lenses : undefined,
  }
}

export const usePhotosStore = defineStore('photos', () => {
  const sections = shallowRef<Section[]>([])
  const query = shallowRef<PhotoQuery>({})
  const loadedOnce = ref(false)
  // The list itself, or its months, could not be loaded.
  const failedToOpen = ref(false)
  const pending = ref(0)
  const failed = ref(0)

  // The trash is ordered by when photos were deleted, so it is not split into months.
  const byMonth = computed(() => !query.value.trash)
  // All loaded photos in order. Months not loaded yet leave gaps.
  const photos = computed(() => sections.value.flatMap((s) => s.photos))
  const empty = computed(() => loadedOnce.value && sections.value.every((s) => s.complete && s.photos.length === 0))

  const order = computed(() => (query.value.trash ? deletedFirst : newestFirst))
  // Only the whole library and the trash can tell from a photo alone that it
  // belongs in the list. Other lists learn about new members by loading.
  const unfiltered = computed(() => {
    const { shared, everyone, album, tags, cameras, lenses, from, to } = query.value
    return !shared && !everyone && album === undefined && !tags?.length && !cameras?.length && !lenses?.length && !from && !to
  })

  let events: EventSource | null = null
  // A list opened later makes the answers still on their way out of date.
  let generation = 0
  // Sections the grid wants, nearest to the screen first.
  let wanted: string[] = []
  let active = 0

  function update(key: string, change: (section: Section) => Section | null) {
    const next: Section[] = []
    for (const section of sections.value) {
      if (section.key !== key) {
        next.push(section)
        continue
      }
      const changed = change(section)
      if (changed) {
        next.push(changed)
      }
    }
    sections.value = next
  }

  function find(key: string) {
    return sections.value.find((s) => s.key === key)
  }

  // Loads the rest of a month, or the next page of a whole list.
  async function loadSection(key: string) {
    const section = find(key)
    if (!section || section.loading || section.complete || section.failed) {
      return
    }
    const started = generation
    active++
    update(key, (s) => ({ ...s, loading: true }))
    try {
      for (;;) {
        const current = find(key)
        if (!current || started !== generation) {
          return
        }
        const { data } = await fetchPage(query.value, current)
        if (started !== generation) {
          return
        }
        if (!data) {
          update(key, (s) => ({ ...s, loading: false, failed: true }))
          return
        }
        let done = false
        update(key, (s) => {
          const photos = sortedUnion(s.photos, data.items)
          done = data.nextCursor === null
          // A finished month knows its count exactly.
          const count = s.month ? (done ? photos.length : Math.max(s.count, photos.length)) : photos.length
          return { ...s, photos, count, cursor: data.nextCursor, complete: done, loading: done ? false : s.loading }
        })
        if (done || !byMonth.value) {
          update(key, (s) => ({ ...s, loading: false }))
          return
        }
      }
    } finally {
      if (started === generation) {
        active--
        pump()
      }
    }
  }

  function sortedUnion(loaded: Photo[], added: Photo[]) {
    const byId = new Map(loaded.map((p) => [p.id, p]))
    for (const photo of added) {
      byId.set(photo.id, photo)
    }
    return [...byId.values()].sort(order.value)
  }

  function pump() {
    for (const key of wanted) {
      if (active >= PARALLEL_LOADS) {
        return
      }
      const section = find(key)
      if (section && !section.loading && !section.complete && !section.failed) {
        void loadSection(key)
      }
    }
  }

  // The grid names the sections near the screen, nearest first. Sections it
  // stops asking for are not started; those already loading finish.
  function want(keys: string[]) {
    wanted = keys
    pump()
  }

  function retry(key: string) {
    update(key, (s) => ({ ...s, failed: false }))
    void loadSection(key)
  }

  // Loads what the viewer needs next to a photo: the rest of its month and the
  // months on both sides, or the next page of a whole list near its end.
  function loadAround(photo: Photo) {
    const list = sections.value
    const i = list.findIndex((s) => s.photos.some((p) => p.id === photo.id))
    if (i < 0) {
      return
    }
    const keys = [list[i]!, list[i + 1], list[i - 1]].filter((s): s is Section => Boolean(s)).map((s) => s.key)
    if (!byMonth.value) {
      const at = list[i]!.photos.findIndex((p) => p.id === photo.id)
      if (at < list[i]!.photos.length - 10) {
        return
      }
    }
    for (const key of keys) {
      void loadSection(key)
    }
  }

  // The photos around one, without gaps: the viewer steps through these, so
  // that it does not skip a month that has not loaded yet.
  function around(photo: Photo): Photo[] {
    const list = sections.value
    const i = list.findIndex((s) => s.photos.some((p) => p.id === photo.id))
    if (i < 0) {
      return [photo]
    }
    let first = i
    while (first > 0 && list[first - 1]!.complete) {
      first--
    }
    let last = i
    while (list[last]!.complete && last + 1 < list.length) {
      last++
    }
    return list.slice(first, last + 1).flatMap((s) => s.photos)
  }

  // Applies changes from the event stream or an upload. Events also come for
  // changes to old photos, so a photo the list has not loaded may already be
  // counted in its month. It joins the list only where it is certainly new: in
  // a loaded part of a month, in a month the list does not have, or within the
  // loaded part of a whole list. Other counts are corrected when months load.
  function merge(changed: Photo[]) {
    const inTrash = Boolean(query.value.trash)
    const known = new Set(photos.value.map((p) => p.id))
    const gone = new Set<number>()
    const arriving: Photo[] = []
    for (const photo of changed) {
      if (Boolean(photo.deletedAt) !== inTrash) {
        gone.add(photo.id)
      } else if (known.has(photo.id)) {
        // It may have moved to another month once its EXIF was read.
        gone.add(photo.id)
        arriving.push(photo)
      } else if (unfiltered.value) {
        arriving.push(photo)
      }
    }
    remove(gone)
    for (const photo of arriving) {
      place(photo)
    }
  }

  function place(photo: Photo) {
    if (!byMonth.value) {
      const [list] = sections.value
      const last = list?.photos.at(-1)
      if (list && (list.complete || !last || order.value(photo, last) <= 0)) {
        update(list.key, (s) => ({ ...s, photos: sortedUnion(s.photos, [photo]), count: s.count + 1 }))
      }
      return
    }
    const key = monthKey(photo.sortAt)
    const section = find(key)
    if (!section) {
      const created = { ...monthSection(key, 1), photos: [photo], complete: true }
      sections.value = [...sections.value, created].sort((a, b) => b.key.localeCompare(a.key))
      return
    }
    const last = section.photos.at(-1)
    if (section.complete || (last !== undefined && order.value(photo, last) <= 0)) {
      update(key, (s) => ({ ...s, photos: sortedUnion(s.photos, [photo]), count: s.count + 1 }))
    }
  }

  // Takes photos off the current list, without changing them on the server.
  // A month left without photos goes.
  function remove(ids: Iterable<number>) {
    const gone = new Set(ids)
    if (gone.size === 0 || !photos.value.some((p) => gone.has(p.id))) {
      return
    }
    sections.value = sections.value.flatMap((s) => {
      const photos = s.photos.filter((p) => !gone.has(p.id))
      const removed = s.photos.length - photos.length
      if (removed === 0) {
        return [s]
      }
      const count = Math.max(photos.length, s.count - removed)
      return s.month && count === 0 ? [] : [{ ...s, photos, count }]
    })
  }

  async function reload() {
    generation++
    const started = generation
    active = 0
    wanted = []
    sections.value = []
    loadedOnce.value = false
    failedToOpen.value = false
    if (!byMonth.value) {
      sections.value = [wholeList()]
      await loadSection(WHOLE_LIST)
      if (started === generation) {
        loadedOnce.value = true
        failedToOpen.value = Boolean(find(WHOLE_LIST)?.failed)
      }
      return
    }
    const { data } = await fetchMonths(query.value)
    if (started !== generation) {
      return
    }
    if (!data) {
      failedToOpen.value = true
      return
    }
    sections.value = data.map((m) => monthSection(m.month, m.count))
    loadedOnce.value = true
    pump()
  }

  async function open(next: PhotoQuery) {
    query.value = next
    await reload()
  }

  async function trash(ids: number[]) {
    await inBatches(ids, (batch) => api.POST('/photos/trash', { body: { ids: batch } }))
    remove(ids)
  }

  async function restore(ids: number[]) {
    await inBatches(ids, (batch) => api.POST('/photos/restore', { body: { ids: batch } }))
    remove(ids)
  }

  async function purge(ids: number[]) {
    await inBatches(ids, (batch) => api.POST('/photos/purge', { body: { ids: batch } }))
    remove(ids)
  }

  function connect() {
    if (events) {
      return
    }
    events = new EventSource(`${API_BASE}/photos/events`)
    events.addEventListener('photos', (message) => {
      const event = JSON.parse(message.data) as PhotoEvent
      pending.value = event.pending
      failed.value = event.failed
      merge(event.photos)
    })
  }

  function disconnect() {
    events?.close()
    events = null
  }

  function reset() {
    disconnect()
    generation++
    active = 0
    wanted = []
    sections.value = []
    query.value = {}
    loadedOnce.value = false
    failedToOpen.value = false
    pending.value = 0
    failed.value = 0
  }

  return {
    sections,
    photos,
    query,
    byMonth,
    loadedOnce,
    failedToOpen,
    empty,
    pending,
    failed,
    want,
    retry,
    loadAround,
    around,
    merge,
    remove,
    reload,
    open,
    trash,
    restore,
    purge,
    connect,
    disconnect,
    reset,
  }
})
