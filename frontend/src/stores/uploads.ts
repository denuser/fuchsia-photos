import { defineStore } from 'pinia'
import { computed, ref, shallowRef, type InjectionKey } from 'vue'

import { API_BASE, notifyUnauthorized, type UploadResult } from '../api/client'
import { t } from '../i18n'
import { apiError } from '../i18n/apiErrors'
import { useNoticesStore } from './notices'
import { usePhotosStore } from './photos'

// Opens the upload dialog of the Upload button in the app bar. App provides it,
// so that a page can offer the same choice, as the empty timeline does.
export const pickUploadsKey: InjectionKey<() => void> = Symbol('pickUploads')

// The album files go into, or null for the library only.
export interface UploadAlbum {
  id: number
  name: string
}

const CONCURRENCY = 3
const MAX_ATTEMPTS = 3
const RETRY_DELAY_MS = 2000

interface Upload {
  file: File
  album: UploadAlbum | null
}

export interface FailedUpload extends Upload {
  error: string
}

// Failures worth another try, as opposed to a file the server will always refuse.
class RetryableError extends Error {}
class SessionEndedError extends Error {}
// The album of the upload was deleted, or is no longer the user's.
class AlbumGoneError extends Error {}

export const useUploadsStore = defineStore('uploads', () => {
  const total = ref(0)
  const done = ref(0)
  const duplicates = ref(0)
  const restored = ref(0)
  // The paths of files in chosen or dropped folders that are not photos or videos.
  const skipped = shallowRef<string[]>([])
  const failures = shallowRef<FailedUpload[]>([])
  const active = ref(0)
  // The album the uploads since the panel was last empty go into: null when
  // some go into the library only, or into different albums.
  const album = shallowRef<UploadAlbum | null>(null)
  // Changes each time a file lands in an album, so that the album page can update.
  const landed = shallowRef<{ album: number } | null>(null)

  // The upload dialog, and the album it opens with.
  const dialogOpen = ref(false)
  const dialogAlbum = shallowRef<UploadAlbum | null>(null)

  function openDialog(into: UploadAlbum | null = null) {
    dialogAlbum.value = into
    dialogOpen.value = true
  }

  const running = computed(() => active.value > 0)
  const visible = computed(() => total.value > 0)
  const progress = computed(() => (total.value ? ((done.value + failures.value.length) / total.value) * 100 : 0))

  let queue: Upload[] = []

  function add(files: File[], into: UploadAlbum | null = null, skippedFiles: string[] = []) {
    if (files.length === 0) {
      if (skippedFiles.length > 0) {
        useNoticesStore().notify(t('uploads.nothing'), 'warning')
      }
      return
    }
    if (!running.value) {
      total.value = 0
      done.value = 0
      duplicates.value = 0
      restored.value = 0
      skipped.value = []
      failures.value = []
      album.value = into
    } else if (album.value?.id !== into?.id) {
      album.value = null
    }
    total.value += files.length
    skipped.value = [...skipped.value, ...skippedFiles]
    queue.push(...files.map((file) => ({ file, album: into })))
    pump()
  }

  function pump() {
    while (active.value < CONCURRENCY && queue.length > 0) {
      active.value++
      void run(queue.shift()!).finally(() => {
        active.value--
        pump()
      })
    }
  }

  async function run(upload: Upload) {
    for (let attempt = 1; ; attempt++) {
      try {
        const result = await send(upload)
        usePhotosStore().merge([result.photo], upload.album?.id)
        if (upload.album) {
          landed.value = { album: upload.album.id }
        }
        if (result.restored) {
          restored.value++
        } else if (result.duplicate) {
          duplicates.value++
        }
        done.value++
        return
      } catch (error) {
        if (error instanceof SessionEndedError) {
          queue = []
          notifyUnauthorized()
          return
        }
        if (error instanceof AlbumGoneError) {
          // The files still waiting for that album would fail the same way.
          const gone = queue.filter((u) => u.album?.id === upload.album?.id)
          queue = queue.filter((u) => u.album?.id !== upload.album?.id)
          failures.value = [...failures.value, ...[upload, ...gone].map((u) => ({ ...u, error: error.message }))]
          return
        }
        if (!(error instanceof RetryableError) || attempt >= MAX_ATTEMPTS) {
          const message = error instanceof Error ? error.message : t('uploads.failed')
          failures.value = [...failures.value, { ...upload, error: message }]
          return
        }
        await new Promise((resolve) => setTimeout(resolve, RETRY_DELAY_MS * attempt))
      }
    }
  }

  async function send({ file, album }: Upload): Promise<UploadResult> {
    const body = new FormData()
    body.append('file', file)

    let response: Response
    try {
      const query = album ? `?album=${album.id}` : ''
      response = await fetch(`${API_BASE}/photos${query}`, { method: 'POST', body })
    } catch {
      throw new RetryableError(t('uploads.network'))
    }
    if (response.ok) {
      return (await response.json()) as UploadResult
    }
    if (response.status === 401) {
      throw new SessionEndedError()
    }
    if (response.status === 404 && album) {
      throw new AlbumGoneError(t('uploads.albumGone', { album: album.name }))
    }
    const message = await errorMessage(response)
    throw response.status >= 500 ? new RetryableError(message) : new Error(message)
  }

  async function errorMessage(response: Response) {
    try {
      return apiError(((await response.json()) as { error: string }).error)
    } catch {
      return t('uploads.status', { status: response.status })
    }
  }

  function retryFailed() {
    const failed = failures.value
    failures.value = []
    total.value -= failed.length
    // Files of different albums go back each with its own.
    const byAlbum = new Map<number | null, Upload[]>()
    for (const f of failed) {
      const key = f.album?.id ?? null
      byAlbum.set(key, [...(byAlbum.get(key) ?? []), f])
    }
    for (const group of byAlbum.values()) {
      add(
        group.map((u) => u.file),
        group[0]!.album,
      )
    }
  }

  function dismiss() {
    if (!running.value) {
      total.value = 0
      failures.value = []
    }
  }

  return {
    total,
    done,
    duplicates,
    restored,
    skipped,
    failures,
    running,
    visible,
    progress,
    album,
    landed,
    dialogOpen,
    dialogAlbum,
    openDialog,
    add,
    retryFailed,
    dismiss,
  }
})
