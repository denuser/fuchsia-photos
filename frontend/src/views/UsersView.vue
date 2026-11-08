<script setup lang="ts">
import { onMounted, ref, shallowRef } from 'vue'

import { api, type Role, type User } from '../api/client'
import { useAuthStore } from '../stores/auth'
import AppDialog from '../components/AppDialog.vue'
import AppFormGroup from '../components/AppFormGroup.vue'
import AppPageHeader from '../components/AppPageHeader.vue'

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
  <div class="page">
    <AppPageHeader title="Users">
      <template #actions>
        <v-btn color="primary" prepend-icon="mdi-account-plus" text="Add user" @click="openCreate" />
      </template>
    </AppPageHeader>
    <div class="page-section users">

      <v-alert v-if="error" type="error" density="compact" class="mb-4" :title="error" />

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

      <AppDialog v-model="createOpen" title="Add user" action="Add user" :error="formError" @submit="create">
        <AppFormGroup v-slot="field" label="Login" required>
          <v-text-field v-bind="field" v-model="form.login" autocomplete="off" />
        </AppFormGroup>
        <AppFormGroup v-slot="field" label="Password" required>
          <v-text-field v-bind="field" v-model="form.password" type="password" autocomplete="new-password" />
        </AppFormGroup>
        <AppFormGroup v-slot="field" label="Role" required>
          <v-select v-bind="field" v-model="form.role" variant="outlined" :items="roles" />
        </AppFormGroup>
      </AppDialog>

      <AppDialog
        :model-value="passwordFor !== null"
        :title="`Reset password of ${passwordFor?.login}`"
        action="Reset password"
        :error="passwordError"
        @update:model-value="passwordFor = null"
        @submit="savePassword"
      >
        <AppFormGroup v-slot="field" label="New password" required>
          <v-text-field
            v-bind="field"
            v-model="newPassword"
            type="password"
            autocomplete="new-password"
            hint="This user will be logged out everywhere."
          />
        </AppFormGroup>
      </AppDialog>
    </div>
  </div>
</template>

<style scoped>
.users {
  max-width: 948px;
}

.role {
  width: 200px;
}

/* A plain field keeps room for a label above its text, which this one does
   not have, so its value would sit lower than the rest of the row. */
.role :deep(.v-field__input) {
  padding-top: 4px;
  padding-bottom: 4px;
}
</style>
