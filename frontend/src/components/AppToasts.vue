<script setup lang="ts">
// The toast alerts of the notices store, stacked in the top right corner over
// everything else, newest at the bottom, as PatternFly places them. They start
// under the app bar, so that they do not hide the Upload button.
import { useNoticesStore } from '../stores/notices'

const notices = useNoticesStore()
</script>

<template>
  <div class="toasts" aria-live="polite">
    <TransitionGroup name="toast">
      <v-alert
        v-for="notice in notices.notices"
        :key="notice.id"
        :type="notice.type"
        :title="notice.title"
        closable
        class="toast"
        data-testid="toast"
        @click:close="notices.dismiss(notice.id)"
      />
    </TransitionGroup>
  </div>
</template>

<style scoped>
.toasts {
  position: fixed;
  top: 72px;
  right: 16px;
  z-index: 3000;
  display: flex;
  flex-direction: column;
  gap: 8px;
  width: min(400px, calc(100vw - 32px));
  pointer-events: none;
}

.toast {
  pointer-events: auto;
  background: rgb(var(--v-theme-surface-bright));
  box-shadow: var(--shadow-lg);
}

.toast-enter-active,
.toast-leave-active {
  transition:
    opacity 0.2s,
    transform 0.2s;
}

.toast-enter-from,
.toast-leave-to {
  opacity: 0;
  transform: translateX(24px);
}
</style>
