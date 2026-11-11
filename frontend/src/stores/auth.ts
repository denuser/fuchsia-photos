import { defineStore } from 'pinia'
import { computed, ref, watch } from 'vue'

import { api, type User } from '../api/client'
import { setLanguage } from '../i18n'

export const useAuthStore = defineStore('auth', () => {
  const user = ref<User | null>(null)
  const loaded = ref(false)
  const isAdmin = computed(() => user.value?.role === 'admin')

  // The user's language applies as soon as the user is known, and the
  // browser's language before login and after logout.
  watch(() => user.value?.language, setLanguage, { immediate: true })

  async function load() {
    if (loaded.value) {
      return
    }
    const { data } = await api.GET('/me')
    user.value = data ?? null
    loaded.value = true
  }

  // Returns an error message, or null on success.
  async function login(login: string, password: string): Promise<string | null> {
    const { data, error } = await api.POST('/login', { body: { login, password } })
    if (error) {
      return error.error
    }
    user.value = data
    return null
  }

  async function logout() {
    await api.POST('/logout')
    user.value = null
  }

  function clear() {
    user.value = null
  }

  return { user, isAdmin, load, login, logout, clear }
})
