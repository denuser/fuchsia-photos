<script setup lang="ts">
import { onMounted, ref } from 'vue'

import { api } from '../api/client'

const status = ref<'checking' | 'ok' | 'unavailable'>('checking')

onMounted(async () => {
  try {
    const { data } = await api.GET('/health')
    status.value = data?.status === 'ok' ? 'ok' : 'unavailable'
  } catch {
    status.value = 'unavailable'
  }
})
</script>

<template>
  <v-container>
    <v-alert v-if="status === 'checking'" type="info" text="Checking the server..." />
    <v-alert v-else-if="status === 'ok'" type="success" text="The server is up." />
    <v-alert v-else type="error" text="The server is unavailable." />
  </v-container>
</template>
