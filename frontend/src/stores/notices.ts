import { defineStore } from 'pinia'
import { ref } from 'vue'

// Short notices about finished actions, shown as toast alerts in the top right
// corner (components/AppToasts.vue). Others than errors close by themselves;
// an error stays until it is closed, so that it can be read.

export type NoticeType = 'success' | 'info' | 'warning' | 'error'

export interface Notice {
  id: number
  title: string
  type: NoticeType
}

const TIMEOUT = 8000
const MAX_SHOWN = 4

export const useNoticesStore = defineStore('notices', () => {
  const notices = ref<Notice[]>([])
  let next = 1

  function notify(title: string, type: NoticeType = 'success') {
    const id = next++
    notices.value = [...notices.value, { id, title, type }].slice(-MAX_SHOWN)
    if (type !== 'error') {
      setTimeout(() => dismiss(id), TIMEOUT)
    }
  }

  function dismiss(id: number) {
    notices.value = notices.value.filter((n) => n.id !== id)
  }

  return { notices, notify, dismiss }
})
