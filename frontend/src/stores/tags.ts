import { defineStore } from 'pinia'
import { shallowRef } from 'vue'

import { api, type Tag } from '../api/client'

export const useTagsStore = defineStore('tags', () => {
  const tags = shallowRef<Tag[]>([])

  async function load() {
    const { data } = await api.GET('/tags')
    if (data) {
      tags.value = data
    }
  }

  return { tags, load }
})
