<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'

import { api, type AppPassword, type ViewerField } from '../api/client'
import AppFormGroup from '../components/AppFormGroup.vue'
import AppPageHeader from '../components/AppPageHeader.vue'
import AppPasswordDialog from '../components/AppPasswordDialog.vue'
import ChangePasswordDialog from '../components/ChangePasswordDialog.vue'
import ConfirmDialog from '../components/ConfirmDialog.vue'
import CopyField from '../components/CopyField.vue'
import { viewerFields } from '../lib/viewerFields'
import { useAuthStore } from '../stores/auth'
import { useNoticesStore } from '../stores/notices'

const auth = useAuthStore()

const passwordDialog = ref(false)
const saving = ref(false)
const error = ref('')

const shown = computed(() => viewerFields.map((f) => f.value).filter((f) => !auth.user?.viewerHiddenFields.includes(f)))

type Changes = { viewerShowsOriginals?: boolean; viewerHiddenFields?: ViewerField[]; navigationHiddenOnPhones?: boolean; gridSquares?: boolean }

// Each change is saved at once, like the switch on the "Shared with me" page.
async function save(body: Changes) {
  saving.value = true
  error.value = ''
  const { data, error: failure } = await api.PATCH('/me', { body })
  saving.value = false
  if (!data) {
    error.value = failure?.error ?? 'The setting could not be saved'
    return
  }
  auth.user = data
}

function setShown(value: ViewerField[] | null) {
  const visible = value ?? []
  void save({ viewerHiddenFields: viewerFields.map((f) => f.value).filter((f) => !visible.includes(f)) })
}

const notices = useNoticesStore()
const webdavAddress = `${window.location.origin}/webdav/`
const appPasswords = ref<AppPassword[]>([])
const appPasswordsLoaded = ref(false)
const appPasswordDialog = ref(false)
const revokeOpen = ref(false)
const revoking = ref<AppPassword>()

async function loadAppPasswords() {
  const { data } = await api.GET('/me/app-passwords')
  if (!data) {
    error.value = 'App passwords could not be loaded'
    return
  }
  appPasswords.value = data
  appPasswordsLoaded.value = true
}

function startRevoke(password: AppPassword) {
  revoking.value = password
  revokeOpen.value = true
}

async function revoke() {
  const password = revoking.value
  if (!password) {
    return
  }
  const { error: failure } = await api.DELETE('/me/app-passwords/{id}', { params: { path: { id: password.id } } })
  if (failure) {
    error.value = failure.error
    return
  }
  appPasswords.value = appPasswords.value.filter((p) => p.id !== password.id)
  notices.notify(`Revoked the app password for ${password.name}`)
}

function formatDate(value: string | null) {
  return value ? new Date(value).toLocaleDateString(undefined, { year: 'numeric', month: '2-digit', day: '2-digit' }) : 'Never'
}

onMounted(loadAppPasswords)
</script>

<template>
  <div v-if="auth.user" class="page">
    <AppPageHeader :title="auth.user.login" :description="auth.isAdmin ? 'Administrator' : 'User'" />
    <div class="page-section profile">

      <v-alert v-if="error" type="error" density="compact" class="mb-4" :title="error" />

      <h2 class="mb-2">Photo grid</h2>
      <v-switch
        :model-value="auth.user.gridSquares"
        :disabled="saving"
        label="Show square previews"
        hint="All previews are the same size, cropped to a square. The size follows the grid preview size an administrator sets."
        persistent-hint
        color="primary"
        density="compact"
        class="mb-6"
        @update:model-value="save({ gridSquares: Boolean($event) })"
      />

      <h2 class="mb-2">Viewer</h2>
      <v-switch
        :model-value="auth.user.viewerShowsOriginals"
        :disabled="saving"
        label="Load originals when zooming in"
        hint="Zooming in then shows the photo at full resolution, otherwise it enlarges the preview. Originals can be much larger. Photos the browser cannot show, such as HEIC outside Safari, keep their previews."
        persistent-hint
        color="primary"
        density="compact"
        class="mb-4"
        @update:model-value="save({ viewerShowsOriginals: Boolean($event) })"
      />

      <h3 class="mb-1">Photo details</h3>
      <div class="text-medium-emphasis mb-2">
        Shown under the photo name when the photo has them. A location is shown only if an administrator allows it.
      </div>
      <div class="fields mb-6">
        <v-checkbox
          v-for="field in viewerFields"
          :key="field.value"
          :model-value="shown"
          :value="field.value"
          :label="field.title"
          :disabled="saving"
          density="compact"
          hide-details
          @update:model-value="setShown"
        />
      </div>

      <h2 class="mb-2">Phone</h2>
      <v-switch
        :model-value="auth.user.navigationHiddenOnPhones"
        :disabled="saving"
        label="Hide the menu on a phone"
        hint="The menu button opens it over the page, so the photos take the whole width of the screen."
        persistent-hint
        color="primary"
        density="compact"
        class="mb-6"
        @update:model-value="save({ navigationHiddenOnPhones: Boolean($event) })"
      />

      <h2 class="mb-2">Password</h2>
      <v-btn prepend-icon="fa-key" variant="tonal" text="Change password" class="mb-6" @click="passwordDialog = true" />
      <ChangePasswordDialog v-model="passwordDialog" />

      <h2 class="mb-2">Upload from a phone</h2>
      <div class="text-medium-emphasis mb-4">
        Sync apps such as PhotoSync or FolderSync can upload photos over WebDAV. Set them up with this address, your login and an
        app password, one for each device. Deleting files in the app leaves the photos here, and photos you moved to the trash stay
        there when the app sends them again. Only JPEG, PNG, WebP and HEIC photos are accepted.
      </div>
      <div class="webdav-access mb-6">
        <AppFormGroup v-slot="field" label="Address" class="mb-0">
          <CopyField v-bind="field" :value="webdavAddress" label="address" />
        </AppFormGroup>
        <AppFormGroup v-slot="field" label="Login" class="mb-0">
          <CopyField v-bind="field" :value="auth.user.login" label="login" />
        </AppFormGroup>
      </div>

      <h3 class="mb-2">App passwords</h3>
      <v-table v-if="appPasswords.length > 0" class="mb-4">
        <thead>
          <tr>
            <th>Device</th>
            <th>Created</th>
            <th>Last used</th>
            <th />
          </tr>
        </thead>
        <tbody>
          <tr v-for="password in appPasswords" :key="password.id">
            <td>{{ password.name }}</td>
            <td>{{ formatDate(password.createdAt) }}</td>
            <td>{{ formatDate(password.lastUsedAt) }}</td>
            <td class="text-right">
              <v-btn icon="fa-trash-can" size="small" variant="text" :aria-label="`Revoke ${password.name}`" @click="startRevoke(password)" />
            </td>
          </tr>
        </tbody>
      </v-table>
      <div v-else-if="appPasswordsLoaded" class="text-medium-emphasis mb-4">No app passwords yet.</div>
      <v-btn prepend-icon="fa-plus" variant="tonal" text="New app password" @click="appPasswordDialog = true" />
      <AppPasswordDialog v-model="appPasswordDialog" @created="appPasswords.push($event)" />
      <ConfirmDialog
        v-model="revokeOpen"
        title="Revoke this app password?"
        :text="`${revoking?.name} will no longer be able to upload. Photos it uploaded stay in your library.`"
        action="Revoke"
        @confirm="revoke"
      />
    </div>
  </div>
</template>

<style scoped>
.profile {
  max-width: 768px;
}

.webdav-access {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(280px, 1fr));
  gap: 24px 16px;
}

.fields {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(220px, 1fr));
}
</style>
