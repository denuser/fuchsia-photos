<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'

import { api, type AppPassword, type Language, type ViewerField, type WebDAVAlbum } from '../api/client'
import AppFormGroup from '../components/AppFormGroup.vue'
import AppPageHeader from '../components/AppPageHeader.vue'
import AppPasswordDialog from '../components/AppPasswordDialog.vue'
import ChangePasswordDialog from '../components/ChangePasswordDialog.vue'
import ConfirmDialog from '../components/ConfirmDialog.vue'
import CopyField from '../components/CopyField.vue'
import FolderAlbumDialog from '../components/FolderAlbumDialog.vue'
import { formatDate, languages, t } from '../i18n'
import { apiError } from '../i18n/apiErrors'
import { viewerFields } from '../lib/viewerFields'
import { useAuthStore } from '../stores/auth'
import { useNoticesStore } from '../stores/notices'

const auth = useAuthStore()

const passwordDialog = ref(false)
const saving = ref(false)
const error = ref('')

const shown = computed(() => viewerFields.filter((f) => !auth.user?.viewerHiddenFields.includes(f)))

// Each language is named in itself, so that a user who cannot read the current one finds theirs.
const languageChoices = computed(() => [
  { value: 'auto', title: t('profile.languageAuto') },
  ...languages.map((value) => ({ value, title: t('profile.languageName', {}, { locale: value }) })),
])

type Changes = {
  viewerShowsOriginals?: boolean
  viewerHiddenFields?: ViewerField[]
  navigationHiddenOnPhones?: boolean
  gridSquares?: boolean
  language?: Language
}

// Each change is saved at once, like the switch on the "Shared with me" page.
async function save(body: Changes) {
  saving.value = true
  error.value = ''
  const { data, error: failure } = await api.PATCH('/me', { body })
  saving.value = false
  if (!data) {
    error.value = apiError(failure, t('profile.saveFailed'))
    return
  }
  auth.user = data
}

function setShown(value: ViewerField[] | null) {
  const visible = value ?? []
  void save({ viewerHiddenFields: viewerFields.filter((f) => !visible.includes(f)) })
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
    error.value = t('profile.appPasswordsFailed')
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
    error.value = apiError(failure)
    return
  }
  appPasswords.value = appPasswords.value.filter((p) => p.id !== password.id)
  notices.notify(t('profile.revoked', { name: password.name }))
}

const folderAlbums = ref<WebDAVAlbum[]>([])
const folderAlbumsLoaded = ref(false)
const folderAlbumDialog = ref(false)

async function loadFolderAlbums() {
  const { data } = await api.GET('/me/webdav-albums')
  if (!data) {
    error.value = t('profile.folderAlbumsFailed')
    return
  }
  folderAlbums.value = data
  folderAlbumsLoaded.value = true
}

function linked(link: WebDAVAlbum) {
  folderAlbums.value = [...folderAlbums.value, link].sort((a, b) => (a.path < b.path ? -1 : a.path > b.path ? 1 : 0))
  notices.notify(t('profile.linked', { path: link.path, album: link.albumName }))
}

async function unlink(link: WebDAVAlbum) {
  const { error: failure } = await api.DELETE('/me/webdav-albums/{id}', { params: { path: { id: link.id } } })
  if (failure) {
    error.value = apiError(failure)
    return
  }
  folderAlbums.value = folderAlbums.value.filter((l) => l.id !== link.id)
  notices.notify(t('profile.unlinked', { path: link.path, album: link.albumName }))
}

function formatUsed(value: string | null) {
  return value ? formatDate(value) : t('profile.never')
}

onMounted(() => {
  void loadAppPasswords()
  void loadFolderAlbums()
})
</script>

<template>
  <div v-if="auth.user" class="page">
    <AppPageHeader :title="auth.user.login" :description="auth.isAdmin ? t('roles.admin') : t('roles.user')" />
    <div class="page-section profile">

      <v-alert v-if="error" type="error" density="compact" class="mb-4" :title="error" />

      <h2 class="mb-2">{{ t('profile.language') }}</h2>
      <AppFormGroup v-slot="field" :label="t('profile.interfaceLanguage')" class="language mb-6">
        <v-select
          v-bind="field"
          :model-value="auth.user.language"
          :items="languageChoices"
          :disabled="saving"
          :hint="t('profile.languageHint')"
          @update:model-value="save({ language: $event })"
        />
      </AppFormGroup>

      <h2 class="mb-2">{{ t('profile.grid') }}</h2>
      <v-switch
        :model-value="auth.user.gridSquares"
        :disabled="saving"
        :label="t('profile.squares')"
        :hint="t('profile.squaresHint')"
        persistent-hint
        color="primary"
        density="compact"
        class="mb-6"
        @update:model-value="save({ gridSquares: Boolean($event) })"
      />

      <h2 class="mb-2">{{ t('profile.viewer') }}</h2>
      <v-switch
        :model-value="auth.user.viewerShowsOriginals"
        :disabled="saving"
        :label="t('profile.originals')"
        :hint="t('profile.originalsHint')"
        persistent-hint
        color="primary"
        density="compact"
        class="mb-4"
        @update:model-value="save({ viewerShowsOriginals: Boolean($event) })"
      />

      <h3 class="mb-1">{{ t('profile.details') }}</h3>
      <div class="text-medium-emphasis mb-2">
        {{ t('profile.detailsHint') }}
      </div>
      <div class="fields mb-6">
        <v-checkbox
          v-for="field in viewerFields"
          :key="field"
          :model-value="shown"
          :value="field"
          :label="t(`viewerFields.${field}`)"
          :disabled="saving"
          density="compact"
          hide-details
          @update:model-value="setShown"
        />
      </div>

      <h2 class="mb-2">{{ t('profile.phone') }}</h2>
      <v-switch
        :model-value="auth.user.navigationHiddenOnPhones"
        :disabled="saving"
        :label="t('profile.hideMenu')"
        :hint="t('profile.hideMenuHint')"
        persistent-hint
        color="primary"
        density="compact"
        class="mb-6"
        @update:model-value="save({ navigationHiddenOnPhones: Boolean($event) })"
      />

      <h2 class="mb-2">{{ t('profile.password') }}</h2>
      <v-btn prepend-icon="fa-key" variant="tonal" :text="t('changePassword.title')" class="mb-6" @click="passwordDialog = true" />
      <ChangePasswordDialog v-model="passwordDialog" />

      <h2 class="mb-2">{{ t('profile.webdav') }}</h2>
      <div class="text-medium-emphasis mb-4">
        {{ t('profile.webdavHint') }}
      </div>
      <div class="webdav-access mb-6">
        <AppFormGroup v-slot="field" :label="t('profile.address')" class="mb-0">
          <CopyField v-bind="field" :value="webdavAddress" :label="t('profile.addressLower')" />
        </AppFormGroup>
        <AppFormGroup v-slot="field" :label="t('profile.login')" class="mb-0">
          <CopyField v-bind="field" :value="auth.user.login" :label="t('profile.loginLower')" />
        </AppFormGroup>
      </div>

      <h3 class="mb-2">{{ t('profile.appPasswords') }}</h3>
      <v-table v-if="appPasswords.length > 0" class="mb-4">
        <thead>
          <tr>
            <th>{{ t('profile.device') }}</th>
            <th>{{ t('profile.created') }}</th>
            <th>{{ t('profile.lastUsed') }}</th>
            <th />
          </tr>
        </thead>
        <tbody>
          <tr v-for="password in appPasswords" :key="password.id">
            <td>{{ password.name }}</td>
            <td>{{ formatDate(password.createdAt) }}</td>
            <td>{{ formatUsed(password.lastUsedAt) }}</td>
            <td class="text-right">
              <v-btn icon="fa-trash-can" size="small" variant="text" :aria-label="t('profile.revoke', { name: password.name })" @click="startRevoke(password)" />
            </td>
          </tr>
        </tbody>
      </v-table>
      <div v-else-if="appPasswordsLoaded" class="text-medium-emphasis mb-4">{{ t('profile.noAppPasswords') }}</div>
      <v-btn prepend-icon="fa-plus" variant="tonal" :text="t('appPassword.new')" @click="appPasswordDialog = true" />
      <AppPasswordDialog v-model="appPasswordDialog" @created="appPasswords.push($event)" />
      <ConfirmDialog
        v-model="revokeOpen"
        :title="t('profile.revokeTitle')"
        :text="t('profile.revokeText', { name: revoking?.name })"
        :action="t('profile.revokeAction')"
        @confirm="revoke"
      />

      <h3 class="mt-6 mb-1">{{ t('profile.folderAlbums') }}</h3>
      <div class="text-medium-emphasis mb-2">
        {{ t('profile.folderAlbumsHint') }}
      </div>
      <v-table v-if="folderAlbums.length > 0" class="mb-4">
        <thead>
          <tr>
            <th>{{ t('folderAlbum.folder') }}</th>
            <th>{{ t('folderAlbum.album') }}</th>
            <th />
          </tr>
        </thead>
        <tbody>
          <tr v-for="link in folderAlbums" :key="link.id">
            <td>{{ link.path }}</td>
            <td>
              <router-link :to="`/albums/${link.albumId}`">{{ link.albumName }}</router-link>
            </td>
            <td class="text-right">
              <v-btn icon="fa-link-slash" size="small" variant="text" :aria-label="t('profile.unlink', { path: link.path })" @click="unlink(link)" />
            </td>
          </tr>
        </tbody>
      </v-table>
      <div v-else-if="folderAlbumsLoaded" class="text-medium-emphasis mb-4">{{ t('profile.noFolders') }}</div>
      <v-btn prepend-icon="fa-link" variant="tonal" :text="t('profile.linkFolder')" @click="folderAlbumDialog = true" />
      <FolderAlbumDialog v-model="folderAlbumDialog" @created="linked" />
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

.language {
  max-width: 320px;
}

.fields {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(220px, 1fr));
}
</style>
