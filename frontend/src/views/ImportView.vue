<script setup lang="ts">
import { computed, onMounted, onUnmounted, ref, shallowRef } from 'vue'

import { api, type ImportProblem, type ImportRun } from '../api/client'
import { useUsersStore } from '../stores/users'

const POLL_MS = 2000

const users = useUsersStore()

const runs = shallowRef<ImportRun[]>([])
const folders = shallowRef<string[]>([])
const owner = ref<number | null>(null)
const folder = ref('')
const error = ref('')

const detailsOf = shallowRef<ImportRun | null>(null)
const problems = shallowRef<ImportProblem[]>([])

const statusTitles: Record<ImportRun['status'], string> = {
  listing: 'Listing files',
  importing: 'Importing',
  finished: 'Finished',
  stopped: 'Stopped',
  failed: 'Failed',
}

const folderItems = computed(() => [
  { title: 'The whole import folder', value: '' },
  ...folders.value.map((f) => ({ title: f, value: f })),
])

const running = computed(() => runs.value.some(isActive))

let timer: ReturnType<typeof setInterval> | undefined

onMounted(async () => {
  void users.load()
  const { data, error: failure } = await api.GET('/imports/folders')
  if (failure) {
    error.value = 'The import folder cannot be read. See the deployment guide.'
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
  error.value = failure?.error ?? ''
  await load()
}

async function stop(run: ImportRun) {
  const { error: failure } = await api.POST('/imports/{id}/stop', { params: { path: { id: run.id } } })
  error.value = failure?.error ?? ''
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
  <v-container class="imports">
    <h1 class="text-h5 mb-4">Import from folder</h1>

    <v-alert v-if="error" type="error" density="compact" class="mb-4" :text="error" />

    <v-form class="d-flex flex-wrap align-start ga-4 mb-2" @submit.prevent="start">
      <v-autocomplete
        v-model="owner"
        :items="users.active"
        item-title="login"
        item-value="id"
        label="Owner"
        density="compact"
        hide-details
        class="field"
      />
      <v-autocomplete
        v-model="folder"
        :items="folderItems"
        label="Folder"
        density="compact"
        hide-details
        class="field"
      />
      <v-btn type="submit" color="primary" text="Start scan" :disabled="owner === null || running" />
    </v-form>
    <p class="text-body-2 text-medium-emphasis mb-6">
      Photos are copied into the owner's library, and the folder stays as it is. Scanning it again adds only new photos.
    </p>

    <v-table v-if="runs.length > 0" density="compact">
      <thead>
        <tr>
          <th>Started</th>
          <th>Owner</th>
          <th>Folder</th>
          <th>Status</th>
          <th class="text-right">Files</th>
          <th class="text-right">Added</th>
          <th class="text-right">Already there</th>
          <th class="text-right">In trash</th>
          <th class="text-right">Deleted</th>
          <th class="text-right">Not photos</th>
          <th class="text-right">Problems</th>
          <th aria-label="Actions" />
        </tr>
      </thead>
      <tbody>
        <tr v-for="run in runs" :key="run.id" data-testid="import-run">
          <td>{{ new Date(run.createdAt).toLocaleString() }}</td>
          <td>{{ users.login(run.ownerId) }}</td>
          <td>{{ run.folder || 'The whole folder' }}</td>
          <td :title="run.error ?? undefined">{{ statusTitles[run.status] }}</td>
          <td class="text-right">
            <template v-if="run.status === 'importing'">{{ processed(run) }} of {{ run.found }}</template>
            <template v-else>{{ run.found }}</template>
          </td>
          <td class="text-right" data-testid="added">{{ run.added }}</td>
          <td class="text-right" data-testid="duplicates">{{ run.duplicates }}</td>
          <td class="text-right">{{ run.inTrash }}</td>
          <td class="text-right">{{ run.purged }}</td>
          <td class="text-right">{{ run.unsupported }}</td>
          <td class="text-right">{{ run.changing + run.failed }}</td>
          <td class="text-right">
            <v-btn v-if="isActive(run)" variant="text" size="small" text="Stop" @click="stop(run)" />
            <v-btn
              v-else-if="run.changing + run.failed > 0 || run.error"
              variant="text"
              size="small"
              text="Details"
              @click="openDetails(run)"
            />
          </td>
        </tr>
      </tbody>
    </v-table>

    <v-dialog :model-value="detailsOf !== null" max-width="720" @update:model-value="detailsOf = null">
      <v-card title="Scan problems">
        <v-card-text>
          <v-alert v-if="detailsOf?.error" type="error" density="compact" class="mb-4" :text="detailsOf.error" />
          <v-table density="compact">
            <tbody>
              <tr v-for="problem in problems" :key="problem.path">
                <td class="path">{{ problem.path }}</td>
                <td>{{ problem.state === 'changing' ? 'Still being copied. The next scan will try it again.' : problem.error }}</td>
              </tr>
            </tbody>
          </v-table>
        </v-card-text>
        <v-card-actions>
          <v-spacer />
          <v-btn text="Close" @click="detailsOf = null" />
        </v-card-actions>
      </v-card>
    </v-dialog>
  </v-container>
</template>

<style scoped>
.imports {
  height: calc(100vh - var(--v-layout-top, 0px));
  overflow-y: auto;
  max-width: 1200px;
}

.field {
  flex: 1 1 240px;
  max-width: 360px;
}

.path {
  word-break: break-all;
}
</style>
