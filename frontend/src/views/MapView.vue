<script setup lang="ts">
import * as maplibregl from 'maplibre-gl'
import 'maplibre-gl/dist/maplibre-gl.css'
import { computed, nextTick, onBeforeUnmount, onMounted, ref, shallowRef, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'

import { api, previewUrl } from '../api/client'
import type { components } from '../api/schema'
import AppPageHeader from '../components/AppPageHeader.vue'
import EmptyState from '../components/EmptyState.vue'
import PhotoBrowser from '../components/PhotoBrowser.vue'
import { t } from '../i18n'
import { usePhotosStore } from '../stores/photos'

type Cluster = components['schemas']['MapCluster']

// The user's photos on a map, in clusters that follow the zoom. A click
// zooms in; at the closest zoom, or on one photo, it lists the cluster's
// photos under the map. Tiles come from OpenStreetMap's public servers,
// straight from the browser (decided on 2026-11-22). MapLibre GL JS draws
// them (chosen over Leaflet on 2026-11-22, which prints a flag before its
// name in the credit line), and can read vector tiles from a file on the NAS
// later without another library.
const route = useRoute()
const router = useRouter()
const photos = usePhotosStore()

const element = ref<HTMLElement | null>(null)
const state = ref<'loading' | 'ready' | 'hidden' | 'failed'>('loading')
const noPhotos = ref(false)
// The cluster whose photos are listed under the map.
const opened = shallowRef<Cluster | null>(null)

let map: maplibregl.Map | null = null
let markers: maplibregl.Marker[] = []
let generation = 0
let timer: number | undefined

const MAX_ZOOM = 18
// At this zoom a cluster is a few metres wide, so a click lists its photos.
const LIST_ZOOM = 17
const PADDING = 48

const style: maplibregl.StyleSpecification = {
  version: 8,
  sources: {
    osm: {
      type: 'raster',
      tiles: ['https://tile.openstreetmap.org/{z}/{x}/{y}.png'],
      tileSize: 256,
      maxzoom: 19,
      attribution: '&copy; <a href="https://www.openstreetmap.org/copyright" target="_blank" rel="noopener noreferrer">OpenStreetMap</a> contributors',
    },
  },
  layers: [{ id: 'osm', type: 'raster', source: 'osm' }],
}

// A marker is plain DOM that MapLibre places, so its classes are global.
function markerElement(cluster: Cluster) {
  const label = t('map.photosHere', { n: cluster.count }, cluster.count)
  const el = document.createElement('div')
  el.className = 'fp-marker'
  el.setAttribute('role', 'button')
  el.tabIndex = 0
  el.setAttribute('aria-label', label)
  el.title = label
  const img = document.createElement('img')
  img.src = previewUrl(cluster.coverId, 'grid', cluster.coverVersion)
  img.alt = ''
  img.draggable = false
  el.append(img)
  if (cluster.count > 1) {
    const count = document.createElement('b')
    count.className = 'fp-marker__count'
    count.textContent = String(cluster.count)
    el.append(count)
  }
  el.addEventListener('click', () => choose(cluster))
  el.addEventListener('keydown', (event) => {
    if (event.key === 'Enter' || event.key === ' ') {
      event.preventDefault()
      choose(cluster)
    }
  })
  return el
}

function show(clusters: Cluster[]) {
  for (const marker of markers) {
    marker.remove()
  }
  markers = []
  if (!map) {
    return
  }
  for (const cluster of clusters) {
    markers.push(new maplibregl.Marker({ element: markerElement(cluster) }).setLngLat([cluster.longitude, cluster.latitude]).addTo(map))
  }
}

function choose(cluster: Cluster) {
  const [west, south, east, north] = cluster.box
  const zoom = map?.getZoom() ?? 0
  const point = south === north && west === east
  if (!map || cluster.count === 1 || point || zoom >= LIST_ZOOM) {
    opened.value = cluster
    void photos.open({ withShared: true, bbox: cluster.box.join(',') })
    return
  }
  const fit = map.cameraForBounds([[west, south], [east, north]], { padding: PADDING, maxZoom: MAX_ZOOM })
  map.easeTo({ center: fit?.center ?? [cluster.longitude, cluster.latitude], zoom: Math.min(MAX_ZOOM, Math.max(fit?.zoom ?? 0, zoom + 1)) })
}

// The grid of clusters is about 64 pixels a cell: a 256 pixel tile has four
// cells at the zoom of its tile, which is one more than MapLibre's own zoom,
// as it counts 512 pixel tiles.
function apiZoom(zoom: number) {
  return Math.min(22, Math.max(0, Math.round(zoom) + 1))
}

async function load(fit = false) {
  if (!map) {
    return
  }
  const started = ++generation
  const bounds = map.getBounds()
  const box = fit
    ? [-180, -90, 180, 90]
    : [bounds.getWest(), bounds.getSouth(), bounds.getEast(), bounds.getNorth()].map((n, i) => {
        const limit = i % 2 === 0 ? 180 : 90
        return Math.max(-limit, Math.min(limit, n))
      })
  const { data, response } = await api.GET('/photos/map', {
    params: { query: { bbox: box.join(','), zoom: fit ? 2 : apiZoom(map.getZoom()) } },
  })
  if (started !== generation) {
    return
  }
  if (!data) {
    state.value = response.status === 403 ? 'hidden' : 'failed'
    return
  }
  state.value = 'ready'
  if (!fit) {
    show(data.clusters)
    return
  }
  // The first look: the whole world's clusters give the area to show.
  if (data.clusters.length === 0) {
    noPhotos.value = true
    return
  }
  const west = Math.min(...data.clusters.map((c) => c.box[0]))
  const south = Math.min(...data.clusters.map((c) => c.box[1]))
  const east = Math.max(...data.clusters.map((c) => c.box[2]))
  const north = Math.max(...data.clusters.map((c) => c.box[3]))
  // The move that follows loads the clusters of the new view.
  map.fitBounds([[west, south], [east, north]], { padding: PADDING, maxZoom: 11, animate: false })
}

function later() {
  window.clearTimeout(timer)
  timer = window.setTimeout(() => void load(), 200)
}

function number(value: unknown) {
  const n = Number(Array.isArray(value) ? value[0] : value)
  return value === undefined || value === null || value === '' || Number.isNaN(n) ? null : n
}

// The viewer's coordinates link to this page with the place in the address.
// The page is already open when it was left for the viewer from the list under
// the map, so the address is read again when it changes, closing the list and
// the viewer in it. The address is cleared once read, so that the same link
// works again.
function place() {
  const lat = number(route.query.lat)
  const lon = number(route.query.lon)
  return lat !== null && lon !== null ? { lat, lon, zoom: number(route.query.zoom) ?? 15 } : null
}

watch(
  () => route.query,
  async () => {
    const target = place()
    if (!target || !map) {
      return
    }
    opened.value = null
    await nextTick()
    // The map has the page's height again once the list is gone.
    map.resize()
    map.easeTo({ center: [target.lon, target.lat], zoom: Math.min(MAX_ZOOM, target.zoom) })
    void router.replace({ path: route.path, query: {} })
  },
)

onMounted(() => {
  if (!element.value) {
    return
  }
  const target = place()
  const located = target !== null
  map = new maplibregl.Map({
    container: element.value,
    style,
    center: located ? [target.lon, target.lat] : [0, 30],
    zoom: located ? target.zoom : 1,
    minZoom: 1,
    maxZoom: MAX_ZOOM,
    maxBounds: [[-180, -85.05], [180, 85.05]],
    attributionControl: { compact: true },
    dragRotate: false,
    pitchWithRotate: false,
    touchPitch: false,
  })
  map.addControl(new maplibregl.NavigationControl({ showCompass: false }), 'top-left')
  map.on('moveend', later)
  void load(!located)
  if (located) {
    void router.replace({ path: route.path, query: {} })
  }
})

onBeforeUnmount(() => {
  window.clearTimeout(timer)
  generation++
  map?.remove()
  map = null
  markers = []
})

const listed = computed(() => (opened.value ? t('map.photosHere', { n: opened.value.count }, opened.value.count) : ''))

function back() {
  opened.value = null
}
</script>

<template>
  <div class="map-page">
    <AppPageHeader :title="t('nav.map')" :description="t('map.description')" />
    <EmptyState v-if="state === 'hidden'" icon="fa-map-location-dot" :title="t('map.hidden')" :text="t('map.hiddenText')" />
    <EmptyState v-else-if="state === 'failed'" icon="fa-map-location-dot" danger :title="t('map.failed')" />
    <EmptyState v-else-if="noPhotos" icon="fa-map-location-dot" :title="t('map.empty')" :text="t('map.emptyText')" />
    <template v-else>
      <div class="map-wrap" :class="{ short: opened }">
        <div ref="element" class="map" data-testid="map" />
      </div>
      <div v-if="opened" class="listed" data-testid="map-photos">
        <PhotoBrowser mode="library">
          <template #toolbar>
            <div class="listed-bar">
              <span>{{ listed }}</span>
              <v-btn variant="text" density="comfortable" :text="t('map.back')" @click="back" />
            </div>
          </template>
        </PhotoBrowser>
      </div>
    </template>
  </div>
</template>

<style scoped>
.map-page {
  display: flex;
  flex-direction: column;
  height: 100%;
}

.map-wrap {
  flex: 1 1 auto;
  min-height: 280px;
  padding: 0 24px 24px;
}

.map-wrap.short {
  flex: 1 1 45%;
  padding-bottom: 16px;
}

.map {
  height: 100%;
  min-height: 240px;
  border-radius: var(--radius-small, 6px);
  overflow: hidden;
  /* The map's panes and controls have z-indexes of their own: keep them under the app bar and dialogs. */
  isolation: isolate;
}

.listed {
  flex: 1 1 55%;
  min-height: 0;
}

.listed-bar {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 0 24px 8px;
  font-weight: 600;
}

@media (max-width: 599px) {
  .map-wrap {
    padding-inline: 16px;
  }

  .listed-bar {
    padding-inline: 16px;
  }
}
</style>

<style>
/* Markers are created outside Vue, so these styles are not scoped. MapLibre
   positions the element by its centre. */
.fp-marker {
  width: 56px;
  height: 56px;
  cursor: pointer;
}

.fp-marker:focus-visible {
  outline: 2px solid rgb(var(--v-theme-primary));
  outline-offset: 2px;
  border-radius: 8px;
}

.fp-marker img {
  display: block;
  width: 56px;
  height: 56px;
  object-fit: cover;
  border: 2px solid #fff;
  border-radius: 8px;
  box-shadow: 0 1px 4px rgb(0 0 0 / 0.5);
  background: #ddd;
}

.fp-marker__count {
  position: absolute;
  top: -8px;
  right: -8px;
  min-width: 22px;
  padding: 1px 6px;
  border-radius: 11px;
  background: rgb(var(--v-theme-primary));
  color: rgb(var(--v-theme-on-primary));
  font-size: 12px;
  font-weight: 600;
  line-height: 18px;
  text-align: center;
}
</style>
