<script setup lang="ts">
import { computed, onBeforeUnmount, ref, watch } from 'vue'

// A strip on the right of the timeline with the years. Dragging its handle
// scrolls through the whole library, months not loaded yet included, and a
// bubble names the month under the handle. On a touch screen it shows while
// the list scrolls and hides soon after; with a mouse it stays.
const props = defineProps<{
  // The height of the list, and of the part on the screen.
  height: number
  viewport: number
  scrollTop: number
  // The first month of each year: its place in the list and its label.
  marks: { y: number; label: string }[]
  labelAt: (y: number) => string
}>()
const emit = defineEmits<{ seek: [top: number] }>()

const PAD = 8
const HANDLE = 40
// Labels closer than this to the one above are left out.
const LABEL_SPACING = 20
const HIDE_AFTER = 1500

const track = computed(() => Math.max(1, props.viewport - PAD * 2 - HANDLE))
const range = computed(() => Math.max(1, props.height - props.viewport))

// The handle stands for a reading line on the screen, which moves from the
// top edge at the start of the list to the bottom edge at its end. A share of
// the track is the same share of the whole list's height, so the year marks
// spread over the track and the last screen's months get their own places.
const share = computed(() => Math.min(1, Math.max(0, props.scrollTop / range.value)))
const readingLine = computed(() => Math.min(props.height - 1, share.value * props.height))

function toTrack(ratio: number) {
  return PAD + Math.min(1, Math.max(0, ratio)) * track.value
}

const handleTop = computed(() => toTrack(share.value))

const labels = computed(() => {
  const shown: { top: number; label: string }[] = []
  for (const mark of props.marks) {
    const top = toTrack(mark.y / props.height) + HANDLE / 2
    if (shown.length === 0 || top - shown.at(-1)!.top >= LABEL_SPACING) {
      shown.push({ top, label: mark.label })
    }
  }
  return shown
})

const dragging = ref(false)
const recent = ref(false)
let hideTimer: ReturnType<typeof setTimeout> | undefined

watch(
  () => props.scrollTop,
  () => {
    recent.value = true
    clearTimeout(hideTimer)
    hideTimer = setTimeout(() => (recent.value = false), HIDE_AFTER)
  },
)

onBeforeUnmount(() => clearTimeout(hideTimer))

const bubble = computed(() => (dragging.value ? props.labelAt(readingLine.value) : ''))

const strip = ref<HTMLElement>()

function seekTo(event: PointerEvent) {
  const rect = strip.value!.getBoundingClientRect()
  const ratio = (event.clientY - rect.top - PAD - HANDLE / 2) / track.value
  emit('seek', Math.min(1, Math.max(0, ratio)) * range.value)
}

function onPointerDown(event: PointerEvent) {
  dragging.value = true
  strip.value!.setPointerCapture(event.pointerId)
  seekTo(event)
  event.preventDefault()
}

function onPointerMove(event: PointerEvent) {
  if (dragging.value) {
    seekTo(event)
  }
}

function onPointerUp() {
  dragging.value = false
}
</script>

<template>
  <div
    ref="strip"
    class="scrubber"
    :class="{ shown: recent || dragging, dragging }"
    data-testid="scrubber"
    @pointerdown="onPointerDown"
    @pointermove="onPointerMove"
    @pointerup="onPointerUp"
    @pointercancel="onPointerUp"
  >
    <div v-for="label in labels" :key="label.label" class="year" :style="{ top: `${label.top}px` }">{{ label.label }}</div>
    <div class="handle" :style="{ top: `${handleTop}px`, height: `${HANDLE}px` }">
      <div v-if="bubble" class="bubble">{{ bubble }}</div>
    </div>
  </div>
</template>

<style scoped>
.scrubber {
  position: absolute;
  top: 0;
  right: 0;
  bottom: 0;
  /* As narrow as a scroll bar, so that it does not cover the photos' clicks.
     The years and the bubble stick out to the left without catching them. */
  width: 24px;
  touch-action: none;
  user-select: none;
  /* On a touch screen the hidden strip lets taps through to the photos. */
  pointer-events: none;
  opacity: 0;
  transition: opacity 0.2s;
}

.scrubber.shown {
  pointer-events: auto;
  opacity: 1;
}

.year {
  position: absolute;
  right: 20px;
  pointer-events: none;
  padding: 0 6px;
  border-radius: 10px;
  background: rgba(var(--v-theme-surface), 0.85);
  font-size: 0.75rem;
  line-height: 20px;
  transform: translateY(-50%);
  opacity: 0;
  transition: opacity 0.2s;
}

.handle {
  position: absolute;
  right: 4px;
  width: 8px;
  border-radius: 4px;
  background: rgb(var(--v-theme-primary));
  cursor: grab;
}

.dragging .handle {
  cursor: grabbing;
}

.dragging .year,
.scrubber:hover .year {
  opacity: 1;
}

.bubble {
  position: absolute;
  top: 50%;
  right: 16px;
  pointer-events: none;
  padding: 4px 12px;
  border-radius: 16px;
  background: rgb(var(--v-theme-primary));
  color: rgb(var(--v-theme-on-primary));
  font-size: 0.875rem;
  white-space: nowrap;
  transform: translateY(-50%);
}

/* With a mouse the handle stays, and the years show on hover. */
@media (hover: hover) {
  .scrubber {
    pointer-events: auto;
    opacity: 1;
  }

  .handle {
    opacity: 0.6;
  }

  .scrubber:hover .handle,
  .dragging .handle {
    opacity: 1;
  }
}

/* On a touch screen the strip is wider for a finger, and the years show with it. */
@media (hover: none) {
  .scrubber {
    width: 40px;
  }

  .shown .year {
    opacity: 1;
  }
}
</style>
