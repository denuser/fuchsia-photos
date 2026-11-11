<script setup lang="ts">
import { computed, onMounted, onUnmounted, ref } from 'vue'

import { api, type MLStatus } from '../api/client'
import { formatDateTime, t } from '../i18n'

const stateColors: Record<MLStatus['state'], string> = {
  available: 'success',
  unavailable: 'warning',
  mismatch: 'error',
  disabled: 'default',
}

const status = ref<MLStatus | null>(null)
let timer: ReturnType<typeof setInterval> | undefined

const state = computed(() =>
  status.value ? { title: t(`ml.states.${status.value.state}`), color: stateColors[status.value.state] } : null,
)
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
    <h2 class="mb-2">{{ t('ml.title') }}</h2>
    <p class="text-medium-emphasis mb-4">
      {{ t('ml.description') }}
    </p>
    <template v-if="status && state">
      <v-table class="mb-6">
        <tbody>
          <tr>
            <td>{{ t('ml.service') }}</td>
            <td><v-chip :color="state.color" size="small" :text="state.title" /></td>
          </tr>
          <tr>
            <td>{{ t('ml.model') }}</td>
            <td>{{ status.model }}</td>
          </tr>
          <tr v-if="status.serviceModel && status.serviceModel !== status.model">
            <td>{{ t('ml.serviceModel') }}</td>
            <td class="text-error">{{ status.serviceModel }}</td>
          </tr>
          <tr v-if="status.device">
            <td>{{ t('ml.device') }}</td>
            <td>{{ status.device }}</td>
          </tr>
          <tr>
            <td>{{ t('ml.processed') }}</td>
            <td>{{ t('ml.progress', { done: status.embedded, total: status.photos, percent: progress }) }}</td>
          </tr>
          <tr v-if="status.failed > 0">
            <td>{{ t('ml.skipped') }}</td>
            <td class="text-error">{{ status.failed }}</td>
          </tr>
          <tr v-if="status.error && status.state !== 'available'">
            <td>{{ t('ml.lastError') }}</td>
            <td>{{ status.error }}</td>
          </tr>
          <tr v-if="status.checkedAt">
            <td>{{ t('ml.checked') }}</td>
            <td>{{ formatDateTime(status.checkedAt) }}</td>
          </tr>
        </tbody>
      </v-table>

      <template v-if="status.failures.length">
        <h3 class="mb-2">{{ t('ml.failures') }}</h3>
        <v-table>
          <thead>
            <tr>
              <th>{{ t('ml.photo') }}</th>
              <th class="text-end">{{ t('ml.attempts') }}</th>
              <th>{{ t('ml.error') }}</th>
              <th>{{ t('ml.when') }}</th>
            </tr>
          </thead>
          <tbody>
            <tr v-for="failure in status.failures" :key="failure.photoId">
              <td>{{ failure.photoId }}</td>
              <td class="text-end">{{ failure.attempts }}</td>
              <td>{{ failure.error }}</td>
              <td>{{ formatDateTime(failure.failedAt) }}</td>
            </tr>
          </tbody>
        </v-table>
      </template>
    </template>
  </section>
</template>
