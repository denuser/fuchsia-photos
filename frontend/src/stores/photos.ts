import { defineStore } from 'pinia'
import { computed, ref, shallowRef } from 'vue'

import { API_BASE, api, type Photo, type PhotoEvent } from '../api/client'

// The server takes this many photos in one bulk action.
const BULK_SIZE = 500

export interface PhotoQuery {
  trash?: boolean
  album?: number
  tags?: number[]
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

export const usePhotosStore = defineStore('photos', () => {
  const photos = shallowRef<Photo[]>([])
  const query = shallowRef<PhotoQuery>({})
  const nextCursor = ref<string | null>(null)
  const loading = ref(false)
  const loadedOnce = ref(false)
  const pending = ref(0)
  const failed = ref(0)
  const hasMore = computed(() => nextCursor.value !== null)

  const order = computed(() => (query.value.trash ? deletedFirst : newestFirst))
  // Only the whole library and the trash can tell from a photo alone that it
  // belongs in the list. A filtered list learns about new members by loading.
  const unfiltered = computed(() => {
    const { album, tags, from, to } = query.value
    return album === undefined && !tags?.length && !from && !to
  })

  let events: EventSource | null = null
  // A list opened later makes the answers still on their way out of date.
  let generation = 0

  async function loadMore() {
    if (loading.value || (loadedOnce.value && !hasMore.value)) {
      return
    }
    const started = generation
    const cursor = nextCursor.value ?? undefined
    const { trash, album, tags, from, to } = query.value
    loading.value = true
    try {
      const { data } = trash
        ? await api.GET('/photos/trash', { params: { query: { cursor } } })
        : await api.GET('/photos', { params: { query: { cursor, album, tag: tags?.length ? tags : undefined, from, to } } })
      if (data && started === generation) {
        nextCursor.value = data.nextCursor
        loadedOnce.value = true
        insert(data.items)
      }
    } finally {
      if (started === generation) {
        loading.value = false
      }
    }
  }

  function insert(changed: Photo[]) {
    const byId = new Map(photos.value.map((p) => [p.id, p]))
    for (const photo of changed) {
      byId.set(photo.id, photo)
    }
    photos.value = [...byId.values()].sort(order.value)
  }

  // Applies changes from the event stream or an upload. A new photo older than
  // everything loaded so far is left for pagination to bring in, as the photos
  // between it and the loaded ones are not known yet.
  function merge(changed: Photo[]) {
    const last = photos.value.at(-1)
    const known = new Set(photos.value.map((p) => p.id))
    const inTrash = Boolean(query.value.trash)
    const left = new Set<number>()
    const wanted: Photo[] = []
    for (const photo of changed) {
      if (Boolean(photo.deletedAt) !== inTrash) {
        left.add(photo.id)
      } else if (known.has(photo.id)) {
        wanted.push(photo)
      } else if (unfiltered.value && (!hasMore.value || !last || order.value(photo, last) <= 0)) {
        wanted.push(photo)
      }
    }
    if (left.size > 0) {
      remove(left)
    }
    if (wanted.length > 0) {
      insert(wanted)
    }
  }

  // Takes photos off the current list, without changing them on the server.
  function remove(ids: Iterable<number>) {
    const gone = new Set(ids)
    if (photos.value.some((p) => gone.has(p.id))) {
      photos.value = photos.value.filter((p) => !gone.has(p.id))
    }
  }

  async function reload() {
    generation++
    loading.value = false
    photos.value = []
    nextCursor.value = null
    loadedOnce.value = false
    await loadMore()
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
    loading.value = false
    photos.value = []
    query.value = {}
    nextCursor.value = null
    loadedOnce.value = false
    pending.value = 0
    failed.value = 0
  }

  return {
    photos,
    query,
    hasMore,
    loading,
    loadedOnce,
    pending,
    failed,
    loadMore,
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
