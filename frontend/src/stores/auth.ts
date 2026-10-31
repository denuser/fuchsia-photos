import { defineStore } from 'pinia'
import { computed, ref } from 'vue'

import { api, type User } from '../api/client'

export const useAuthStore = defineStore('auth', () => {
  const user = ref<User | null>(null)
  const loaded = ref(false)
  const isAdmin = computed(() => user.value?.role === 'admin')

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
