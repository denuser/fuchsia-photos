import { defineStore } from 'pinia'
import { computed, shallowRef } from 'vue'

import { api, type UserRef } from '../api/client'
import { useAuthStore } from './auth'

export const useUsersStore = defineStore('users', () => {
  const auth = useAuthStore()

  // Active users, whom photos can be shared with.
  const active = shallowRef<UserRef[]>([])
  // An administrator also needs the logins of disabled users, who still own photos.
  const disabled = shallowRef<UserRef[]>([])

  const everyone = computed(() => [...active.value, ...disabled.value])
  const logins = computed(() => new Map(everyone.value.map((u) => [u.id, u.login])))
  // Whom the current user can share with.
  const recipients = computed(() => active.value.filter((u) => u.id !== auth.user?.id))

  async function load() {
    const { data } = await api.GET('/users/directory')
    active.value = data ?? []
    if (auth.isAdmin) {
      const { data: all } = await api.GET('/users')
      disabled.value = (all ?? []).filter((u) => u.disabled).map((u) => ({ id: u.id, login: u.login }))
    }
  }

  function login(id: number) {
    return logins.value.get(id) ?? null
  }

  return { everyone, recipients, load, login }
})
