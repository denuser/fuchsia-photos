<script setup lang="ts">
import { computed, onMounted, onUnmounted, ref, shallowRef } from 'vue'

import { api, type LibraryFile, type LibraryFolder } from '../api/client'
import AppAutocomplete from '../components/AppAutocomplete.vue'
import AppDialog from '../components/AppDialog.vue'
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
type FileKind = 'problems' | 'duplicates' | 'unsupported'

const filesOf = ref<LibraryFolder | null>(null)
const fileKind = ref<FileKind>('problems')
const files = shallowRef<LibraryFile[]>([])
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

async function openFiles(folder: LibraryFolder, kind: FileKind = 'problems') {
  filesOf.value = folder
  await showFiles(kind)
}

async function showFiles(kind: FileKind) {
  fileKind.value = kind
  files.value = []
  const { data } = await api.GET('/library-folders/{id}/files', { params: { path: { id: filesOf.value!.id }, query: { kind } } })
  files.value = data ?? []
}

function fileNote(file: LibraryFile) {
  if (file.problem) {
    return `${t(`library.problemKinds.${file.problem}`)}${file.detail ? `: ${file.detail}` : ''}`
  }
  return file.purged ? t('library.purgedNote') : ''
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
            <th class="text-right">{{ t('library.unsupported') }}</th>
            <th class="text-right">{{ t('library.problems') }}</th>
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
            <td class="text-right">{{ folder.unsupported }}</td>
            <td class="text-right" :class="{ 'text-error': folder.problems > 0 }">{{ folder.problems }}</td>
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
              <v-btn
                variant="text"
                size="small"
                :text="t('library.files')"
                :disabled="folder.problems + folder.duplicates + folder.unsupported === 0"
                @click="openFiles(folder, folder.problems > 0 ? 'problems' : folder.duplicates > 0 ? 'duplicates' : 'unsupported')"
              />
              <v-btn variant="text" size="small" :text="t('library.scanNow')" @click="scanNow(folder)" />
              <v-btn variant="text" size="small" color="error" :text="t('library.remove')" @click="askRemove(folder)" />
            </td>
          </tr>
        </tbody>
      </v-table>

      <AppDialog
        :model-value="filesOf !== null"
        :title="t('library.filesTitle', { path: filesOf?.path })"
        :cancel="t('common.close')"
        max-width="720"
        @update:model-value="filesOf = null"
      >
        <v-btn-toggle :model-value="fileKind" mandatory density="compact" class="mb-4" @update:model-value="showFiles">
          <v-btn v-for="kind in ['problems', 'duplicates', 'unsupported']" :key="kind" :value="kind" :text="t(`library.kinds.${kind}`)" />
        </v-btn-toggle>
        <p v-if="files.length === 0" class="text-medium-emphasis">{{ t('library.noFiles') }}</p>
        <v-table v-else>
          <tbody>
            <tr v-for="file in files" :key="file.path" data-testid="library-file">
              <td class="path">{{ file.path }}</td>
              <td>{{ fileNote(file) }}</td>
            </tr>
          </tbody>
        </v-table>
      </AppDialog>

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
