<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref, watch } from 'vue'

import { previewUrl, type Photo } from '../api/client'
import { justify, visibleRows } from '../lib/justify'

const props = defineProps<{ photos: Photo[]; hasMore: boolean }>()
const emit = defineEmits<{ open: [photo: Photo]; 'load-more': [] }>()

const TARGET_HEIGHT = 200
const GAP = 4
const PADDING = 4
// Rows kept rendered outside the viewport, in pixels, so that scrolling does not flash.
const OVERSCAN = 800
const LOAD_MORE_MARGIN = 2000

const scroller = ref<HTMLElement>()
const width = ref(0)
const viewportHeight = ref(0)
const scrollTop = ref(0)

const layout = computed(() =>
  justify(
    props.photos.map((p) => ({ aspect: p.width && p.height ? p.width / p.height : 1 })),
    width.value - PADDING * 2,
    TARGET_HEIGHT,
    GAP,
  ),
)

const tiles = computed(() => {
  const { rows } = layout.value
  const [start, end] = visibleRows(rows, scrollTop.value - OVERSCAN, scrollTop.value + viewportHeight.value + OVERSCAN)
  return rows.slice(start, end).flatMap((row) =>
    row.tiles.map((tile) => ({
      photo: props.photos[tile.index]!,
      style: {
        transform: `translate(${tile.x + PADDING}px, ${row.y + PADDING}px)`,
        width: `${tile.width}px`,
        height: `${row.height}px`,
      },
    })),
  )
})

function checkLoadMore() {
  const remaining = layout.value.height - scrollTop.value - viewportHeight.value
  if (props.hasMore && remaining < LOAD_MORE_MARGIN) {
    emit('load-more')
  }
}

watch(layout, checkLoadMore)

function onScroll() {
  scrollTop.value = scroller.value?.scrollTop ?? 0
  checkLoadMore()
}

const resizeObserver = new ResizeObserver(([entry]) => {
  width.value = entry!.contentRect.width
  viewportHeight.value = entry!.contentRect.height
})

onMounted(() => resizeObserver.observe(scroller.value!))
onBeforeUnmount(() => resizeObserver.disconnect())
</script>

<template>
  <div ref="scroller" class="scroller" @scroll.passive="onScroll">
    <div class="canvas" :style="{ height: `${layout.height + PADDING * 2}px` }">
      <button
        v-for="{ photo, style } in tiles"
        :key="photo.id"
        type="button"
        class="tile"
        :style="style"
        :disabled="photo.status !== 'ready'"
        :aria-label="photo.name"
        @click="emit('open', photo)"
      >
        <img v-if="photo.status === 'ready'" :src="previewUrl(photo.id, 'grid')" :alt="photo.name" decoding="async" />
        <v-progress-circular v-else-if="photo.status === 'pending'" indeterminate size="24" width="2" />
        <v-tooltip v-else :text="`${photo.name} could not be processed`" location="top">
          <template #activator="{ props: tooltip }">
            <v-icon v-bind="tooltip" icon="mdi-image-broken-variant" />
          </template>
        </v-tooltip>
      </button>
    </div>
  </div>
</template>

<style scoped>
.scroller {
  height: 100%;
  overflow-y: auto;
}

.canvas {
  position: relative;
  overflow: hidden;
}

.tile {
  position: absolute;
  top: 0;
  left: 0;
  display: flex;
  align-items: center;
  justify-content: center;
  padding: 0;
  border: 0;
  overflow: hidden;
  background: rgba(128, 128, 128, 0.18);
  cursor: pointer;
}

.tile:disabled {
  cursor: default;
}

.tile:focus-visible {
  outline: 2px solid rgb(var(--v-theme-primary));
  outline-offset: -2px;
}

.tile img {
  display: block;
  width: 100%;
  height: 100%;
  object-fit: cover;
}
</style>
