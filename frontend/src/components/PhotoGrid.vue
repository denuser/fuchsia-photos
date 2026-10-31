<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref, watch } from 'vue'

import { previewUrl, type Photo } from '../api/client'
import { justify, visibleRows } from '../lib/justify'

const props = defineProps<{ photos: Photo[]; hasMore: boolean; selected: ReadonlySet<number> }>()
const emit = defineEmits<{ open: [photo: Photo]; 'load-more': []; toggle: [photo: Photo, range: boolean] }>()

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

const selecting = computed(() => props.selected.size > 0)

function onTileClick(photo: Photo, event: MouseEvent) {
  if (selecting.value) {
    emit('toggle', photo, event.shiftKey)
  } else {
    emit('open', photo)
  }
}

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
      <div
        v-for="{ photo, style } in tiles"
        :key="photo.id"
        class="tile"
        :class="{ selected: selected.has(photo.id), selecting }"
        :style="style"
      >
        <button
          type="button"
          class="open"
          :disabled="photo.status !== 'ready' && !selecting"
          :aria-label="photo.name"
          @click="onTileClick(photo, $event)"
        >
          <img v-if="photo.status === 'ready'" :src="previewUrl(photo.id, 'grid')" :alt="photo.name" decoding="async" />
          <v-progress-circular v-else-if="photo.status === 'pending'" indeterminate size="24" width="2" />
          <v-tooltip v-else :text="`${photo.name} could not be processed`" location="top">
            <template #activator="{ props: tooltip }">
              <v-icon v-bind="tooltip" icon="mdi-image-broken-variant" />
            </template>
          </v-tooltip>
        </button>
        <button
          type="button"
          class="check"
          role="checkbox"
          :aria-checked="selected.has(photo.id)"
          :aria-label="`Select ${photo.name}`"
          @click="emit('toggle', photo, $event.shiftKey)"
        >
          <v-icon :icon="selected.has(photo.id) ? 'mdi-check-circle' : 'mdi-circle-outline'" size="22" />
        </button>
      </div>
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
  overflow: hidden;
  background: rgba(128, 128, 128, 0.18);
}

.open {
  display: flex;
  align-items: center;
  justify-content: center;
  width: 100%;
  height: 100%;
  padding: 0;
  border: 0;
  background: none;
  cursor: pointer;
}

.open:disabled {
  cursor: default;
}

.open:focus-visible,
.check:focus-visible {
  outline: 2px solid rgb(var(--v-theme-primary));
  outline-offset: -2px;
}

.open img {
  display: block;
  width: 100%;
  height: 100%;
  object-fit: cover;
  transition: transform 0.1s;
}

.check {
  position: absolute;
  top: 4px;
  left: 4px;
  display: flex;
  padding: 2px;
  border: 0;
  border-radius: 50%;
  background: rgba(0, 0, 0, 0.35);
  color: #fff;
  cursor: pointer;
  opacity: 0;
}

.tile:hover .check,
.check:focus-visible,
.selecting .check {
  opacity: 1;
}

.selected .check {
  background: #fff;
  color: rgb(var(--v-theme-primary));
}

.selected .open img {
  transform: scale(0.92);
}

.selected {
  background: rgba(var(--v-theme-primary), 0.2);
}

@media (hover: none) {
  .check {
    opacity: 1;
  }
}
</style>
