<script setup lang="ts">
import { computed, nextTick, onBeforeUnmount, onMounted, reactive, ref, watch } from 'vue'
import { useDisplay } from 'vuetify'

import { api, copyUrl, isVideo, motionUrl, originalUrl, previewUrl, videoUrl, type Camera, type Photo, type ViewerField } from '../api/client'
import { formatDateTime, intlLocale, t } from '../i18n'
import { apiError } from '../i18n/apiErrors'
import { formatDuration } from '../lib/duration'
import { rawFormats } from '../lib/rawFormats'
import { useAuthStore } from '../stores/auth'
import { usePhotosStore } from '../stores/photos'
import AppDialog from './AppDialog.vue'
import AppFormGroup from './AppFormGroup.vue'
import { useUsersStore } from '../stores/users'

// paused leaves the keyboard to a dialog opened over the viewer.
const props = defineProps<{
  photos: Photo[]
  photo: Photo
  canTag: boolean
  canManage: boolean
  // Whether the viewer offers to find photos that look like this one.
  canFindSimilar: boolean
  // Whether the viewer offers to archive the photo: not in the trash.
  canArchive: boolean
  paused: boolean
}>()
const emit = defineEmits<{
  close: []
  navigate: [photo: Photo]
  trash: [photo: Photo]
  archive: [photo: Photo, value: boolean, done: (ok: boolean) => void]
  tags: [photo: Photo]
  share: [photo: Photo]
  similar: [photo: Photo]
}>()


const auth = useAuthStore()
const users = useUsersStore()

// Versions can replace the image, details and download. Navigation and
// library actions still use the photo that opened the viewer.
const original = ref<Photo | null>(null)
const raw = ref<Photo | null>(null)
const selectedVersion = ref<'edited' | 'raw' | null>(null)
const shown = computed(() => (selectedVersion.value === 'raw' ? raw.value : selectedVersion.value === 'edited' ? original.value : null) ?? props.photo)

// Whether the photo is the user's favorite, which only the photo's own
// address tells. The star acts on the photo of the list, also while its
// original is shown.
const favorite = ref<boolean | null>(null)
// Whether the photo is in the user's archive, which the photo's own address
// tells too.
const archived = ref<boolean | null>(null)

watch(
  () => props.photo.id,
  async (id) => {
    selectedVersion.value = null
    original.value = null
    raw.value = null
    favorite.value = null
    archived.value = null
    const { data } = await api.GET('/photos/{id}', { params: { path: { id } } })
    if (!data || id !== props.photo.id) {
      return
    }
    favorite.value = data.favorite ?? null
    archived.value = data.archived ?? null
    const editedVersion = data.versions?.find((v) => v.kind === 'edited')
    if (editedVersion) {
      const { data: photo } = await api.GET('/photos/{id}', { params: { path: { id: editedVersion.id } } })
      if (id !== props.photo.id) {
        return
      }
      original.value = photo ?? null
    }
    const rawVersion = data.versions?.find((v) => v.kind === 'raw')
      ?? original.value?.versions?.find((v) => v.kind === 'raw')
    if (rawVersion) {
      const { data: photo } = await api.GET('/photos/{id}', { params: { path: { id: rawVersion.id } } })
      if (photo && id === props.photo.id) {
        raw.value = photo
      }
    }
  },
  { immediate: true },
)

// The description goes with the photo to everyone who sees it; its owner
// edits it in a dialog, which keeps the keyboard while it is open.
const MAX_DESCRIPTION = 2000
const photosStore = usePhotosStore()
const saved = reactive(new Map<number, string>())
const description = computed(() => saved.get(shown.value.id) ?? shown.value.description)
const canEditDescription = computed(() => shown.value.ownerId === auth.user?.id)
const descriptionOpen = ref(false)
const descriptionDraft = ref('')
const descriptionSaving = ref(false)
const descriptionError = ref('')

function editDescription() {
  descriptionDraft.value = description.value
  descriptionError.value = ''
  descriptionOpen.value = true
}

async function saveDescription() {
  const photo = shown.value
  descriptionSaving.value = true
  const { data, error } = await api.PATCH('/photos/{id}', {
    params: { path: { id: photo.id } },
    body: { description: descriptionDraft.value },
  })
  descriptionSaving.value = false
  if (!data) {
    descriptionError.value = apiError(error)
    return
  }
  saved.set(data.id, data.description)
  // A hidden original is not in the lists and must not join them.
  if (data.id === props.photo.id) {
    photosStore.merge([data])
  }
  descriptionOpen.value = false
}

// Actions used less often. On a phone they fold into a "⋮" menu, as the
// selection bar's do, so that the bar keeps one row; wider screens show
// them as buttons.
const { xs } = useDisplay()
// The menu takes Escape and the arrows while it is open.
const moreOpen = ref(false)
interface ViewerAction {
  key: string
  icon: string
  text: string
  click: () => void
  active?: boolean
}
const secondaryActions = computed<ViewerAction[]>(() => [
  ...(props.canFindSimilar ? [{ key: 'similar', icon: 'fa-clone', text: t('viewer.similar'), click: () => emit('similar', props.photo) }] : []),
  ...(original.value
    ? [
        {
          key: 'original',
          icon: 'fa-clock-rotate-left',
          text: selectedVersion.value === 'edited' ? t('viewer.showEdited') : t('viewer.showOriginal'),
          click: () => (selectedVersion.value = selectedVersion.value === 'edited' ? null : 'edited'),
          active: selectedVersion.value === 'edited',
        },
      ]
    : []),
  ...(raw.value
    ? [{
        key: 'raw',
        icon: 'fa-file-image',
        text: selectedVersion.value === 'raw' ? t('viewer.showPhoto') : t('viewer.showRaw'),
        click: () => (selectedVersion.value = selectedVersion.value === 'raw' ? null : 'raw'),
        active: selectedVersion.value === 'raw',
      }]
    : []),
  ...(canEditDescription.value ? [{ key: 'description', icon: 'fa-pen', text: t('viewer.editDescription'), click: editDescription }] : []),
  ...(props.canTag ? [{ key: 'tags', icon: 'fa-tag', text: t('browser.tags'), click: () => emit('tags', props.photo) }] : []),
  ...(archived.value !== null && props.canArchive
    ? [
        {
          key: 'archive',
          icon: archived.value ? 'fa-box-open' : 'fa-box-archive',
          text: archived.value ? t('browser.unarchive') : t('browser.archive'),
          click: toggleArchive,
        },
      ]
    : []),
  ...(props.canManage ? [{ key: 'share', icon: 'fa-share-nodes', text: t('browser.share'), click: () => emit('share', props.photo) }] : []),
])

// The mark sends no event, and the photo stays in the list until the page
// loads again, also on the Favorites page, so that the viewer keeps its place.
async function toggleFavorite() {
  const id = props.photo.id
  const value = !favorite.value
  const { response } = await api.POST('/photos/favorites', { body: { ids: [id], favorite: value } })
  if (response.ok && id === props.photo.id) {
    favorite.value = value
  }
}

function toggleArchive() {
  const id = props.photo.id
  const value = !archived.value
  emit('archive', props.photo, value, (ok) => {
    if (ok && id === props.photo.id) {
      archived.value = value
    }
  })
}

const ready = computed(() => props.photos.filter((p) => p.status === 'ready'))
const index = computed(() => ready.value.findIndex((p) => p.id === props.photo.id))
const previous = computed(() => ready.value[index.value - 1])
const next = computed(() => ready.value[index.value + 1])

const details = computed(() => {
  const date = new Date(shown.value.takenAt ?? shown.value.createdAt)
  const owner = shown.value.ownerId === auth.user?.id ? null : users.login(shown.value.ownerId)
  const duration = shown.value.duration !== null && formatDuration(shown.value.duration)
  return [selectedVersion.value === 'edited' && t('viewer.originalVersion'), rawFormats.get(shown.value.mime), owner && t('viewer.sharedBy', { owner }), formatDateTime(date), shown.value.width && `${shown.value.width} × ${shown.value.height}`, duration]
    .filter(Boolean)
    .join(' · ')
})

const hidden = computed(() => new Set<ViewerField>(auth.user?.viewerHiddenFields ?? []))

const camera = computed(() => (shown.value.camera ? describeCamera(shown.value.camera) : ''))

const location = computed(() => {
  const l = shown.value.location
  if (!l || hidden.value.has('location')) {
    return null
  }
  const lat = l.latitude.toFixed(5)
  const lon = l.longitude.toFixed(5)
  return { text: `${lat}, ${lon}`, to: { path: '/map', query: { lat, lon, zoom: '15' } } }
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
    ['flash', c.flash && t('viewer.flash')],
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
  const text = t('viewer.mm', { value: round(mm) })
  return mm35 && Math.abs(mm35 - mm) >= 1 ? t('viewer.equivalent', { focal: text, value: Math.round(mm35) }) : text
}

// Short exposures are fractions such as 1/250, longer ones decimals such as 0.8 or 2.
// Phones write exposures such as 0.002481 that are no exact fraction, and
// rounding them to a tenth of a second would show 0, so anything shorter than
// a quarter of a second is a fraction.
function exposureTime(seconds: number) {
  const inverse = 1 / seconds
  const fraction = seconds < 0.25 || (seconds < 1 && Math.abs(inverse - Math.round(inverse)) < 0.05)
  const value = fraction ? `1/${Math.round(inverse)}` : round(seconds)
  return t('viewer.seconds', { value })
}

// One decimal place, with the decimal separator of the interface language.
function round(n: number) {
  return (Math.round(n * 10) / 10).toLocaleString(intlLocale.value)
}

// Chosen in the profile: whether zooming in loads the original. Otherwise the
// viewer zooms the preview.
const showOriginals = computed(() => auth.user?.viewerShowsOriginals ?? false)

// Formats the browser failed to decode, such as HEIC outside Safari. Their
// originals are not downloaded again, and the viewer keeps the preview.
const undecodable = reactive(new Set<string>())

// The original of a video is no image, and can be gigabytes.
function canShowOriginal(photo: Photo) {
  return !isVideo(photo) && !rawFormats.has(photo.mime) && !undecodable.has(photo.mime)
}

// A video plays in place of the photo once the user starts it: stepping
// through a library must not download every video on the way. It plays its
// H.264 copy when there is one, and its original until then. When the browser
// cannot play the original, such as HEVC in some browsers, the viewer shows
// the poster and offers the download, and waits for the copy if one is coming.
const video = computed(() => isVideo(shown.value))
const videoElement = ref<HTMLVideoElement | null>(null)
const unplayable = ref(false)

// The state of the copy, which the viewer learns again while it waits.
const videoCopy = ref(shown.value.videoCopy)
const copyReady = computed(() => videoCopy.value === 'ready')
// What the video element plays, chosen when the item opens, so that a copy
// made meanwhile does not interrupt the original playing.
const videoSource = ref(playSource())
function playSource() {
  return shown.value.videoCopy === 'ready' ? copyUrl(shown.value.id) : videoUrl(shown.value.id)
}
// Whether a copy was asked for and is still being made.
const copyComing = ref(false)
let copyPoll = 0

// Asks for the copy once the user starts the video or the motion, which puts
// it before the other items waiting. Copies are made only where the
// graphics allow, so the answer may be that none is coming.
let copyAsked = 0
async function askForCopy(): Promise<boolean> {
  const id = shown.value.id
  if (videoCopy.value !== null || copyAsked === id) {
    return copyComing.value
  }
  copyAsked = id
  const { data } = await api.POST('/photos/{id}/copy', { params: { path: { id } } })
  if (id === shown.value.id && data?.queued) {
    copyComing.value = true
    pollCopy(id)
    return true
  }
  return false
}

// An HDR video plays grey in browsers that do not tone map it, such as
// Firefox, so its first play waits for the copy, which is tone mapped, rather
// than starting the original. When no copy is coming, the original plays.
const waitingForCopy = ref(false)

async function onVideoPlay() {
  const element = videoElement.value
  if (waitingForCopy.value) {
    element?.pause()
    return
  }
  const id = shown.value.id
  if (!shown.value.hdr || videoCopy.value !== null || copyAsked === id || element === null) {
    askForCopy()
    return
  }
  element.pause()
  waitingForCopy.value = true
  const coming = await askForCopy()
  if (!coming && waitingForCopy.value && id === shown.value.id) {
    stopWaitingForCopy()
  }
}

// Plays what the video element now holds: the copy, or the original when no
// copy came or the user chose not to wait, as when the transcoder stopped
// halfway or the copy of a long video takes minutes. A copy that arrives
// later does not interrupt the original. A browser may refuse to start it outside the user's click; the
// user then starts it.
function stopWaitingForCopy() {
  waitingForCopy.value = false
  nextTick(() => videoElement.value?.play().catch(() => {}))
}

function pollCopy(id: number) {
  clearTimeout(copyPoll)
  copyPoll = window.setTimeout(async () => {
    const { data } = await api.GET('/photos/{id}', { params: { path: { id } } })
    if (id !== shown.value.id) {
      return
    }
    if (data?.videoCopy == null) {
      pollCopy(id)
      return
    }
    copyArrived(data.videoCopy)
  }, 3000)
}

// Switches to the copy where the original could not play or has not started.
function copyArrived(state: NonNullable<Photo['videoCopy']>) {
  copyComing.value = false
  clearTimeout(copyPoll)
  videoCopy.value = state
  if (waitingForCopy.value) {
    if (state === 'ready') {
      videoSource.value = copyUrl(shown.value.id)
      // An original the browser could not decode, such as HEVC, may have
      // failed while the viewer waited.
      unplayable.value = false
    }
    stopWaitingForCopy()
    return
  }
  const element = videoElement.value
  const started = element !== null && (!element.paused || element.currentTime > 0)
  if (state === 'ready' && (unplayable.value || !started)) {
    videoSource.value = copyUrl(shown.value.id)
    unplayable.value = false
  }
}

function onVideoUnplayable() {
  // A copy made since the item opened plays instead.
  if (copyReady.value && videoSource.value !== copyUrl(shown.value.id)) {
    videoSource.value = copyUrl(shown.value.id)
    return
  }
  unplayable.value = true
  askForCopy()
}

// Some browsers play the sound of a video they cannot decode over a black
// picture, without an error; the picture's size then stays zero.
function onVideoMetadata(event: Event) {
  if ((event.target as HTMLVideoElement).videoWidth === 0) {
    onVideoUnplayable()
  }
}

// The motion of a motion photo or a Live Photo plays once over the photo, by the button, and
// the photo comes back at its end.
const motionPlaying = ref(false)
const motionFailed = ref(false)
// Chosen when the motion starts, like the video's source.
const motionSource = ref('')
let motionFailedTimer = 0

function toggleMotion() {
  if (motionPlaying.value) {
    motionPlaying.value = false
    return
  }
  resetView()
  motionSource.value = copyReady.value ? copyUrl(shown.value.id) : motionUrl(shown.value.id, shown.value.previewVersion)
  motionPlaying.value = true
  askForCopy()
}

const motionElement = ref<HTMLVideoElement | null>(null)

// Started from code rather than by autoplay: a browser may refuse to play it
// with sound, without an error, and it then plays muted.
watch(motionElement, (element) => {
  element?.play().catch(() => {
    if (motionElement.value !== element) {
      return
    }
    element.muted = true
    element.play().catch(onMotionFailed)
  })
})

function onMotionFailed() {
  if (!copyReady.value) {
    askForCopy()
  }
  motionPlaying.value = false
  motionFailed.value = true
  clearTimeout(motionFailedTimer)
  motionFailedTimer = window.setTimeout(() => (motionFailed.value = false), 4000)
}

function onMotionMetadata(event: Event) {
  if ((event.target as HTMLVideoElement).videoWidth === 0) {
    onMotionFailed()
  }
}

watch(
  () => shown.value.id,
  () => {
    unplayable.value = false
    motionPlaying.value = false
    motionFailed.value = false
    videoCopy.value = shown.value.videoCopy
    videoSource.value = playSource()
    copyComing.value = false
    waitingForCopy.value = false
    clearTimeout(copyPoll)
  },
)

// Events bring the copy too, for the owner's own items.
watch(
  () => shown.value.videoCopy,
  (state) => {
    if (state !== null && videoCopy.value === null) {
      copyArrived(state)
    }
  },
)

onBeforeUnmount(() => {
  clearTimeout(motionFailedTimer)
  clearTimeout(copyPoll)
})

// The original replaces the preview only once it has been decoded, so the
// preview stays on the screen while a large file loads.
const originalShown = ref(false)
let originalRequested = 0

function loadOriginal() {
  const photo = shown.value
  if (originalShown.value || originalRequested === photo.id || !canShowOriginal(photo)) {
    return
  }
  originalRequested = photo.id
  const image = new Image()
  image.src = originalUrl(photo.id)
  image.decode().then(
    () => {
      if (shown.value.id === photo.id) {
        originalShown.value = true
      }
    },
    () => {
      undecodable.add(photo.mime)
    },
  )
}

const source = computed(() => (originalShown.value ? originalUrl(shown.value.id) : previewUrl(shown.value.id, 'viewer')))

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
// scales it up to the resolution of what zooming shows, one image pixel per
// CSS pixel.
const stage = ref<HTMLElement | null>(null)
const image = ref<HTMLImageElement | null>(null)
const stageSize = ref({ width: 0, height: 0 })
// The preview's own size, measured when it loads; the original replaces it
// only later and leaves this as it is.
const naturalSize = ref<{ width: number; height: number } | null>(null)

const photoSize = computed(() =>
  shown.value.width && shown.value.height ? { width: shown.value.width, height: shown.value.height } : naturalSize.value,
)

// Zooming shows the original only when the profile asks for it and the
// browser can decode it. Otherwise it enlarges the preview, which goes no
// further than the preview's own resolution: past it the photo only blurs.
const zoomSize = computed(() =>
  showOriginals.value && canShowOriginal(shown.value) ? photoSize.value : naturalSize.value,
)

// A video keeps clear of the bar with the camera details at the bottom, which
// would otherwise cover its controls and take their clicks.
const bottomBar = ref<HTMLElement | null>(null)
const bottomBarHeight = ref(0)
const videoReserve = computed(() => (video.value ? bottomBarHeight.value : 0))

const fit = computed(() => {
  const size = photoSize.value
  if (!size || stageSize.value.width === 0) {
    return null
  }
  const height = stageSize.value.height - videoReserve.value
  const ratio = Math.min(1, stageSize.value.width / size.width, height / size.height)
  const width = size.width * ratio
  const maxScale = Math.max(1, (zoomSize.value?.width ?? 0) / width)
  return { width, height: size.height * ratio, maxScale }
})

// The scale over the fitting size, and the offset of the photo's centre from
// the centre of the screen, in screen pixels.
type View = { scale: number; x: number; y: number }
type Point = { x: number; y: number }

const view = ref<View>({ scale: 1, x: 0, y: 0 })
// Set for a zoom by a click or a key, which animates; gestures follow the
// fingers directly.
const animated = ref(false)

const zoomable = computed(() => !video.value && !motionPlaying.value && (fit.value?.maxScale ?? 1) > 1.01)
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
watch([() => shown.value.id, () => stageSize.value.width, () => stageSize.value.height], () => {
  view.value = { scale: 1, x: 0, y: 0 }
  animated.value = false
})

// The limit falls when an original turns out to be undecodable, or when the
// profile setting changes; a zoomed photo then stays zoomed, within the limit.
watch(() => fit.value?.maxScale, () => {
  const { scale, x, y } = view.value
  setView(scale, x, y)
})

watch(
  () => shown.value.id,
  () => {
    originalShown.value = false
    naturalSize.value = null
  },
)

watch([() => shown.value.id, showOriginals, zoomed], () => {
  if (showOriginals.value && zoomed.value) {
    loadOriginal()
  }
}, { immediate: true })

function measure() {
  if (stage.value) {
    stageSize.value = { width: stage.value.clientWidth, height: stage.value.clientHeight }
  }
  bottomBarHeight.value = bottomBar.value?.offsetHeight ?? 0
}

// The bar comes and goes with the details of each photo.
watch([camera, location], () => nextTick(measure))

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

// The height of the controls a browser draws at the bottom of a video.
const VIDEO_CONTROLS = 56

function onPointerDown(event: PointerEvent) {
  // The video's own controls take the mouse, and a finger on its bottom
  // edge, where dragging along the time bar must not step to another photo.
  if (videoElement.value && event.target === videoElement.value) {
    const rect = videoElement.value.getBoundingClientRect()
    if (event.pointerType === 'mouse' || event.clientY > rect.bottom - VIDEO_CONTROLS) {
      return
    }
  }
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

// Chrome on Android turns a swipe into a scroll with a fling even under
// touch-action: none, and the first tap after it only stops the fling: the
// buttons took no click for most of a second after a swipe. A cancelled
// touchmove starts no scroll. The video's controls, which make no gesture,
// keep their touches.
function onTouchMove(event: TouchEvent) {
  if (gesture && event.cancelable) {
    event.preventDefault()
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
  () => shown.value.id,
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
  if (props.paused || descriptionOpen.value || moreOpen.value) {
    return
  }
  if (event.key === 'Escape' && zoomed.value) {
    resetView(true)
  } else if (event.key === 'Escape') {
    emit('close')
  } else if (videoElement.value && document.activeElement === videoElement.value) {
    // The arrows of a focused video seek it.
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
  <div class="viewer" data-testid="viewer" role="dialog" aria-modal="true" :aria-label="shown.name">
    <div
      ref="stage"
      class="stage"
      :class="{ zoomable, zoomed }"
      @pointerdown="onPointerDown"
      @touchmove="onTouchMove"
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
        <div v-if="video" class="slide" :style="{ paddingBottom: `${videoReserve}px` }">
          <template v-if="!unplayable">
            <video
              ref="videoElement"
              :key="shown.id"
              :src="videoSource"
              :poster="previewUrl(shown.id, 'viewer')"
              :style="fit ? { width: `${fit.width}px`, height: `${fit.height}px` } : undefined"
              class="image"
              data-testid="viewer-video"
              controls
              playsinline
              preload="none"
              @error="onVideoUnplayable"
              @loadedmetadata="onVideoMetadata"
              @play="onVideoPlay"
            />
            <div v-if="waitingForCopy" class="unplayable" data-testid="video-hdr-copy-coming">
              <v-progress-circular indeterminate size="16" width="2" class="mr-2" />{{ t('viewer.videoHdrCopyComing') }}
              <div>
                <v-btn variant="text" class="mt-2" data-testid="video-play-original" :text="t('viewer.playOriginal')" @click="stopWaitingForCopy" />
              </div>
            </div>
          </template>
          <template v-else>
            <img
              :src="previewUrl(shown.id, 'viewer')"
              :alt="shown.name"
              :style="fit ? { width: `${fit.width}px`, height: `${fit.height}px` } : undefined"
              class="image"
              draggable="false"
            />
            <div class="unplayable" data-testid="video-unplayable">
              <div>{{ t('viewer.videoUnsupported') }}</div>
              <div v-if="copyComing" class="details" data-testid="video-copy-coming">
                <v-progress-circular indeterminate size="16" width="2" class="mr-2" />{{ t('viewer.videoCopyComing') }}
              </div>
              <div v-else class="details">{{ t('viewer.videoUnsupportedText') }}</div>
              <v-btn :href="originalUrl(shown.id)" prepend-icon="fa-download" variant="flat" class="mt-3" :text="t('viewer.download')" />
            </div>
          </template>
        </div>
        <div v-else class="slide">
          <video
            v-if="motionPlaying"
            :key="`motion-${shown.id}`"
            :src="motionSource"
            :style="fit ? { width: `${fit.width}px`, height: `${fit.height}px` } : undefined"
            class="image motion"
            ref="motionElement"
            data-testid="viewer-motion"
            playsinline
            @ended="motionPlaying = false"
            @error="onMotionFailed"
            @loadedmetadata="onMotionMetadata"
          />
          <img
            ref="image"
            :key="shown.id"
            :src="source"
            :alt="shown.name"
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
        <div class="name">{{ shown.name }}</div>
        <div class="details">{{ details }}</div>
        <div v-if="description" class="description" data-testid="viewer-description">{{ description }}</div>
      </div>
      <v-btn
        v-if="shown.hasMotion && !video"
        :icon="motionPlaying ? 'fa-stop' : 'fa-regular-circle-dot'"
        variant="text"
        data-testid="viewer-motion-button"
        :aria-label="motionPlaying ? t('viewer.stopMotion') : t('viewer.playMotion')"
        :title="motionPlaying ? t('viewer.stopMotion') : t('viewer.playMotion')"
        @click="toggleMotion"
      />
      <v-btn
        v-if="favorite !== null"
        :icon="favorite ? 'fa-star' : 'fa-regular-star'"
        variant="text"
        data-testid="viewer-favorite-button"
        :aria-pressed="favorite"
        :aria-label="favorite ? t('viewer.removeFavorite') : t('viewer.addFavorite')"
        :title="favorite ? t('viewer.removeFavorite') : t('viewer.addFavorite')"
        @click="toggleFavorite"
      />
      <v-menu v-if="xs && secondaryActions.length" v-model="moreOpen">
        <template #activator="{ props: menu }">
          <v-btn v-bind="menu" icon="fa-ellipsis-vertical" variant="text" data-testid="viewer-more" :aria-label="t('viewer.moreActions')" />
        </template>
        <v-list density="compact">
          <v-list-item
            v-for="action in secondaryActions"
            :key="action.key"
            :title="action.text"
            :prepend-icon="action.icon"
            :active="action.active"
            :data-testid="`viewer-${action.key}-button`"
            @click="action.click"
          />
        </v-list>
      </v-menu>
      <template v-else>
        <v-btn
          v-for="action in secondaryActions"
          :key="action.key"
          :icon="action.icon"
          variant="text"
          :active="action.active"
          :aria-pressed="action.active"
          :data-testid="`viewer-${action.key}-button`"
          :aria-label="action.text"
          :title="action.text"
          @click="action.click"
        />
      </template>
      <v-btn v-if="canManage" icon="fa-trash-can" variant="text" :aria-label="t('browser.moveToTrash')" @click="emit('trash', photo)" />
      <v-btn :href="originalUrl(shown.id)" icon="fa-download" variant="text" :aria-label="t('viewer.download')" />
      <v-btn icon="fa-xmark" variant="text" :aria-label="t('common.close')" @click="emit('close')" />
    </div>

    <div v-if="camera || location" ref="bottomBar" class="bar bottom">
      <div class="details" data-testid="camera">
        {{ camera }}<template v-if="camera && location"> · </template>
        <router-link v-if="location" :to="location.to" class="location" :title="t('viewer.showOnMap')">{{
          location.text
        }}</router-link>
      </div>
    </div>

    <div v-if="motionFailed" class="notice" role="status">
      {{ t(copyComing ? 'viewer.motionCopyComing' : 'viewer.motionUnsupported') }}
    </div>

    <v-btn
      v-if="previous"
      class="nav left"
      icon="fa-angle-left"
      variant="text"
      size="x-large"
      :aria-label="t('viewer.previous')"
      @click="emit('navigate', previous)"
    />
    <v-btn
      v-if="next"
      class="nav right"
      icon="fa-angle-right"
      variant="text"
      size="x-large"
      :aria-label="t('viewer.next')"
      @click="emit('navigate', next)"
    />
    <AppDialog
      v-model="descriptionOpen"
      :title="t('viewer.editDescription')"
      :action="t('common.save')"
      :error="descriptionError"
      :loading="descriptionSaving"
      :disabled="descriptionDraft.trim().length > MAX_DESCRIPTION"
      :form="false"
      :max-width="560"
      @submit="saveDescription"
    >
      <AppFormGroup v-slot="field" :label="t('viewer.description')">
        <v-textarea
          v-bind="field"
          v-model="descriptionDraft"
          data-testid="description-field"
          rows="4"
          auto-grow
          autofocus
          :counter="MAX_DESCRIPTION"
        />
      </AppFormGroup>
    </AppDialog>
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
  box-sizing: border-box;
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

/* The motion lies over the photo, at its size, and leaves gestures to it. */
.image.motion {
  position: absolute;
  pointer-events: none;
}

.unplayable {
  position: absolute;
  padding: 16px 24px;
  border-radius: 8px;
  background: rgba(0, 0, 0, 0.7);
  text-align: center;
}

.notice {
  position: absolute;
  bottom: 72px;
  left: 50%;
  max-width: calc(100% - 32px);
  padding: 8px 16px;
  border-radius: 999px;
  background: rgba(0, 0, 0, 0.75);
  transform: translateX(-50%);
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

/* A long description scrolls within a few lines, so the photo keeps the screen. */
.description {
  max-height: 4.5em;
  overflow-y: auto;
  margin-top: 4px;
  white-space: pre-line;
  overflow-wrap: anywhere;
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
