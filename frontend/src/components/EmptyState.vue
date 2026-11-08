<script setup lang="ts">
// An empty state as PatternFly lays it out: a large icon in a subtle color
// (red for a failure), a title, a line of text, and the actions that get the
// user out of it, the primary one first.
defineProps<{ icon: string; title: string; text?: string; danger?: boolean }>()
</script>

<template>
  <div class="empty-state" data-testid="empty-state">
    <v-icon :icon="icon" size="56" class="empty-state__icon" :class="{ 'empty-state__icon--danger': danger }" />
    <h2>{{ title }}</h2>
    <p v-if="text" class="empty-state__body">{{ text }}</p>
    <div v-if="$slots.actions" class="empty-state__actions">
      <slot name="actions" />
    </div>
  </div>
</template>

<style scoped>
.empty-state {
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  height: 100%;
  padding: 32px;
  text-align: center;
}

.empty-state__icon {
  margin-bottom: 24px;
  color: rgba(var(--v-theme-on-surface), var(--v-medium-emphasis-opacity));
}

.empty-state__icon--danger {
  color: rgb(var(--v-theme-error));
}

.empty-state__body {
  margin: 16px 0 0;
  font-size: 1rem;
  color: rgba(var(--v-theme-on-surface), var(--v-medium-emphasis-opacity));
}

.empty-state__actions {
  display: flex;
  flex-wrap: wrap;
  justify-content: center;
  gap: 16px;
  margin-top: 32px;
}
</style>
