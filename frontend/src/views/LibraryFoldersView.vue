<script setup lang="ts">
import { computed, onMounted, onUnmounted, ref, shallowRef } from 'vue'

import { api, type LibraryFolder } from '../api/client'
import AppAutocomplete from '../components/AppAutocomplete.vue'
import AppFormGroup from '../components/AppFormGroup.vue'
import AppPageHeader from '../components/AppPageHeader.vue'
import ConfirmDialog from '../components/ConfirmDialog.vue'
import { formatDateTime, t } from '../i18n'
import { apiError } from '../i18n/apiErrors'
import { useNoticesStore } from '../stores/notices'
import { useUsersStore } from '../stores/users'

const POLL_MS = 5000
// The intervals offered, in seconds. A folder with another one, set through the API, keeps it.
const INTERVALS = [60, 600, 3600, 21600, 86400]

const users = useUsersStore()
const notices = useNoticesStore()

const folders = shallowRef<LibraryFolder[]>([])
const available = shallowRef<string[]>([])
const owner = ref<number | null>(null)
const path = ref<string | null>(null)
const interval = ref(600)
const error = ref('')
const removing = ref<LibraryFolder | null>(null)
const removeOpen = ref(false)

const intervalItems = (current?: number) => {
  const values = current !== undefined && !INTERVALS.includes(current) ? [...INTERVALS, current].sort((a, b) => a - b) : INTERVALS
  return values.map((value) => ({ title: intervalName(value), value }))
}

function intervalName(seconds: number) {
  const names: Record<number, string> = {
    60: t('library.intervals.minute'),
    600: t('library.intervals.tenMinutes'),
    3600: t('library.intervals.hour'),
    21600: t('library.intervals.sixHours'),
    86400: t('library.intervals.day'),
  }
  return names[seconds] ?? t('library.intervals.seconds', { n: seconds })
}

const pathItems = computed(() => available.value.map((p) => ({ title: p, value: p })))

let timer: ReturnType<typeof setInterval> | undefined

onMounted(async () => {
  void users.load()
  await Promise.all([load(), loadAvailable()])
  timer = setInterval(() => void load(), POLL_MS)
})

onUnmounted(() => clearInterval(timer))

async function load() {
  const { data } = await api.GET('/library-folders')
  folders.value = data ?? []
}

async function loadAvailable() {
  const { data, error: failure } = await api.GET('/library-folders/available')
  if (failure) {
    error.value = t('library.unreadable')
  }
  available.value = data ?? []
}

async function add() {
  const { error: failure } = await api.POST('/library-folders', {
    body: { path: path.value!, ownerId: owner.value!, scanInterval: interval.value },
  })
  error.value = failure ? apiError(failure) : ''
  if (!failure) {
    path.value = null
  }
  await Promise.all([load(), loadAvailable()])
}

async function setInterval_(folder: LibraryFolder, seconds: number) {
  const { error: failure } = await api.PATCH('/library-folders/{id}', {
    params: { path: { id: folder.id } },
    body: { scanInterval: seconds },
  })
  error.value = failure ? apiError(failure) : ''
  await load()
}

async function scanNow(folder: LibraryFolder) {
  const { error: failure } = await api.POST('/library-folders/{id}/scan', { params: { path: { id: folder.id } } })
  if (failure) {
    error.value = apiError(failure)
    return
  }
  error.value = ''
  notices.notify(t('library.scanQueued', { path: folder.path }))
}

function askRemove(folder: LibraryFolder) {
  removing.value = folder
  removeOpen.value = true
}

async function remove() {
  const { error: failure } = await api.DELETE('/library-folders/{id}', { params: { path: { id: removing.value!.id } } })
  error.value = failure ? apiError(failure) : ''
  await Promise.all([load(), loadAvailable()])
}

// One line of what the last scan found, leaving out what was zero.
function summary(folder: LibraryFolder) {
  const r = folder.lastResult
  if (!r) {
    return ''
  }
  return [
    r.added > 0 && t('library.found.added', { n: r.added }, r.added),
    r.moved > 0 && t('library.found.moved', { n: r.moved }, r.moved),
    r.replaced > 0 && t('library.found.replaced', { n: r.replaced }, r.replaced),
    r.missing > 0 && t('library.found.missing', { n: r.missing }, r.missing),
    r.trashed > 0 && t('library.found.trashed', { n: r.trashed }, r.trashed),
    r.conflicts > 0 && t('library.found.conflicts', { n: r.conflicts }, r.conflicts),
    r.failed > 0 && t('library.found.failed', { n: r.failed }, r.failed),
    r.waiting > 0 && t('library.found.waiting', { n: r.waiting }, r.waiting),
  ]
    .filter(Boolean)
    .join(', ') || t('library.found.nothing')
}
</script>

<template>
  <div class="page">
    <AppPageHeader :title="t('library.title')" :description="t('library.description')" />
    <div class="page-section library">
      <v-alert v-if="error" type="error" density="compact" class="mb-4" :title="error" />

      <v-form class="d-flex flex-wrap align-end ga-4 mb-6" @submit.prevent="add">
        <AppFormGroup v-slot="field" :label="t('allPhotos.owner')" required class="field mb-0">
          <AppAutocomplete v-bind="field" v-model="owner" :items="users.active" item-title="login" item-value="id" />
        </AppFormGroup>
        <AppFormGroup v-slot="field" :label="t('folderAlbum.folder')" required class="field mb-0">
          <AppAutocomplete v-bind="field" v-model="path" :items="pathItems" data-testid="library-path" />
        </AppFormGroup>
        <AppFormGroup v-slot="field" :label="t('library.scanEvery')" required class="field mb-0">
          <v-select v-bind="field" v-model="interval" variant="outlined" :items="intervalItems()" />
        </AppFormGroup>
        <v-btn type="submit" color="primary" :text="t('library.add')" :disabled="owner === null || path === null" />
      </v-form>

      <v-table v-if="folders.length > 0">
        <thead>
          <tr>
            <th>{{ t('folderAlbum.folder') }}</th>
            <th>{{ t('allPhotos.owner') }}</th>
            <th class="text-right">{{ t('library.photos') }}</th>
            <th class="text-right">{{ t('library.missing') }}</th>
            <th class="text-right">{{ t('library.duplicates') }}</th>
            <th>{{ t('library.scanEvery') }}</th>
            <th>{{ t('library.lastScan') }}</th>
            <th :aria-label="t('users.actions')" />
          </tr>
        </thead>
        <tbody>
          <tr v-for="folder in folders" :key="folder.id" data-testid="library-folder">
            <td class="path">{{ folder.path }}</td>
            <td>{{ users.login(folder.ownerId) }}</td>
            <td class="text-right">{{ folder.photos }}</td>
            <td class="text-right" :class="{ 'text-error': folder.missing > 0 }">{{ folder.missing }}</td>
            <td class="text-right">{{ folder.duplicates }}</td>
            <td class="interval">
              <v-select
                :model-value="folder.scanInterval"
                variant="outlined"
                density="compact"
                hide-details
                :items="intervalItems(folder.scanInterval)"
                :aria-label="t('library.scanEvery')"
                @update:model-value="setInterval_(folder, $event)"
              />
            </td>
            <td>
              <template v-if="folder.lastScanAt">
                {{ formatDateTime(folder.lastScanAt) }}
                <div v-if="folder.lastError" class="text-error" data-testid="library-error">{{ apiError(folder.lastError) }}</div>
                <div v-else class="text-medium-emphasis">{{ summary(folder) }}</div>
              </template>
              <template v-else>{{ t('library.neverScanned') }}</template>
            </td>
            <td class="text-right">
              <v-btn variant="text" size="small" :text="t('library.scanNow')" @click="scanNow(folder)" />
              <v-btn variant="text" size="small" color="error" :text="t('library.remove')" @click="askRemove(folder)" />
            </td>
          </tr>
        </tbody>
      </v-table>

      <ConfirmDialog
        v-model="removeOpen"
        :title="t('library.removeTitle', { path: removing?.path })"
        :text="t('library.removeText')"
        :action="t('library.remove')"
        @confirm="remove"
      />
    </div>
  </div>
</template>

<style scoped>
.library {
  max-width: 1248px;
}

.field {
  flex: 1 1 240px;
  max-width: 360px;
}

.path {
  word-break: break-all;
}

.interval {
  min-width: 170px;
}
</style>
