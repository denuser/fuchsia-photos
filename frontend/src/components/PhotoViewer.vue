<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref, watch } from 'vue'

import { originalUrl, previewUrl, type Camera, type Photo, type ViewerField } from '../api/client'
import { useAuthStore } from '../stores/auth'
import { useUsersStore } from '../stores/users'

// paused leaves the keyboard to a dialog opened over the viewer.
const props = defineProps<{ photos: Photo[]; photo: Photo; canTag: boolean; canManage: boolean; paused: boolean }>()
const emit = defineEmits<{ close: []; navigate: [photo: Photo]; trash: [photo: Photo]; tags: [photo: Photo]; share: [photo: Photo] }>()

const auth = useAuthStore()
const users = useUsersStore()

const ready = computed(() => props.photos.filter((p) => p.status === 'ready'))
const index = computed(() => ready.value.findIndex((p) => p.id === props.photo.id))
const previous = computed(() => ready.value[index.value - 1])
const next = computed(() => ready.value[index.value + 1])

const details = computed(() => {
  const date = new Date(props.photo.takenAt ?? props.photo.createdAt)
  const owner = props.photo.ownerId === auth.user?.id ? null : users.login(props.photo.ownerId)
  return [owner && `Shared by ${owner}`, date.toLocaleString(), props.photo.width && `${props.photo.width} × ${props.photo.height}`]
    .filter(Boolean)
    .join(' · ')
})

const hidden = computed(() => new Set<ViewerField>(auth.user?.viewerHiddenFields ?? []))

const camera = computed(() => (props.photo.camera ? describeCamera(props.photo.camera) : ''))

const location = computed(() => {
  const l = props.photo.location
  if (!l || hidden.value.has('location')) {
    return null
  }
  const lat = l.latitude.toFixed(5)
  const lon = l.longitude.toFixed(5)
  return { text: `${lat}, ${lon}`, url: `https://www.openstreetmap.org/?mlat=${lat}&mlon=${lon}#map=15/${lat}/${lon}` }
})

// Formats the shooting details the way cameras show them, such as
// "Canon EOS R5 · 35 mm · ƒ/2.8 · 1/250 s · ISO 100", leaving out the fields
// the user hid in the profile.
function describeCamera(c: Camera) {
  const parts: [ViewerField, unknown][] = [
    ['camera', cameraName(c)],
    ['lens', c.lens],
    ['focalLength', c.focalLength && focalLength(c.focalLength, c.focalLength35mm)],
    ['aperture', c.fNumber && `ƒ/${round(c.fNumber)}`],
    ['exposureTime', c.exposureTime && exposureTime(c.exposureTime)],
    ['iso', c.iso && `ISO ${c.iso}`],
    ['exposureCompensation', c.exposureCompensation && `${c.exposureCompensation > 0 ? '+' : '−'}${round(Math.abs(c.exposureCompensation))} EV`],
    ['flash', c.flash && 'Flash'],
  ]
  return parts
    .filter(([field, text]) => text && !hidden.value.has(field))
    .map(([, text]) => text)
    .join(' · ')
}

// Many cameras repeat the maker in the model, as in "Canon" and "Canon EOS R5".
function cameraName(c: Camera) {
  if (!c.make || !c.model) {
    return c.model ?? c.make
  }
  const maker = c.make.split(' ')[0].toLowerCase()
  return c.model.toLowerCase().startsWith(maker) ? c.model : `${c.make} ${c.model}`
}

function focalLength(mm: number, mm35?: number) {
  const text = `${round(mm)} mm`
  return mm35 && Math.abs(mm35 - mm) >= 1 ? `${text} (${Math.round(mm35)} mm equiv.)` : text
}

// Short exposures are fractions such as 1/250, longer ones decimals such as 0.8 or 2.
function exposureTime(seconds: number) {
  const inverse = 1 / seconds
  return seconds < 1 && Math.abs(inverse - Math.round(inverse)) < 0.05 ? `1/${Math.round(inverse)} s` : `${round(seconds)} s`
}

function round(n: number) {
  return String(Math.round(n * 10) / 10)
}

// Chosen in the profile: whether zooming in loads the original. Otherwise the
// viewer zooms the preview.
const showOriginals = computed(() => auth.user?.viewerShowsOriginals ?? false)

// Formats the browser failed to decode, such as HEIC outside Safari. Their
// originals are not downloaded again, and the viewer keeps the preview.
const undecodable = new Set<string>()

function canShowOriginal(photo: Photo) {
  return !undecodable.has(photo.mime)
}

// The original replaces the preview only once it has been decoded, so the
// preview stays on the screen while a large file loads.
const originalShown = ref(false)
let originalRequested = 0

function loadOriginal() {
  const photo = props.photo
  if (originalShown.value || originalRequested === photo.id || !canShowOriginal(photo)) {
    return
  }
  originalRequested = photo.id
  const image = new Image()
  image.src = originalUrl(photo.id)
  image.decode().then(
    () => {
      if (props.photo.id === photo.id) {
        originalShown.value = true
      }
    },
    () => {
      undecodable.add(photo.mime)
    },
  )
}

const source = computed(() => (originalShown.value ? originalUrl(props.photo.id) : previewUrl(props.photo.id, 'viewer')))

// Loads the neighbours ahead of time, so that stepping through photos feels instant.
watch(
  [previous, next],
  (neighbours) => {
    for (const photo of neighbours) {
      if (photo) {
        new Image().src = previewUrl(photo.id, 'viewer')
      }
    }
  },
  { immediate: true },
)

// The image is laid out at the size that fits the screen, and a transform
// scales it up to the photo's own resolution, one image pixel per CSS pixel.
const stage = ref<HTMLElement | null>(null)
const image = ref<HTMLImageElement | null>(null)
const stageSize = ref({ width: 0, height: 0 })
const naturalSize = ref<{ width: number; height: number } | null>(null)

const photoSize = computed(() =>
  props.photo.width && props.photo.height ? { width: props.photo.width, height: props.photo.height } : naturalSize.value,
)

const fit = computed(() => {
  const size = photoSize.value
  if (!size || stageSize.value.width === 0) {
    return null
  }
  const ratio = Math.min(1, stageSize.value.width / size.width, stageSize.value.height / size.height)
  return { width: size.width * ratio, height: size.height * ratio, maxScale: 1 / ratio }
})

// The scale over the fitting size, and the offset of the photo's centre from
// the centre of the screen, in screen pixels.
type View = { scale: number; x: number; y: number }
type Point = { x: number; y: number }

const view = ref<View>({ scale: 1, x: 0, y: 0 })
// Set for a zoom by a click or a key, which animates; gestures follow the
// fingers directly.
const animated = ref(false)

const zoomable = computed(() => (fit.value?.maxScale ?? 1) > 1.01)
const zoomed = computed(() => view.value.scale > 1.001)

const transform = computed(() => {
  const { scale, x, y } = view.value
  return scale === 1 && x === 0 && y === 0 ? undefined : `translate(${x}px, ${y}px) scale(${scale})`
})

function clampScale(scale: number) {
  return Math.min(Math.max(scale, 1), fit.value?.maxScale ?? 1)
}

// A photo larger than the screen can move until its edge meets the edge of the
// screen; a smaller one stays in the middle.
function setView(scale: number, x: number, y: number, animate = false) {
  const f = fit.value
  if (!f) {
    return
  }
  scale = clampScale(scale)
  const spareX = Math.max(0, (f.width * scale - stageSize.value.width) / 2)
  const spareY = Math.max(0, (f.height * scale - stageSize.value.height) / 2)
  view.value = { scale, x: Math.min(Math.max(x, -spareX), spareX), y: Math.min(Math.max(y, -spareY), spareY) }
  animated.value = animate
}

function resetView(animate = false) {
  setView(1, 0, 0, animate)
}

// Scales the photo from the view start, so that the point of the photo that
// was under the screen point from comes under the screen point to. With the
// same point, as for a click or the wheel, that point stays in place.
function zoomAround(scale: number, start: View, from: Point, to: Point, animate = false) {
  const rect = stage.value?.getBoundingClientRect()
  if (!rect) {
    return
  }
  const cx = rect.left + rect.width / 2
  const cy = rect.top + rect.height / 2
  const k = clampScale(scale) / start.scale
  setView(scale, to.x - cx - k * (from.x - cx - start.x), to.y - cy - k * (from.y - cy - start.y), animate)
}

// Zoomed out for each photo and each screen size.
watch([() => props.photo.id, () => fit.value?.maxScale, () => stageSize.value.width, () => stageSize.value.height], () => {
  view.value = { scale: 1, x: 0, y: 0 }
  animated.value = false
})

watch(
  () => props.photo.id,
  () => {
    originalShown.value = false
    naturalSize.value = null
  },
)

watch([() => props.photo.id, showOriginals, zoomed], () => {
  if (showOriginals.value && zoomed.value) {
    loadOriginal()
  }
}, { immediate: true })

function measure() {
  if (stage.value) {
    stageSize.value = { width: stage.value.clientWidth, height: stage.value.clientHeight }
  }
}

function onImageLoad() {
  if (image.value && !naturalSize.value) {
    naturalSize.value = { width: image.value.naturalWidth, height: image.value.naturalHeight }
  }
}

// A wheel notch zooms by about a tenth; a touchpad pinch, which comes as
// wheel events with small deltas, zooms smoothly.
function onWheel(event: WheelEvent) {
  if (!zoomable.value) {
    return
  }
  const delta = (event.deltaY || event.deltaX) * (event.deltaMode === WheelEvent.DOM_DELTA_LINE ? 16 : 1)
  const point = { x: event.clientX, y: event.clientY }
  const start = view.value
  zoomAround(start.scale * Math.exp(-Math.max(-100, Math.min(100, delta)) / 1000), start, point, point)
}

// Pointers down on the stage, by id. Moves and releases are taken from the
// window, so that a drag continues over the bars or outside the window.
const pointers = new Map<number, Point>()
let downAt: Point = { x: 0, y: 0 }

function middle(a: Point, b: Point) {
  return { x: (a.x + b.x) / 2, y: (a.y + b.y) / 2 }
}

// A horizontal swipe with one finger steps to the neighbouring photo, unless
// the photo is zoomed in, when the finger moves the photo instead. Two fingers
// zoom: the scale follows the distance between them, and the point between
// them stays under them. The mouse moves a zoomed photo too.
//
// For a swipe the photos lie in a row, and the row follows the finger, so the
// neighbour comes in from the side. On release the row slides either back or
// on to the neighbour, and only once the neighbour is in place does it become
// the viewer's photo.
type Gesture =
  | { kind: 'swipe' }
  | { kind: 'pan'; start: View; from: Point }
  | { kind: 'pinch'; start: View; from: Point; distance: number }

const SWIPE_DISTANCE = 50
const SLIDE_GAP = 16
let gesture: Gesture | null = null
const swipeOffset = ref(0)
const sliding = ref(false)
let slideTarget: Photo | null = null
let slideFallback = 0

// Starts the gesture that the pointers now down make, such as a pan with the
// finger left after a pinch.
function startGesture(swipe: boolean) {
  const [a, b] = pointers.values()
  if (gesture?.kind === 'swipe') {
    slideTo(0, null)
  }
  gesture = null
  if (a && b && zoomable.value && !sliding.value) {
    gesture = { kind: 'pinch', start: view.value, from: middle(a, b), distance: Math.max(1, Math.hypot(a.x - b.x, a.y - b.y)) }
  } else if (a && !b && zoomed.value) {
    gesture = { kind: 'pan', start: view.value, from: a }
  } else if (a && !b && swipe && !sliding.value) {
    gesture = { kind: 'swipe' }
  }
  if (gesture && gesture.kind !== 'swipe') {
    animated.value = false
  }
}

function onPointerDown(event: PointerEvent) {
  const point = { x: event.clientX, y: event.clientY }
  pointers.set(event.pointerId, point)
  if (pointers.size === 1) {
    downAt = point
  }
  startGesture(event.pointerType !== 'mouse')
}

function onPointerMove(event: PointerEvent) {
  if (!pointers.has(event.pointerId)) {
    return
  }
  pointers.set(event.pointerId, { x: event.clientX, y: event.clientY })
  const [a, b] = pointers.values()
  if (gesture?.kind === 'pinch' && b) {
    const distance = Math.hypot(a.x - b.x, a.y - b.y)
    zoomAround((gesture.start.scale * distance) / gesture.distance, gesture.start, gesture.from, middle(a, b))
  } else if (gesture?.kind === 'pan') {
    const { start, from } = gesture
    setView(start.scale, start.x + a.x - from.x, start.y + a.y - from.y)
  } else if (gesture?.kind === 'swipe') {
    const dx = event.clientX - downAt.x
    // Past the first or the last photo, the photo moves only a little.
    swipeOffset.value = (dx > 0 ? previous.value : next.value) ? dx : dx / 4
  }
}

function onPointerUp(event: PointerEvent) {
  if (!pointers.delete(event.pointerId)) {
    return
  }
  if (gesture?.kind !== 'swipe') {
    startGesture(false)
    return
  }
  gesture = null
  const dx = event.clientX - downAt.x
  const dy = event.clientY - downAt.y
  const target = dx > 0 ? previous.value : next.value
  if (event.type !== 'pointerup' || Math.abs(dx) < SWIPE_DISTANCE || Math.abs(dx) < Math.abs(dy) || !target) {
    slideTo(0, null)
    return
  }
  slideTo(Math.sign(dx) * (stageSize.value.width + SLIDE_GAP), target)
}

function slideTo(offset: number, target: Photo | null) {
  if (swipeOffset.value === offset) {
    finishSlide(target)
    return
  }
  slideTarget = target
  sliding.value = true
  swipeOffset.value = offset
  // In case transitionend never comes, such as in a hidden tab.
  clearTimeout(slideFallback)
  slideFallback = window.setTimeout(() => finishSlide(slideTarget), 500)
}

function onSlideEnd(event: TransitionEvent) {
  if (sliding.value && event.target === event.currentTarget && event.propertyName === 'transform') {
    finishSlide(slideTarget)
  }
}

function finishSlide(target: Photo | null) {
  clearTimeout(slideFallback)
  slideTarget = null
  if (target) {
    // The row jumps back to the middle in the same update that makes the
    // neighbour the viewer's photo, see the watch on the photo below.
    emit('navigate', target)
  } else {
    sliding.value = false
    swipeOffset.value = 0
  }
}

watch(
  () => props.photo.id,
  () => {
    clearTimeout(slideFallback)
    slideTarget = null
    sliding.value = false
    swipeOffset.value = 0
  },
)

onBeforeUnmount(() => clearTimeout(slideFallback))

// A click zooms to the photo's own resolution at the clicked point, or back.
// Dragging the photo also ends in a click, which must not zoom.
function onClick(event: MouseEvent) {
  if (!zoomable.value || Math.hypot(event.clientX - downAt.x, event.clientY - downAt.y) > 5) {
    return
  }
  if (zoomed.value) {
    resetView(true)
  } else if (event.target === image.value) {
    const point = { x: event.clientX, y: event.clientY }
    zoomAround(fit.value!.maxScale, view.value, point, point, true)
  }
}

function onKey(event: KeyboardEvent) {
  if (props.paused) {
    return
  }
  if (event.key === 'Escape' && zoomed.value) {
    resetView(true)
  } else if (event.key === 'Escape') {
    emit('close')
  } else if (event.key === 'ArrowLeft' && previous.value) {
    emit('navigate', previous.value)
  } else if (event.key === 'ArrowRight' && next.value) {
    emit('navigate', next.value)
  }
}

onMounted(() => {
  measure()
  window.addEventListener('keydown', onKey)
  window.addEventListener('resize', measure)
  window.addEventListener('pointermove', onPointerMove)
  window.addEventListener('pointerup', onPointerUp)
  window.addEventListener('pointercancel', onPointerUp)
})
onBeforeUnmount(() => {
  window.removeEventListener('keydown', onKey)
  window.removeEventListener('resize', measure)
  window.removeEventListener('pointermove', onPointerMove)
  window.removeEventListener('pointerup', onPointerUp)
  window.removeEventListener('pointercancel', onPointerUp)
})
</script>

<template>
  <div class="viewer" data-testid="viewer" role="dialog" aria-modal="true" :aria-label="photo.name">
    <div
      ref="stage"
      class="stage"
      :class="{ zoomable, zoomed }"
      @pointerdown="onPointerDown"
      @click="onClick"
      @wheel.prevent="onWheel"
    >
      <div
        class="track"
        :class="{ sliding }"
        :style="swipeOffset ? { transform: `translateX(${swipeOffset}px)` } : undefined"
        @transitionend="onSlideEnd"
      >
        <div v-if="previous" :key="previous.id" class="slide neighbour" :style="{ left: `calc(-100% - ${SLIDE_GAP}px)` }">
          <img :src="previewUrl(previous.id, 'viewer')" alt="" class="image" draggable="false" />
        </div>
        <div class="slide">
          <img
            ref="image"
            :key="photo.id"
            :src="source"
            :alt="photo.name"
            :data-source="originalShown ? 'original' : 'preview'"
            :style="fit ? { width: `${fit.width}px`, height: `${fit.height}px`, transform } : undefined"
            :class="{ animated }"
            class="image"
            data-testid="viewer-image"
            draggable="false"
            @load="onImageLoad"
          />
        </div>
        <div v-if="next" :key="next.id" class="slide neighbour" :style="{ left: `calc(100% + ${SLIDE_GAP}px)` }">
          <img :src="previewUrl(next.id, 'viewer')" alt="" class="image" draggable="false" />
        </div>
      </div>
    </div>

    <div class="bar">
      <div class="info">
        <div class="name">{{ photo.name }}</div>
        <div class="details">{{ details }}</div>
      </div>
      <v-btn v-if="canTag" icon="mdi-tag-outline" variant="text" aria-label="Tags" @click="emit('tags', photo)" />
      <template v-if="canManage">
        <v-btn icon="mdi-share-variant-outline" variant="text" aria-label="Share" @click="emit('share', photo)" />
        <v-btn icon="mdi-trash-can-outline" variant="text" aria-label="Move to trash" @click="emit('trash', photo)" />
      </template>
      <v-btn :href="originalUrl(photo.id)" icon="mdi-download" variant="text" aria-label="Download the original" />
      <v-btn icon="mdi-close" variant="text" aria-label="Close" @click="emit('close')" />
    </div>

    <div v-if="camera || location" class="bar bottom">
      <div class="details" data-testid="camera">
        {{ camera }}<template v-if="camera && location"> · </template>
        <a v-if="location" :href="location.url" target="_blank" rel="noopener noreferrer" class="location">{{
          location.text
        }}</a>
      </div>
    </div>

    <v-btn
      v-if="previous"
      class="nav left"
      icon="mdi-chevron-left"
      variant="text"
      size="x-large"
      aria-label="Previous photo"
      @click="emit('navigate', previous)"
    />
    <v-btn
      v-if="next"
      class="nav right"
      icon="mdi-chevron-right"
      variant="text"
      size="x-large"
      aria-label="Next photo"
      @click="emit('navigate', next)"
    />
  </div>
</template>

<style scoped>
.viewer {
  position: fixed;
  inset: 0;
  z-index: 1200;
  display: flex;
  align-items: center;
  justify-content: center;
  background: #000;
  color: #fff;
}

.stage {
  position: absolute;
  inset: 0;
  display: flex;
  align-items: center;
  justify-content: center;
  overflow: hidden;
  /* Swipes and pinches are the viewer's, not the browser's. */
  touch-action: none;
}

/* The photo and its neighbours in a row, moved by a swipe. The row follows
   the finger directly and animates only when it slides on release. */
.track {
  position: absolute;
  inset: 0;
}

.track.sliding {
  transition: transform 0.25s ease-out;
}

.slide {
  position: absolute;
  top: 0;
  left: 0;
  width: 100%;
  height: 100%;
  display: flex;
  align-items: center;
  justify-content: center;
}

.neighbour {
  pointer-events: none;
}

.neighbour .image {
  width: 100%;
  height: 100%;
}

.image.animated {
  transition: transform 0.25s ease-out;
}

.stage.zoomable {
  cursor: zoom-in;
}

.stage.zoomed {
  cursor: grab;
}

.stage.zoomed:active {
  cursor: grabbing;
}

/* Until the size is known, the image fits the screen the plain way. */
.image {
  flex: none;
  max-width: 100%;
  max-height: 100%;
  object-fit: contain;
  user-select: none;
}

.bar {
  position: absolute;
  top: 0;
  right: 0;
  left: 0;
  display: flex;
  align-items: center;
  padding: 8px 8px 24px 16px;
  background: linear-gradient(rgba(0, 0, 0, 0.7), transparent);
}

.bar.bottom {
  top: auto;
  bottom: 0;
  display: block;
  padding: 24px 16px 12px;
  background: linear-gradient(transparent, rgba(0, 0, 0, 0.7));
}

.info {
  flex: 1;
  min-width: 0;
}

/* On a narrow screen the buttons take a row of their own above the name and
   the date, so that neither squeezes the other. */
@media (max-width: 599.98px) {
  .bar {
    flex-wrap: wrap;
    justify-content: flex-end;
    padding-left: 8px;
  }

  .info {
    flex-basis: 100%;
    order: 1;
    padding: 0 8px;
  }
}

.name {
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.details {
  font-size: 0.8rem;
  opacity: 0.75;
}

.location {
  color: inherit;
}

.nav {
  position: absolute;
  top: 50%;
  transform: translateY(-50%);
}

.left {
  left: 8px;
}

.right {
  right: 8px;
}
</style>
