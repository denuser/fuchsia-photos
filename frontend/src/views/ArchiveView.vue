<script setup lang="ts">
import { onMounted } from 'vue'

import AppPageHeader from '../components/AppPageHeader.vue'
import EmptyState from '../components/EmptyState.vue'
import PhotoBrowser from '../components/PhotoBrowser.vue'
import { t } from '../i18n'
import { usePhotosStore } from '../stores/photos'

const photos = usePhotosStore()

// The user's own photos and other users' photos the user archived.
onMounted(() => void photos.open({ withShared: true, archived: true }))
</script>

<template>
  <PhotoBrowser mode="library">
    <template #header>
      <AppPageHeader :title="t('nav.archive')" :description="t('archive.description')" />
    </template>
    <template #empty>
      <EmptyState icon="fa-box-archive" :title="t('archive.empty')" :text="t('archive.emptyText')" />
    </template>
  </PhotoBrowser>
</template>
