<script setup lang="ts">
import { onMounted } from 'vue'

import AppPageHeader from '../components/AppPageHeader.vue'
import EmptyState from '../components/EmptyState.vue'
import PhotoBrowser from '../components/PhotoBrowser.vue'
import { t } from '../i18n'
import { usePhotosStore } from '../stores/photos'

const photos = usePhotosStore()

// The photos of this day in earlier years, a section for each year.
onMounted(() => void photos.open({ memories: true }))
</script>

<template>
  <PhotoBrowser mode="library">
    <template #header>
      <AppPageHeader :title="t('nav.memories')" :description="t('memories.description')" />
    </template>
    <template #empty>
      <EmptyState icon="fa-clock-rotate-left" :title="t('memories.empty')" :text="t('memories.emptyText')" />
    </template>
  </PhotoBrowser>
</template>
