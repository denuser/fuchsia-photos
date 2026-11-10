<script setup lang="ts">
import { onMounted, onUnmounted, ref } from 'vue'

import { API_BASE, type QueueStats } from '../api/client'

const kindNames: Record<string, string> = {
  process_photo: 'Photo processing',
  render_grid: 'Grid previews',
  render_previews: 'Previews from originals',
  folder_scan: 'Folder scans',
  cleanup: 'Cleanup',
}

const stats = ref<QueueStats | null>(null)
// Set while the connection is down: "retrying" while the browser reconnects,
// "closed" once it gave up, as after a refused or ended session.
const lost = ref<'retrying' | 'closed' | null>(null)
let events: EventSource | null = null

function kindName(kind: string) {
  return kindNames[kind] ?? kind
}

function when(at: string) {
  return new Date(at).toLocaleString(undefined, { dateStyle: 'short', timeStyle: 'short' })
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
    <h2 class="mb-2">Queue</h2>
    <p class="text-medium-emphasis mb-4">Background jobs, updated as they change. Finished jobs are kept for a day.</p>

    <v-alert
      v-if="lost"
      type="warning"
      density="compact"
      class="mb-4"
      :title="lost === 'closed' ? 'Lost connection to the server. Reload the page.' : 'Lost connection to the server, reconnecting'"
    />

    <template v-if="stats">
      <p v-if="stats.kinds.length === 0" class="mb-6">No jobs in the last day.</p>
      <v-table v-else class="mb-6">
        <thead>
          <tr>
            <th>Job</th>
            <th class="text-end">Waiting</th>
            <th class="text-end">Running</th>
            <th class="text-end">Retrying</th>
            <th class="text-end">Done, 1 h</th>
            <th class="text-end">Done, 24 h</th>
            <th class="text-end">Failed, 24 h</th>
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
        <h3 class="mb-2">Latest errors</h3>
        <v-table class="mb-6">
          <thead>
            <tr>
              <th>Time</th>
              <th>Job</th>
              <th>Attempt</th>
              <th>Error</th>
            </tr>
          </thead>
          <tbody>
            <tr v-for="item in stats.errors" :key="item.jobId">
              <td class="text-no-wrap">{{ when(item.at) }}</td>
              <td>
                {{ kindName(item.kind) }}
                <div v-if="item.photoId" class="text-medium-emphasis">Photo {{ item.photoId }}</div>
              </td>
              <td class="text-no-wrap">
                {{ item.attempt }} of {{ item.maxAttempts }}
                <div class="text-medium-emphasis">{{ item.state === 'retryable' ? 'will retry' : 'gave up' }}</div>
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
