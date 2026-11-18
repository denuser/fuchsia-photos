<script setup lang="ts">
import { computed, onMounted, onUnmounted, ref, shallowRef } from 'vue'

import { api, type ImportAlbumTrash, type ImportKind, type ImportProblem, type ImportRun } from '../api/client'
import AppAutocomplete from '../components/AppAutocomplete.vue'
import { formatDateTime, t } from '../i18n'
import { apiError } from '../i18n/apiErrors'
import { useUsersStore } from '../stores/users'
import AppDialog from '../components/AppDialog.vue'
import AppFormGroup from '../components/AppFormGroup.vue'
import AppPageHeader from '../components/AppPageHeader.vue'

const POLL_MS = 2000

const users = useUsersStore()

const runs = shallowRef<ImportRun[]>([])
const folders = shallowRef<string[]>([])
const owner = ref<number | null>(null)
const folder = ref('')
const kind = ref<ImportKind>('folder')
const albumTrash = ref<ImportAlbumTrash>('skip')
const error = ref('')

const detailsOf = shallowRef<ImportRun | null>(null)
const problems = shallowRef<ImportProblem[]>([])

const folderItems = computed(() => [
  { title: t('import.wholeFolder'), value: '' },
  ...folders.value.map((f) => ({ title: f, value: f })),
])

const kindItems = computed(() => [
  { title: t('import.kinds.folder'), value: 'folder' },
  { title: t('import.kinds.takeout'), value: 'takeout' },
])

const albumTrashItems = computed(() => [
  { title: t('import.albumTrashes.skip'), value: 'skip' },
  { title: t('import.albumTrashes.add'), value: 'add' },
  { title: t('import.albumTrashes.restore'), value: 'restore' },
])

const running = computed(() => runs.value.some(isActive))

let timer: ReturnType<typeof setInterval> | undefined

onMounted(async () => {
  void users.load()
  const { data, error: failure } = await api.GET('/imports/folders')
  if (failure) {
    error.value = t('import.unreadable')
  }
  folders.value = data ?? []
  await load()
  timer = setInterval(() => {
    if (running.value) {
      void load()
    }
  }, POLL_MS)
})

onUnmounted(() => clearInterval(timer))

async function load() {
  const { data } = await api.GET('/imports')
  runs.value = data ?? []
}

function isActive(run: ImportRun) {
  return run.status === 'listing' || run.status === 'importing' || run.status === 'matching'
}

function processed(run: ImportRun) {
  return run.added + run.duplicates + run.inTrash + run.purged + run.unsupported + run.changing + run.failed
}

// A finished scan with failed files reads as failed when nothing else came of
// it, as when the import folder cannot be read, and as finished with errors
// otherwise, so that it does not pass for a scan that found nothing.
function statusOf(run: ImportRun) {
  if (run.status !== 'finished' || run.failed === 0) {
    return { text: t(`import.statuses.${run.status}`), failed: run.status === 'failed' }
  }
  if (processed(run) === run.failed && run.sidecars === 0) {
    return { text: t('import.statuses.failed'), failed: true }
  }
  return { text: t('import.finishedWithErrors'), failed: true }
}

// The application's user cannot read the file: a problem of the NAS's
// permissions, not of the file.
const permissionDenied = computed(() => problems.value.some((p) => p.error?.includes('permission denied')))

// A Takeout scan learns how many files its parts hold only as it reads them.
function showsProgress(run: ImportRun) {
  return run.status === 'importing' && run.kind === 'folder'
}

async function start() {
  const { error: failure } = await api.POST('/imports', {
    body: {
      ownerId: owner.value!,
      folder: folder.value,
      kind: kind.value,
      ...(kind.value === 'takeout' ? { albumTrash: albumTrash.value } : {}),
    },
  })
  error.value = failure ? apiError(failure) : ''
  await load()
}

async function stop(run: ImportRun) {
  const { error: failure } = await api.POST('/imports/{id}/stop', { params: { path: { id: run.id } } })
  error.value = failure ? apiError(failure) : ''
  await load()
}

async function openDetails(run: ImportRun) {
  detailsOf.value = run
  problems.value = []
  const { data } = await api.GET('/imports/{id}', { params: { path: { id: run.id } } })
  problems.value = data?.problems ?? []
}
</script>

<template>
  <div class="page">
    <AppPageHeader
      :title="t('import.title')"
      :description="t('import.description')"
    />
    <div class="page-section imports">

      <v-alert v-if="error" type="error" density="compact" class="mb-4" :title="error" />

      <v-form class="d-flex flex-wrap align-end ga-4 mb-6" @submit.prevent="start">
        <AppFormGroup v-slot="field" :label="t('allPhotos.owner')" required class="field mb-0">
          <AppAutocomplete v-bind="field" v-model="owner" :items="users.active" item-title="login" item-value="id" />
        </AppFormGroup>
        <AppFormGroup v-slot="field" :label="t('folderAlbum.folder')" required class="field mb-0">
          <AppAutocomplete v-bind="field" v-model="folder" :items="folderItems" />
        </AppFormGroup>
        <AppFormGroup v-slot="field" :label="t('import.kind')" required class="field mb-0">
          <v-select v-bind="field" v-model="kind" variant="outlined" :items="kindItems" data-testid="import-kind" />
        </AppFormGroup>
        <AppFormGroup v-if="kind === 'takeout'" v-slot="field" :label="t('import.albumTrash')" required class="field mb-0">
          <v-select v-bind="field" v-model="albumTrash" variant="outlined" :items="albumTrashItems" />
        </AppFormGroup>
        <v-btn type="submit" color="primary" :text="t('import.start')" :disabled="owner === null || running" />
      </v-form>

      <v-table v-if="runs.length > 0">
        <thead>
          <tr>
            <th>{{ t('import.started') }}</th>
            <th>{{ t('allPhotos.owner') }}</th>
            <th>{{ t('folderAlbum.folder') }}</th>
            <th>{{ t('import.status') }}</th>
            <th class="text-right">{{ t('import.files') }}</th>
            <th class="text-right">{{ t('import.added') }}</th>
            <th class="text-right">{{ t('import.duplicates') }}</th>
            <th class="text-right">{{ t('import.inTrash') }}</th>
            <th class="text-right">{{ t('import.purged') }}</th>
            <th class="text-right">{{ t('import.unsupported') }}</th>
            <th class="text-right">{{ t('import.sidecars') }}</th>
            <th class="text-right">{{ t('import.problems') }}</th>
            <th :aria-label="t('users.actions')" />
          </tr>
        </thead>
        <tbody>
          <tr v-for="run in runs" :key="run.id" data-testid="import-run">
            <td>{{ formatDateTime(run.createdAt) }}</td>
            <td>{{ users.login(run.ownerId) }}</td>
            <td>
              {{ run.folder || t('import.wholeFolderShort') }}
              <div v-if="run.kind === 'takeout'" class="text-medium-emphasis">{{ t('import.kinds.takeout') }}</div>
            </td>
            <td :title="run.error ?? undefined" :class="{ 'text-error': statusOf(run).failed }" data-testid="import-status">
              {{ statusOf(run).text }}
            </td>
            <td class="text-right">
              <template v-if="showsProgress(run)">{{ t('import.progress', { done: processed(run), total: run.found }) }}</template>
              <template v-else>{{ run.found }}</template>
            </td>
            <td class="text-right" data-testid="added">{{ run.added }}</td>
            <td class="text-right" data-testid="duplicates">{{ run.duplicates }}</td>
            <td class="text-right">{{ run.inTrash }}</td>
            <td class="text-right">{{ run.purged }}</td>
            <td class="text-right">{{ run.unsupported }}</td>
            <td class="text-right">
              <template v-if="run.kind === 'takeout'">
                {{ run.sidecars }}
                <div v-if="run.status === 'finished'" class="text-medium-emphasis" data-testid="matching">
                  {{ t('import.matching', { edited: run.edited, withoutSidecar: run.withoutSidecar, unmatched: run.unmatchedSidecars }) }}
                  <div v-if="run.albums > 0" data-testid="albums">
                    {{ t('import.albums', { n: run.albums }, run.albums) }}<template v-if="run.restored > 0">,
                      {{ t('import.restored', { n: run.restored }, run.restored) }}</template>
                  </div>
                  <div v-if="run.trashed > 0" data-testid="trashed">
                    {{ t('import.trashed', { n: run.trashed }, run.trashed) }}
                  </div>
                </div>
              </template>
            </td>
            <td class="text-right" :class="{ 'text-error': run.failed > 0 }">{{ run.changing + run.failed }}</td>
            <td class="text-right">
              <v-btn v-if="isActive(run)" variant="text" size="small" :text="t('import.stop')" @click="stop(run)" />
              <v-btn
                v-else-if="run.changing + run.failed > 0 || run.error"
                variant="text"
                size="small"
                :text="t('import.details')"
                @click="openDetails(run)"
              />
            </td>
          </tr>
        </tbody>
      </v-table>

      <AppDialog
        :model-value="detailsOf !== null"
        :title="t('import.problemsTitle')"
        :cancel="t('common.close')"
        :error="detailsOf?.error ?? ''"
        max-width="720"
        @update:model-value="detailsOf = null"
      >
        <v-alert
          v-if="permissionDenied"
          type="info"
          density="compact"
          class="mb-4"
          :title="t('import.permissionDenied')"
          :text="t('import.permissionDeniedHelp')"
          data-testid="permission-denied"
        />
        <v-table>
          <tbody>
            <tr v-for="problem in problems" :key="problem.path">
              <td class="path">{{ problem.path }}</td>
              <td>{{ problem.state === 'changing' ? t('import.changing') : problem.error }}</td>
            </tr>
          </tbody>
        </v-table>
      </AppDialog>
    </div>
  </div>
</template>

<style scoped>
.imports {
  max-width: 1248px;
}

.field {
  flex: 1 1 240px;
  max-width: 360px;
}

.path {
  word-break: break-all;
}
</style>
