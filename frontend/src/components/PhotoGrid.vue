<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref, watch } from 'vue'

import { previewUrl, type Photo } from '../api/client'
import { useGridEdge } from '../lib/gridEdge'
import { visibleRows } from '../lib/justify'
import { layoutTimeline, monthTitle, rowHeight, visibleSections, type SectionLayout } from '../lib/timeline'
import { useAuthStore } from '../stores/auth'
import type { Section } from '../stores/photos'
import TimelineScrubber from './TimelineScrubber.vue'

// The photos of a list in sections: months with titles, or one whole list.
// Only what is near the screen is rendered, and sections near the screen are
// asked for, so that a large library loads only the months being looked at.
// Selectable tells which photos the user may select: a list can mix the user's
// photos with other users' read-only ones.
const props = defineProps<{ sections: Section[]; byMonth: boolean; selected: ReadonlySet<number>; selectable: (photo: Photo) => boolean }>()
const emit = defineEmits<{ open: [photo: Photo]; toggle: [photo: Photo, range: boolean]; want: [keys: string[]]; retry: [key: string] }>()

const PADDING = 4
// Rows kept rendered outside the viewport, in pixels, so that scrolling does not flash.
const OVERSCAN = 800
// Sections this close to the viewport load ahead of time, in pixels.
const LOAD_MARGIN = 1500
// The scrubber is shown when the list is at least this many screens long.
const SCRUBBER_SCREENS = 3

const scroller = ref<HTMLElement>()
const width = ref(0)
const viewportHeight = ref(0)
const scrollTop = ref(0)
const pixelRatio = ref(window.devicePixelRatio || 1)

const auth = useAuthStore()
const gridEdge = useGridEdge()

// Square tiles show a grid preview at its own resolution: as many CSS pixels
// as the preview has pixels on this screen.
const square = computed(() => (auth.user?.gridSquares ? gridEdge.value / pixelRatio.value : null))

const layout = computed(() => layoutTimeline(props.sections, width.value - PADDING * 2, props.byMonth, square.value))
const canvasHeight = computed(() => layout.value.height + PADDING * 2)
const canvasStyle = computed(() => ({
  height: `${canvasHeight.value}px`,
  '--row-height': `${rowHeight(width.value - PADDING * 2, square.value)}px`,
}))

const selecting = computed(() => props.selected.size > 0)

function onTileClick(photo: Photo, event: MouseEvent) {
  if (selecting.value && props.selectable(photo)) {
    emit('toggle', photo, event.shiftKey)
  } else {
    emit('open', photo)
  }
}

function at(x: number, y: number) {
  return `translate(${x + PADDING}px, ${y + PADDING}px)`
}

const visible = computed(() => {
  const top = scrollTop.value - OVERSCAN
  const bottom = scrollTop.value + viewportHeight.value + OVERSCAN
  const [start, end] = visibleSections(layout.value, top, bottom)
  return layout.value.sections.slice(start, end)
})

const headers = computed(() =>
  visible.value
    .filter((s) => s.header > 0)
    .map((s) => ({ key: s.section.key, title: monthTitle(s.section), style: { transform: at(0, s.y), height: `${s.header}px` } })),
)

const pending = computed(() =>
  visible.value
    .filter((s) => s.pendingHeight > 0)
    .map((s) => ({
      section: s.section,
      // A month shows the rows it will have; a whole list shows only its status.
      estimate: s.section.month !== null && s.section.photos.length < s.section.count,
      style: { transform: at(0, s.y + s.pendingY), width: `${width.value - PADDING * 2}px`, height: `${s.pendingHeight}px` },
    })),
)

const tiles = computed(() => {
  const top = scrollTop.value - OVERSCAN
  const bottom = scrollTop.value + viewportHeight.value + OVERSCAN
  return visible.value.flatMap((s) => {
    const offset = s.y + s.header
    const [start, end] = visibleRows(s.rows, top - offset, bottom - offset)
    return s.rows.slice(start, end).flatMap((row) =>
      row.tiles.map((tile) => ({
        photo: s.section.photos[tile.index]!,
        style: {
          transform: at(tile.x, offset + row.y),
          width: `${tile.width}px`,
          height: `${row.height}px`,
        },
      })),
    )
  })
})

// Asks for the sections whose unloaded part is near the screen, nearest first.
function requestSections() {
  const top = scrollTop.value - LOAD_MARGIN
  const bottom = scrollTop.value + viewportHeight.value + LOAD_MARGIN
  const centre = scrollTop.value + viewportHeight.value / 2
  const [start, end] = visibleSections(layout.value, top, bottom)
  const wanted: { key: string; distance: number }[] = []
  for (const s of layout.value.sections.slice(start, end)) {
    if (s.pendingHeight === 0 || s.section.loading || s.section.failed) {
      continue
    }
    const from = s.y + s.pendingY
    const to = from + s.pendingHeight
    if (to < top || from > bottom) {
      continue
    }
    wanted.push({ key: s.section.key, distance: centre < from ? from - centre : centre > to ? centre - to : 0 })
  }
  wanted.sort((a, b) => a.distance - b.distance)
  emit(
    'want',
    wanted.map((w) => w.key),
  )
}

// Asks at most every 100 ms while scrolling, and once more when it stops.
let requestTimer: ReturnType<typeof setTimeout> | undefined
let requestedAt = 0

function scheduleRequest() {
  clearTimeout(requestTimer)
  const wait = Math.max(0, requestedAt + 100 - Date.now())
  requestTimer = setTimeout(() => {
    requestedAt = Date.now()
    requestSections()
  }, wait)
}

// When a section above the screen loads, its real height replaces the
// estimate. The list keeps the first section on the screen where it was, so
// that the photos being looked at do not jump.
let anchor: { key: string; offset: number } | null = null

function sectionAt(y: number): SectionLayout | undefined {
  const [start] = visibleSections(layout.value, y, y)
  return layout.value.sections[start]
}

function rememberAnchor() {
  const s = sectionAt(scrollTop.value - PADDING)
  anchor = s ? { key: s.section.key, offset: scrollTop.value - s.y } : null
}

watch(
  layout,
  (next) => {
    const element = scroller.value
    if (element && anchor && element.scrollTop > 0) {
      const s = next.sections.find((l) => l.section.key === anchor!.key)
      if (s && Math.abs(s.y + anchor.offset - element.scrollTop) > 1) {
        element.scrollTop = s.y + anchor.offset
        scrollTop.value = element.scrollTop
      }
    }
    rememberAnchor()
    scheduleRequest()
  },
  { flush: 'post' },
)

function onScroll() {
  scrollTop.value = scroller.value?.scrollTop ?? 0
  rememberAnchor()
  scheduleRequest()
}

const scrubber = computed(() => props.byMonth && canvasHeight.value > viewportHeight.value * SCRUBBER_SCREENS)

// The first month of each year, for the scrubber.
const years = computed(() => {
  const marks: { y: number; label: string }[] = []
  let last: number | null = null
  for (const s of layout.value.sections) {
    const year = s.section.month?.start.getFullYear()
    if (year !== undefined && year !== last) {
      marks.push({ y: s.y, label: String(year) })
      last = year
    }
  }
  return marks
})

function labelAt(y: number) {
  const s = sectionAt(y)
  return s ? monthTitle(s.section) : ''
}

function seek(top: number) {
  if (scroller.value) {
    scroller.value.scrollTop = top
  }
}

const resizeObserver = new ResizeObserver(([entry]) => {
  width.value = entry!.contentRect.width
  viewportHeight.value = entry!.contentRect.height
  // Zooming the page changes the ratio and resizes the grid.
  pixelRatio.value = window.devicePixelRatio || 1
})

onMounted(() => resizeObserver.observe(scroller.value!))
onBeforeUnmount(() => {
  resizeObserver.disconnect()
  clearTimeout(requestTimer)
})
</script>

<template>
  <div class="grid">
    <div ref="scroller" class="scroller" data-testid="grid-scroller" :class="{ scrubbed: scrubber }" @scroll.passive="onScroll">
      <div class="canvas" :style="canvasStyle">
        <h2 v-for="header in headers" :key="header.key" class="month" data-testid="month" :style="header.style">
          {{ header.title }}
        </h2>
        <div v-for="item in pending" :key="item.section.key" class="pending" :class="{ estimate: item.estimate }" :style="item.style">
          <div v-if="item.section.failed" class="status">
            <span>The photos could not be loaded.</span>
            <v-btn text="Retry" variant="text" color="primary" @click="emit('retry', item.section.key)" />
          </div>
          <div v-else-if="!item.estimate" class="status">
            <v-progress-circular indeterminate size="24" width="2" aria-label="Loading photos" />
          </div>
        </div>
        <div
          v-for="{ photo, style } in tiles"
          :key="photo.id"
          class="tile"
          data-testid="tile"
          :class="{ selected: selected.has(photo.id), selecting }"
          :style="style"
        >
          <button
            type="button"
            class="open"
            data-testid="tile-open"
            :disabled="photo.status !== 'ready' && !(selecting && selectable(photo))"
            :aria-label="photo.name"
            @click="onTileClick(photo, $event)"
          >
            <img v-if="photo.status === 'ready'" :src="previewUrl(photo.id, 'grid', photo.previewVersion)" :alt="photo.name" decoding="async" />
            <v-progress-circular v-else-if="photo.status === 'pending'" indeterminate size="24" width="2" />
            <v-tooltip v-else :text="`${photo.name} could not be processed`" location="top">
              <template #activator="{ props: tooltip }">
                <v-icon v-bind="tooltip" icon="fa-triangle-exclamation" />
              </template>
            </v-tooltip>
          </button>
          <button
            v-if="selectable(photo)"
            type="button"
            class="check"
            data-testid="tile-check"
            role="checkbox"
            :aria-checked="selected.has(photo.id)"
            :aria-label="`Select ${photo.name}`"
            @click="emit('toggle', photo, $event.shiftKey)"
          >
            <v-icon :icon="selected.has(photo.id) ? 'fa-circle-check' : 'fa-regular-circle'" size="22" />
          </button>
        </div>
      </div>
    </div>
    <TimelineScrubber
      v-if="scrubber"
      :height="canvasHeight"
      :viewport="viewportHeight"
      :scroll-top="scrollTop"
      :marks="years"
      :label-at="labelAt"
      @seek="seek"
    />
  </div>
</template>

<style scoped>
.grid {
  position: relative;
  height: 100%;
}

.scroller {
  height: 100%;
  overflow-y: auto;
}

/* The scrubber takes the place of the scroll bar. */
.scroller.scrubbed {
  scrollbar-width: none;
}

.scroller.scrubbed::-webkit-scrollbar {
  display: none;
}

.month,
.pending {
  position: absolute;
  top: 0;
  left: 0;
}

.month {
  display: flex;
  align-items: flex-end;
  padding: 0 8px 8px;
  font-size: 1rem;
  white-space: nowrap;
}

/* Rows of grey blocks where the photos of a month will be. */
.pending.estimate {
  background: repeating-linear-gradient(
    to bottom,
    rgba(128, 128, 128, 0.12) 0 var(--row-height),
    transparent var(--row-height) calc(var(--row-height) + 4px)
  );
}

.status {
  display: flex;
  align-items: center;
  justify-content: center;
  gap: 8px;
  height: 64px;
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
