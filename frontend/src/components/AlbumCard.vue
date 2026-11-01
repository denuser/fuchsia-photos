<script setup lang="ts">
import { previewUrl, type Album } from '../api/client'

defineProps<{ album: Album; subtitle: string }>()
</script>

<template>
  <v-card :to="`/albums/${album.id}`" variant="flat" border>
    <v-img v-if="album.coverPhotoId" :src="previewUrl(album.coverPhotoId, 'grid')" :alt="album.name" aspect-ratio="1.33" cover />
    <div v-else class="no-cover"><v-icon icon="mdi-image-album" size="48" /></div>
    <v-card-item :title="album.name" :subtitle="subtitle">
      <template v-if="$slots.actions" #append>
        <slot name="actions" />
      </template>
    </v-card-item>
  </v-card>
</template>

<style scoped>
.no-cover {
  display: flex;
  align-items: center;
  justify-content: center;
  aspect-ratio: 1.33;
  background: rgba(128, 128, 128, 0.18);
}
</style>
