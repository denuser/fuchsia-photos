<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { useDisplay } from 'vuetify'

import { isVideo, previewUrl, type Photo } from '../api/client'
import { formatDuration } from '../lib/duration'
import { visibleRows } from '../lib/justify'
import { layoutTimeline, monthTitle, tileSide, visibleSections, type SectionLayout } from '../lib/timeline'
import { useAuthStore } from '../stores/auth'
import type { Section } from '../stores/photos'
import TimelineScrubber from './TimelineScrubber.vue'
import { t } from '../i18n'

// The photos of a list in sections: months with titles, or one whole list.
// Only what is near the screen is rendered, and sections near the screen are
// asked for, so that a large library loads only the months being looked at.
// Selectable tells which photos the user may select: a list can mix the user's
// photos with other users' read-only ones.
const props = defineProps<{ sections: Section[]; byMonth: boolean; selected: ReadonlySet<number>; selectable: (photo: Photo) => boolean }>()
const emit = defineEmits<{ open: [photo: Photo]; toggle: [photo: Photo, range: boolean]; want: [keys: string[]]; retry: [key: string] }>()

// The photos line up with the page title and the toolbar: their sides keep
// the padding of a PatternFly page section, 24px, or 16px on a phone, as
// .page-section does. The header above already ends with its own padding.
const { xs } = useDisplay()
const paddingX = computed(() => (xs.value ? 16 : 24))
const PADDING_TOP = 0
const paddingBottom = computed(() => paddingX.value)
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

const auth = useAuthStore()

// The width the tiles fill.
const inner = computed(() => width.value - paddingX.value * 2)

// The size the user chose, in CSS pixels for this width: the target height of
// rows that keep each photo's shape, and the side of a square.
const side = computed(() => tileSide(inner.value, auth.user?.gridSize ?? 'medium'))
const square = computed(() => (auth.user?.gridSquares ? side.value : null))

const layout = computed(() => layoutTimeline(props.sections, inner.value, props.byMonth, side.value, square.value))
const canvasHeight = computed(() => layout.value.height + PADDING_TOP + paddingBottom.value)
const canvasStyle = computed(() => ({ height: `${canvasHeight.value}px` }))

const selecting = computed(() => props.selected.size > 0)

function onTileClick(photo: Photo, event: MouseEvent) {
  if (selecting.value && props.selectable(photo)) {
    emit('toggle', photo, event.shiftKey)
  } else {
    emit('open', photo)
  }
}

function at(x: number, y: number) {
  return `translate(${x + paddingX.value}px, ${y + PADDING_TOP}px)`
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
      // A month keeps the height of the rows it will have, left empty: grey rows
      // drawn there jumped when the photos replaced them with rows of their own
      // shapes (removed on 2026-11-18). A whole list shows
      // its status.
      estimate: s.section.month !== null && s.section.photos.length < s.section.count,
      style: { transform: at(0, s.y + s.pendingY), width: `${inner.value}px`, height: `${s.pendingHeight}px` },
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
  const s = sectionAt(scrollTop.value - PADDING_TOP)
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
        <div v-for="item in pending" :key="item.section.key" class="pending" :style="item.style">
          <div v-if="item.section.failed" class="status">
            <span>{{ t('grid.loadFailed') }}</span>
            <v-btn :text="t('common.retry')" variant="text" color="primary" @click="emit('retry', item.section.key)" />
          </div>
          <div v-else-if="!item.estimate" class="status">
            <v-progress-circular indeterminate size="24" width="2" :aria-label="t('grid.loading')" />
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
            <v-tooltip v-else :text="t('grid.failed', { name: photo.name })" location="top">
              <template #activator="{ props: tooltip }">
                <v-icon v-bind="tooltip" icon="fa-triangle-exclamation" data-testid="tile-failed" />
              </template>
            </v-tooltip>
            <span v-if="isVideo(photo) || photo.hasMotion || photo.hasRaw" class="badges">
              <span
                v-if="isVideo(photo)"
                class="badge"
                data-testid="tile-video"
                :title="t('grid.video', { duration: formatDuration(photo.duration ?? 0) })"
              >
                <v-icon icon="fa-play" size="small" />
                <span v-if="photo.duration !== null">{{ formatDuration(photo.duration) }}</span>
              </span>
              <template v-else>
                <span v-if="photo.hasRaw" class="badge raw" data-testid="tile-raw" :title="t('grid.raw')">RAW</span>
                <span v-if="photo.hasMotion" class="badge" data-testid="tile-motion" :title="t('grid.motion')">
                  <v-icon icon="fa-regular-circle-dot" size="small" />
                </span>
              </template>
            </span>
          </button>
          <button
            v-if="selectable(photo)"
            type="button"
            class="check"
            data-testid="tile-check"
            role="checkbox"
            :aria-checked="selected.has(photo.id)"
            :aria-label="t('grid.select', { name: photo.name })"
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
  padding-bottom: 8px;
  font-size: 1rem;
  white-space: nowrap;
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
  /* PatternFly's small radius, as on buttons and fields. */
  border-radius: var(--radius-small);
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
  border-radius: inherit;
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
  border-radius: inherit;
  object-fit: cover;
  transition: transform 0.1s;
}

/* A video's length, or the marks of a RAW and a motion photo, in the corner
   a player puts them. */
.badges {
  position: absolute;
  right: 4px;
  bottom: 4px;
  display: flex;
  gap: 4px;
  pointer-events: none;
}

.badge {
  display: flex;
  align-items: center;
  gap: 4px;
  padding: 2px 6px;
  border-radius: 999px;
  background: rgba(0, 0, 0, 0.5);
  color: #fff;
  font-size: 0.75rem;
  font-variant-numeric: tabular-nums;
  pointer-events: none;
}

.badge.raw {
  font-weight: 600;
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
