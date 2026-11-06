<script setup lang="ts">
import { onMounted, ref, shallowRef } from 'vue'

import { api, type Role, type User } from '../api/client'
import { useAuthStore } from '../stores/auth'

const auth = useAuthStore()

const users = shallowRef<User[]>([])
const error = ref('')

const roles: { title: string; value: Role }[] = [
  { title: 'User', value: 'user' },
  { title: 'Administrator', value: 'admin' },
]

const createOpen = ref(false)
const form = ref<{ login: string; password: string; role: Role }>({ login: '', password: '', role: 'user' })
const formError = ref('')

const passwordFor = shallowRef<User | null>(null)
const newPassword = ref('')
const passwordError = ref('')

onMounted(load)

async function load() {
  const { data } = await api.GET('/users')
  users.value = data ?? []
}

function openCreate() {
  form.value = { login: '', password: '', role: 'user' }
  formError.value = ''
  createOpen.value = true
}

async function create() {
  const { error: failure } = await api.POST('/users', { body: form.value })
  if (failure) {
    formError.value = failure.error
    return
  }
  createOpen.value = false
  await load()
}

async function update(user: User, body: { role?: Role; disabled?: boolean }) {
  const { error: failure } = await api.PATCH('/users/{id}', { params: { path: { id: user.id } }, body })
  error.value = failure?.error ?? ''
  await load()
}

function openPassword(user: User) {
  passwordFor.value = user
  newPassword.value = ''
  passwordError.value = ''
}

async function savePassword() {
  const { error: failure } = await api.PATCH('/users/{id}', {
    params: { path: { id: passwordFor.value!.id } },
    body: { password: newPassword.value },
  })
  if (failure) {
    passwordError.value = failure.error
    return
  }
  passwordFor.value = null
}
</script>

<template>
  <v-container class="users">
    <div class="d-flex align-center mb-4">
      <h1 class="text-h5">Users</h1>
      <v-spacer />
      <v-btn color="primary" prepend-icon="mdi-account-plus" text="Add user" @click="openCreate" />
    </div>

    <v-alert v-if="error" type="error" density="compact" class="mb-4" :text="error" />

    <v-table>
      <thead>
        <tr>
          <th>Login</th>
          <th>Role</th>
          <th>Active</th>
          <th aria-label="Actions" />
        </tr>
      </thead>
      <tbody>
        <tr v-for="user in users" :key="user.id">
          <td>{{ user.login }}</td>
          <td class="role">
            <v-select
              :model-value="user.role"
              :items="roles"
              :disabled="user.id === auth.user?.id"
              density="compact"
              variant="plain"
              hide-details
              :aria-label="`Role of ${user.login}`"
              @update:model-value="update(user, { role: $event })"
            />
          </td>
          <td>
            <v-switch
              :model-value="!user.disabled"
              :disabled="user.id === auth.user?.id"
              color="primary"
              density="compact"
              hide-details
              :aria-label="`${user.login} is active`"
              @update:model-value="update(user, { disabled: !$event })"
            />
          </td>
          <td class="text-right">
            <v-btn variant="text" size="small" text="Reset password" @click="openPassword(user)" />
          </td>
        </tr>
      </tbody>
    </v-table>

    <v-dialog v-model="createOpen" max-width="420">
      <v-card title="Add user">
        <v-form @submit.prevent="create">
          <v-card-text>
            <v-alert v-if="formError" type="error" density="compact" class="mb-4" :text="formError" />
            <v-text-field v-model="form.login" label="Login" autocomplete="off" />
            <v-text-field v-model="form.password" label="Password" type="password" autocomplete="new-password" />
            <v-select v-model="form.role" variant="outlined" :items="roles" label="Role" />
          </v-card-text>
          <v-card-actions>
            <v-spacer />
            <v-btn text="Cancel" @click="createOpen = false" />
            <v-btn type="submit" color="primary" text="Add user" />
          </v-card-actions>
        </v-form>
      </v-card>
    </v-dialog>

    <v-dialog :model-value="passwordFor !== null" max-width="420" @update:model-value="passwordFor = null">
      <v-card :title="`Reset password of ${passwordFor?.login}`">
        <v-form @submit.prevent="savePassword">
          <v-card-text>
            <v-alert v-if="passwordError" type="error" density="compact" class="mb-4" :text="passwordError" />
            <v-text-field
              v-model="newPassword"
              label="New password"
              type="password"
              autocomplete="new-password"
              hint="This user will be logged out everywhere."
              persistent-hint
            />
          </v-card-text>
          <v-card-actions>
            <v-spacer />
            <v-btn text="Cancel" @click="passwordFor = null" />
            <v-btn type="submit" color="primary" text="Reset password" />
          </v-card-actions>
        </v-form>
      </v-card>
    </v-dialog>
  </v-container>
</template>

<style scoped>
.users {
  height: 100%;
  overflow-y: auto;
  max-width: 900px;
}

.role {
  width: 200px;
}
</style>
