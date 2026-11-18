<script setup lang="ts">
// A page that takes files and folders dropped on it, with a hint over the
// page while they are dragged. Disabled, it only lays out its content.
import { ref } from 'vue'

import { droppedFiles, type Chosen } from '../lib/uploadFiles'

const props = withDefaults(defineProps<{ hint: string; disabled?: boolean }>(), { disabled: false })
const emit = defineEmits<{ files: [chosen: Chosen] }>()

const dragging = ref(false)

function over(event: DragEvent) {
  if (props.disabled) {
    return
  }
  event.preventDefault()
  dragging.value = true
}

async function drop(event: DragEvent) {
  if (props.disabled || !event.dataTransfer) {
    return
  }
  event.preventDefault()
  dragging.value = false
  emit('files', await droppedFiles(event.dataTransfer))
}
</script>

<template>
  <div class="drop-zone" @dragover="over" @dragleave.self="dragging = false" @drop="drop">
    <slot />
    <div v-if="dragging" class="drop-hint">{{ hint }}</div>
  </div>
</template>

<style scoped>
.drop-zone {
  position: relative;
  height: 100%;
}

.drop-hint {
  position: absolute;
  inset: 8px;
  display: flex;
  align-items: center;
  justify-content: center;
  border: 2px dashed rgb(var(--v-theme-primary));
  border-radius: 8px;
  background: rgba(var(--v-theme-surface), 0.85);
  font-size: 1.25rem;
  pointer-events: none;
}
</style>
