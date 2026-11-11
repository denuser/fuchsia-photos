<script setup lang="ts">
import { onMounted, onUnmounted, ref } from 'vue'

import { API_BASE, type QueueStats } from '../api/client'
import { formatDateTime, t } from '../i18n'

const kinds = ['process_photo', 'render_grid', 'render_previews', 'folder_scan', 'embed_photos', 'cleanup'] as const

const stats = ref<QueueStats | null>(null)
// Set while the connection is down: "retrying" while the browser reconnects,
// "closed" once it gave up, as after a refused or ended session.
const lost = ref<'retrying' | 'closed' | null>(null)
let events: EventSource | null = null

function kindName(kind: string) {
  return (kinds as readonly string[]).includes(kind) ? t(`queue.kinds.${kind as (typeof kinds)[number]}`) : kind
}

// The server sends the queue at once and then whenever it changes. The browser
// reconnects by itself after a dropped connection.
onMounted(() => {
  events = new EventSource(`${API_BASE}/queue/events`)
  events.addEventListener('queue', (message) => {
    stats.value = JSON.parse(message.data) as QueueStats
    lost.value = null
  })
  events.onerror = () => {
    lost.value = events?.readyState === EventSource.CLOSED ? 'closed' : 'retrying'
  }
})

onUnmounted(() => {
  events?.close()
  events = null
})
</script>

<template>
  <section class="queue" data-testid="queue">
    <h2 class="mb-2">{{ t('queue.title') }}</h2>
    <p class="text-medium-emphasis mb-4">{{ t('queue.description') }}</p>

    <v-alert
      v-if="lost"
      type="warning"
      density="compact"
      class="mb-4"
      :title="lost === 'closed' ? t('queue.closed') : t('queue.retrying')"
    />

    <template v-if="stats">
      <p v-if="stats.kinds.length === 0" class="mb-6">{{ t('queue.empty') }}</p>
      <v-table v-else class="mb-6">
        <thead>
          <tr>
            <th>{{ t('queue.job') }}</th>
            <th class="text-end">{{ t('queue.waiting') }}</th>
            <th class="text-end">{{ t('queue.running') }}</th>
            <th class="text-end">{{ t('queue.retryingJobs') }}</th>
            <th class="text-end">{{ t('queue.doneHour') }}</th>
            <th class="text-end">{{ t('queue.doneDay') }}</th>
            <th class="text-end">{{ t('queue.failedDay') }}</th>
          </tr>
        </thead>
        <tbody>
          <tr v-for="row in stats.kinds" :key="row.kind">
            <td>{{ kindName(row.kind) }}</td>
            <td class="text-end">{{ row.waiting }}</td>
            <td class="text-end">{{ row.running }}</td>
            <td class="text-end">{{ row.retrying }}</td>
            <td class="text-end">{{ row.doneHour }}</td>
            <td class="text-end">{{ row.doneDay }}</td>
            <td class="text-end" :class="{ 'text-error': row.failedDay > 0 }">{{ row.failedDay }}</td>
          </tr>
        </tbody>
      </v-table>

      <template v-if="stats.errors.length">
        <h3 class="mb-2">{{ t('queue.errors') }}</h3>
        <v-table class="mb-6">
          <thead>
            <tr>
              <th>{{ t('queue.time') }}</th>
              <th>{{ t('queue.job') }}</th>
              <th>{{ t('queue.attempt') }}</th>
              <th>{{ t('queue.error') }}</th>
            </tr>
          </thead>
          <tbody>
            <tr v-for="item in stats.errors" :key="item.jobId">
              <td class="text-no-wrap">{{ formatDateTime(item.at) }}</td>
              <td>
                {{ kindName(item.kind) }}
                <div v-if="item.photoId" class="text-medium-emphasis">{{ t('queue.photo', { id: item.photoId }) }}</div>
              </td>
              <td class="text-no-wrap">
                {{ t('queue.attemptOf', { attempt: item.attempt, max: item.maxAttempts }) }}
                <div class="text-medium-emphasis">{{ item.state === 'retryable' ? t('queue.willRetry') : t('queue.gaveUp') }}</div>
              </td>
              <td class="error-text">{{ item.error }}</td>
            </tr>
          </tbody>
        </v-table>
      </template>
    </template>
  </section>
</template>

<style scoped>
.queue {
  max-width: 840px;
}

.error-text {
  overflow-wrap: anywhere;
}
</style>
