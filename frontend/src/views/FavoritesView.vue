<script setup lang="ts">
import { onMounted } from 'vue'

import AppPageHeader from '../components/AppPageHeader.vue'
import EmptyState from '../components/EmptyState.vue'
import PhotoBrowser from '../components/PhotoBrowser.vue'
import { t } from '../i18n'
import { usePhotosStore } from '../stores/photos'

const photos = usePhotosStore()

// The user's own favorites and other users' photos the user marked.
onMounted(() => void photos.open({ withShared: true, favorites: true }))
</script>

<template>
  <PhotoBrowser mode="library">
    <template #header>
      <AppPageHeader :title="t('nav.favorites')" :description="t('favorites.description')" />
    </template>
    <template #empty>
      <EmptyState icon="fa-star" :title="t('favorites.empty')" :text="t('favorites.emptyText')" />
    </template>
  </PhotoBrowser>
</template>
