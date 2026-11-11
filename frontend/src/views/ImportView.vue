<script setup lang="ts">
import { computed, onMounted, onUnmounted, ref, shallowRef } from 'vue'

import { api, type ImportProblem, type ImportRun } from '../api/client'
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
const error = ref('')

const detailsOf = shallowRef<ImportRun | null>(null)
const problems = shallowRef<ImportProblem[]>([])

const folderItems = computed(() => [
  { title: t('import.wholeFolder'), value: '' },
  ...folders.value.map((f) => ({ title: f, value: f })),
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
  return run.status === 'listing' || run.status === 'importing'
}

function processed(run: ImportRun) {
  return run.added + run.duplicates + run.inTrash + run.purged + run.unsupported + run.changing + run.failed
}

async function start() {
  const { error: failure } = await api.POST('/imports', { body: { ownerId: owner.value!, folder: folder.value } })
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
            <th class="text-right">{{ t('import.problems') }}</th>
            <th :aria-label="t('users.actions')" />
          </tr>
        </thead>
        <tbody>
          <tr v-for="run in runs" :key="run.id" data-testid="import-run">
            <td>{{ formatDateTime(run.createdAt) }}</td>
            <td>{{ users.login(run.ownerId) }}</td>
            <td>{{ run.folder || t('import.wholeFolderShort') }}</td>
            <td :title="run.error ?? undefined">{{ t(`import.statuses.${run.status}`) }}</td>
            <td class="text-right">
              <template v-if="run.status === 'importing'">{{ t('import.progress', { done: processed(run), total: run.found }) }}</template>
              <template v-else>{{ run.found }}</template>
            </td>
            <td class="text-right" data-testid="added">{{ run.added }}</td>
            <td class="text-right" data-testid="duplicates">{{ run.duplicates }}</td>
            <td class="text-right">{{ run.inTrash }}</td>
            <td class="text-right">{{ run.purged }}</td>
            <td class="text-right">{{ run.unsupported }}</td>
            <td class="text-right">{{ run.changing + run.failed }}</td>
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
