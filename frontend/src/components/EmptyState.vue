<script setup lang="ts">
defineProps<{ icon: string; title: string; text?: string }>()
</script>

<template>
  <div class="empty">
    <v-icon :icon="icon" size="64" class="mb-4" />
    <div class="text-h6">{{ title }}</div>
    <div v-if="text" class="text-medium-emphasis">{{ text }}</div>
  </div>
</template>

<style scoped>
.empty {
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  height: 100%;
}
</style>
