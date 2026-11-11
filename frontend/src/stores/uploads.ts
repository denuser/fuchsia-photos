import { defineStore } from 'pinia'
import { computed, ref, shallowRef, type InjectionKey } from 'vue'

import { API_BASE, notifyUnauthorized, type UploadResult } from '../api/client'
import { t } from '../i18n'
import { apiError } from '../i18n/apiErrors'
import { usePhotosStore } from './photos'

// Opens the file picker of the Upload button in the app bar. App provides it,
// so that a page can offer the same choice, as the empty timeline does.
export const pickUploadsKey: InjectionKey<() => void> = Symbol('pickUploads')

const CONCURRENCY = 3
const MAX_ATTEMPTS = 3
const RETRY_DELAY_MS = 2000

export interface FailedUpload {
  file: File
  error: string
}

// Failures worth another try, as opposed to a file the server will always refuse.
class RetryableError extends Error {}
class SessionEndedError extends Error {}

export const useUploadsStore = defineStore('uploads', () => {
  const total = ref(0)
  const done = ref(0)
  const duplicates = ref(0)
  const restored = ref(0)
  const failures = shallowRef<FailedUpload[]>([])
  const active = ref(0)

  const running = computed(() => active.value > 0)
  const visible = computed(() => total.value > 0)
  const progress = computed(() => (total.value ? ((done.value + failures.value.length) / total.value) * 100 : 0))

  let queue: File[] = []

  function add(files: File[]) {
    if (!running.value) {
      total.value = 0
      done.value = 0
      duplicates.value = 0
      restored.value = 0
      failures.value = []
    }
    total.value += files.length
    queue.push(...files)
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

  async function run(file: File) {
    for (let attempt = 1; ; attempt++) {
      try {
        const result = await send(file)
        usePhotosStore().merge([result.photo])
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
        if (!(error instanceof RetryableError) || attempt >= MAX_ATTEMPTS) {
          const message = error instanceof Error ? error.message : t('uploads.failed')
          failures.value = [...failures.value, { file, error: message }]
          return
        }
        await new Promise((resolve) => setTimeout(resolve, RETRY_DELAY_MS * attempt))
      }
    }
  }

  async function send(file: File): Promise<UploadResult> {
    const body = new FormData()
    body.append('file', file)

    let response: Response
    try {
      response = await fetch(`${API_BASE}/photos`, { method: 'POST', body })
    } catch {
      throw new RetryableError(t('uploads.network'))
    }
    if (response.ok) {
      return (await response.json()) as UploadResult
    }
    if (response.status === 401) {
      throw new SessionEndedError()
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
    const files = failures.value.map((f) => f.file)
    failures.value = []
    total.value -= files.length
    add(files)
  }

  function dismiss() {
    if (!running.value) {
      total.value = 0
      failures.value = []
    }
  }

  return { total, done, duplicates, restored, failures, running, visible, progress, add, retryFailed, dismiss }
})
