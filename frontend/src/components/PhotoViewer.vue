<script setup lang="ts">
import Panzoom, { type PanzoomObject } from '@panzoom/panzoom'
import { computed, onBeforeUnmount, onMounted, ref, shallowRef, watch } from 'vue'

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

// Chosen in the profile.
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
        if (showOriginals.value && canShowOriginal(photo)) {
          new Image().src = originalUrl(photo.id)
        }
      }
    }
  },
  { immediate: true },
)

// The image is laid out at the size that fits the screen, and Panzoom scales
// it up to the photo's own resolution, one image pixel per CSS pixel.
const stage = ref<HTMLElement | null>(null)
const image = ref<HTMLImageElement | null>(null)
const stageSize = ref({ width: 0, height: 0 })
const naturalSize = ref<{ width: number; height: number } | null>(null)
const panzoom = shallowRef<PanzoomObject | null>(null)
const scale = ref(1)

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

const zoomable = computed(() => (fit.value?.maxScale ?? 1) > 1.01)
const zoomed = computed(() => scale.value > 1.001)

watch(
  () => props.photo.id,
  () => {
    originalShown.value = false
    naturalSize.value = null
    scale.value = 1
  },
)

watch([() => props.photo.id, showOriginals, zoomed], () => {
  if (showOriginals.value || zoomed.value) {
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

function onScale(event: Event) {
  scale.value = (event as CustomEvent<{ scale: number }>).detail.scale
}

// Recreates Panzoom for each photo and each screen size, zoomed out.
watch(
  [image, () => fit.value?.maxScale],
  ([element, maxScale], _, onCleanup) => {
    scale.value = 1
    if (!element || !maxScale || maxScale <= 1.01) {
      return
    }
    const instance = Panzoom(element, {
      canvas: true,
      maxScale,
      minScale: 1,
      startScale: 1,
      panOnlyWhenZoomed: true,
      cursor: '',
      step: 0.3,
    })
    element.addEventListener('panzoomchange', onScale)
    panzoom.value = instance
    onCleanup(() => {
      element.removeEventListener('panzoomchange', onScale)
      instance.destroy()
      instance.resetStyle()
      panzoom.value = null
    })
  },
  { flush: 'post' },
)

function onWheel(event: WheelEvent) {
  if (panzoom.value) {
    panzoom.value.zoomWithWheel(event)
    // Zoomed all the way out, the photo goes back to the centre.
    if (panzoom.value.getScale() <= 1.001) {
      panzoom.value.reset({ animate: false })
    }
  }
}

// A click zooms to the photo's own resolution at the clicked point, or back.
// Dragging the photo also ends in a click, which must not zoom.
// The stage takes pointerdown while it captures, because Panzoom stops it on
// the photo's slide.
let downAt = { x: 0, y: 0 }

// A horizontal swipe with one finger steps to the neighbouring photo, unless
// the photo is zoomed in, when the finger moves the photo instead. The photos
// lie in a row, and the row follows the finger, so the neighbour comes in from
// the side. A second finger, for a pinch, ends the swipe.
//
// On release the row slides either back or on to the neighbour, and only once
// the neighbour is in place does it become the viewer's photo.
const SWIPE_DISTANCE = 50
const SLIDE_GAP = 16
const pointers = new Set<number>()
let swiping = false
const swipeOffset = ref(0)
const sliding = ref(false)
let slideTarget: Photo | null = null
let slideFallback = 0

function onPointerDown(event: PointerEvent) {
  downAt = { x: event.clientX, y: event.clientY }
  pointers.add(event.pointerId)
  swiping = event.pointerType !== 'mouse' && pointers.size === 1 && !zoomed.value && !sliding.value
}

function onPointerMove(event: PointerEvent) {
  if (!swiping || !pointers.has(event.pointerId)) {
    return
  }
  if (pointers.size > 1 || zoomed.value) {
    swiping = false
    slideTo(0, null)
    return
  }
  const dx = event.clientX - downAt.x
  // Past the first or the last photo, the photo moves only a little.
  swipeOffset.value = (dx > 0 ? previous.value : next.value) ? dx : dx / 4
}

function onPointerUp(event: PointerEvent) {
  pointers.delete(event.pointerId)
  if (!swiping) {
    return
  }
  swiping = false
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

function onClick(event: MouseEvent) {
  const instance = panzoom.value
  if (!instance || !fit.value || Math.hypot(event.clientX - downAt.x, event.clientY - downAt.y) > 5) {
    return
  }
  if (zoomed.value) {
    instance.reset()
  } else if (event.target === image.value) {
    instance.zoomToPoint(fit.value.maxScale, event)
  }
}

function onKey(event: KeyboardEvent) {
  if (props.paused) {
    return
  }
  if (event.key === 'Escape' && zoomed.value) {
    panzoom.value?.reset()
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
})
onBeforeUnmount(() => {
  window.removeEventListener('keydown', onKey)
  window.removeEventListener('resize', measure)
})
</script>

<template>
  <div class="viewer" data-testid="viewer" role="dialog" aria-modal="true" :aria-label="photo.name">
    <div
      ref="stage"
      class="stage"
      :class="{ zoomable, zoomed }"
      @pointerdown.capture="onPointerDown"
      @pointermove="onPointerMove"
      @pointerup="onPointerUp"
      @pointercancel="onPointerUp"
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
            :style="fit ? { width: `${fit.width}px`, height: `${fit.height}px` } : undefined"
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
