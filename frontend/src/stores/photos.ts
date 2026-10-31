import { defineStore } from 'pinia'
import { computed, ref, shallowRef } from 'vue'

import { API_BASE, api, type Photo, type PhotoEvent } from '../api/client'

function newestFirst(a: Photo, b: Photo) {
  return b.sortAt.localeCompare(a.sortAt) || b.id - a.id
}

export const usePhotosStore = defineStore('photos', () => {
  const photos = shallowRef<Photo[]>([])
  const nextCursor = ref<string | null>(null)
  const loading = ref(false)
  const loadedOnce = ref(false)
  const pending = ref(0)
  const failed = ref(0)
  const hasMore = computed(() => nextCursor.value !== null)

  let events: EventSource | null = null

  async function loadMore() {
    if (loading.value || (loadedOnce.value && !hasMore.value)) {
      return
    }
    loading.value = true
    try {
      const { data } = await api.GET('/photos', {
        params: { query: nextCursor.value ? { cursor: nextCursor.value } : {} },
      })
      if (data) {
        nextCursor.value = data.nextCursor
        loadedOnce.value = true
        insert(data.items)
      }
    } finally {
      loading.value = false
    }
  }

  function insert(changed: Photo[]) {
    const byId = new Map(photos.value.map((p) => [p.id, p]))
    for (const photo of changed) {
      byId.set(photo.id, photo)
    }
    photos.value = [...byId.values()].sort(newestFirst)
  }

  // Applies changes from the event stream or an upload. A new photo older than
  // everything loaded so far is left for pagination to bring in, as the photos
  // between it and the loaded ones are not known yet.
  function merge(changed: Photo[]) {
    const last = photos.value.at(-1)
    const known = new Set(photos.value.map((p) => p.id))
    const wanted = changed.filter(
      (p) => known.has(p.id) || !hasMore.value || !last || newestFirst(p, last) <= 0,
    )
    if (wanted.length > 0) {
      insert(wanted)
    }
  }

  async function reload() {
    photos.value = []
    nextCursor.value = null
    loadedOnce.value = false
    await loadMore()
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
    photos.value = []
    nextCursor.value = null
    loadedOnce.value = false
    pending.value = 0
    failed.value = 0
  }

  return { photos, hasMore, loading, loadedOnce, pending, failed, loadMore, merge, reload, connect, disconnect, reset }
})
