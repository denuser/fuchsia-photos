<script setup lang="ts">
import { computed, onMounted, onUnmounted, ref } from 'vue'

import { api, type MLStatus } from '../api/client'

const stateNames: Record<MLStatus['state'], { title: string; color: string }> = {
  available: { title: 'Available', color: 'success' },
  unavailable: { title: 'Unavailable', color: 'warning' },
  mismatch: { title: 'Runs another model', color: 'error' },
  disabled: { title: 'Not configured', color: 'default' },
}

const status = ref<MLStatus | null>(null)
let timer: ReturnType<typeof setInterval> | undefined

const state = computed(() => (status.value ? stateNames[status.value.state] : null))
const progress = computed(() => {
  const s = status.value
  return s && s.photos > 0 ? Math.round((s.embedded / s.photos) * 100) : 0
})

async function load() {
  const { data } = await api.GET('/ml')
  if (data) {
    status.value = data
  }
}

function when(at: string) {
  return new Date(at).toLocaleString(undefined, { dateStyle: 'short', timeStyle: 'short' })
}

// The server checks the service every 30 seconds, and the job embeds photos
// in batches, so a few seconds are often enough.
onMounted(() => {
  void load()
  timer = setInterval(() => void load(), 5000)
})
onUnmounted(() => clearInterval(timer))
</script>

<template>
  <section data-testid="ml">
    <h2 class="mb-2">Machine learning</h2>
    <p class="text-medium-emphasis mb-4">
      The service that makes search by description and similar photos work. Photos are processed while it is available.
    </p>
    <template v-if="status && state">
      <v-table class="mb-6">
        <tbody>
          <tr>
            <td>Service</td>
            <td><v-chip :color="state.color" size="small" :text="state.title" /></td>
          </tr>
          <tr>
            <td>Model</td>
            <td>{{ status.model }}</td>
          </tr>
          <tr v-if="status.serviceModel && status.serviceModel !== status.model">
            <td>Service model</td>
            <td class="text-error">{{ status.serviceModel }}</td>
          </tr>
          <tr v-if="status.device">
            <td>Device</td>
            <td>{{ status.device }}</td>
          </tr>
          <tr>
            <td>Processed photos</td>
            <td>{{ status.embedded }} of {{ status.photos }} ({{ progress }}%)</td>
          </tr>
          <tr v-if="status.failed > 0">
            <td>Skipped after failures</td>
            <td class="text-error">{{ status.failed }}</td>
          </tr>
          <tr v-if="status.error && status.state !== 'available'">
            <td>Last error</td>
            <td>{{ status.error }}</td>
          </tr>
          <tr v-if="status.checkedAt">
            <td>Checked</td>
            <td>{{ when(status.checkedAt) }}</td>
          </tr>
        </tbody>
      </v-table>

      <template v-if="status.failures.length">
        <h3 class="mb-2">Latest failures</h3>
        <v-table>
          <thead>
            <tr>
              <th>Photo</th>
              <th class="text-end">Attempts</th>
              <th>Error</th>
              <th>When</th>
            </tr>
          </thead>
          <tbody>
            <tr v-for="failure in status.failures" :key="failure.photoId">
              <td>{{ failure.photoId }}</td>
              <td class="text-end">{{ failure.attempts }}</td>
              <td>{{ failure.error }}</td>
              <td>{{ when(failure.failedAt) }}</td>
            </tr>
          </tbody>
        </v-table>
      </template>
    </template>
  </section>
</template>
