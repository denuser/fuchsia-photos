<script setup lang="ts">
import { computed, inject, onMounted, ref, shallowRef, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { useDisplay } from 'vuetify'

import { api, type MediaKind, type NameCount } from '../api/client'
import AppAutocomplete from '../components/AppAutocomplete.vue'
import EmptyState from '../components/EmptyState.vue'
import FilterChips, { type FilterGroup } from '../components/FilterChips.vue'
import PhotoBrowser from '../components/PhotoBrowser.vue'
import { formatDate, t } from '../i18n'
import { apiError } from '../i18n/apiErrors'
import { useAuthStore } from '../stores/auth'
import { listFilters, usePhotosStore, type PhotoQuery } from '../stores/photos'
import { useTagsStore } from '../stores/tags'
import { pickUploadsKey, useUploadsStore } from '../stores/uploads'

const route = useRoute()
const router = useRouter()
const pickUploads = inject(pickUploadsKey, () => {})
const auth = useAuthStore()
const photos = usePhotosStore()
const tags = useTagsStore()
const uploads = useUploadsStore()

const dragging = ref(false)
const browser = ref<InstanceType<typeof PhotoBrowser> | null>(null)

// Whether the timeline shows other users' photos the user can see next to the
// user's own. It is a setting of the user rather than a filter in the address,
// so that it stays as chosen on every device.
const withShared = computed(() => auth.user?.timelineIncludesShared ?? true)
const savingScope = ref(false)

async function setWithShared(value: boolean) {
  if (value === withShared.value) {
    return
  }
  savingScope.value = true
  const { data, error } = await api.PATCH('/me', { body: { timelineIncludesShared: value } })
  savingScope.value = false
  if (data) {
    auth.user = data
  } else {
    browser.value?.say(apiError(error, t('profile.saveFailed')), 'error')
  }
}

// On a phone the types, cameras, lenses and dates hide behind a button, so the filters take one row.
const { xs } = useDisplay()

// The types, cameras and lenses of the photos the other filters keep, with how many photos each would add.
const kindList = shallowRef<NameCount[]>([])
const cameraList = shallowRef<NameCount[]>([])
const lensList = shallowRef<NameCount[]>([])

// The order the types are offered in, rather than the server's alphabetical one.
const kindOrder: MediaKind[] = ['photo', 'motion', 'video']
function isKind(name: string): name is MediaKind {
  return (kindOrder as string[]).includes(name)
}

// The address holds the filters, so that they survive a reload and a tag page can link to them.
function queryList(key: string) {
  const value = route.query[key]
  const list = Array.isArray(value) ? value : value ? [value] : []
  return list.filter((item): item is string => typeof item === 'string')
}
const tagIds = computed(() => queryList('tag').map(Number).filter((id) => Number.isInteger(id)))
const cameras = computed(() => queryList('camera'))
const lenses = computed(() => queryList('lens'))
const kinds = computed(() => queryList('kind').filter(isKind))
const fromDate = computed(() => (typeof route.query.from === 'string' ? route.query.from : ''))
const toDate = computed(() => (typeof route.query.to === 'string' ? route.query.to : ''))
const dateFields = computed(() => [
  { key: 'from', label: t('timeline.from'), value: fromDate.value },
  { key: 'to', label: t('timeline.to'), value: toDate.value },
])
// The filters a phone keeps behind the button.
const moreFiltered = computed(() => kinds.value.length > 0 || cameras.value.length > 0 || lenses.value.length > 0 || Boolean(fromDate.value || toDate.value))
const filtered = computed(() => tagIds.value.length > 0 || moreFiltered.value)

interface FilterChange {
  tag?: number[]
  camera?: string[]
  lens?: string[]
  kind?: string[]
  from?: string | null
  to?: string | null
}

function setFilter(change: FilterChange) {
  const next = { tag: tagIds.value, kind: kinds.value as string[], camera: cameras.value, lens: lenses.value, from: fromDate.value, to: toDate.value, ...change }
  void router.replace({
    query: {
      ...(next.tag.length > 0 ? { tag: next.tag.map(String) } : {}),
      ...(next.kind.length > 0 ? { kind: next.kind } : {}),
      ...(next.camera.length > 0 ? { camera: next.camera } : {}),
      ...(next.lens.length > 0 ? { lens: next.lens } : {}),
      ...(next.from ? { from: next.from } : {}),
      ...(next.to ? { to: next.to } : {}),
    },
  })
}

// A chosen name the other filters leave no photos stays at the top of its list, so that it can be removed.
function withChosen(items: NameCount[], chosen: string[]) {
  const missing = chosen.filter((name) => !items.some((item) => item.name === name))
  if (missing.length === 0) {
    return items
  }
  return [...missing.map((name) => ({ name, count: 0 })), ...items]
}

function kindTitle(name: string) {
  return isKind(name) ? t(`timeline.kind.${name}`) : name
}

// The filters with a count for each choice. A camera or a lens is its own title; a type is translated.
const cameraFields = computed(() => [
  {
    key: 'kind',
    label: t('timeline.anyKind'),
    counted: t('timeline.kinds'),
    items: withChosen(kindList.value, kinds.value)
      .map((item) => ({ ...item, title: kindTitle(item.name) }))
      .sort((a, b) => kindOrder.indexOf(a.name as MediaKind) - kindOrder.indexOf(b.name as MediaKind)),
    value: kinds.value as string[],
  },
  {
    key: 'camera',
    label: t('timeline.anyCamera'),
    counted: t('timeline.cameras'),
    items: withChosen(cameraList.value, cameras.value).map((item) => ({ ...item, title: item.name })),
    value: cameras.value,
  },
  {
    key: 'lens',
    label: t('timeline.anyLens'),
    counted: t('timeline.lenses'),
    items: withChosen(lensList.value, lenses.value).map((item) => ({ ...item, title: item.name })),
    value: lenses.value,
  },
])

// The filters in use, as chip groups under the toolbar.
// The address keeps a day as YYYY-MM-DD; the date fields work with Date.
function asDate(day: string) {
  const date = new Date(`${day}T00:00:00`)
  return Number.isNaN(date.getTime()) ? null : date
}

function asDay(value: unknown) {
  if (!(value instanceof Date) || Number.isNaN(value.getTime())) {
    return null
  }
  const pad = (n: number) => String(n).padStart(2, '0')
  return `${value.getFullYear()}-${pad(value.getMonth() + 1)}-${pad(value.getDate())}`
}

function dayTitle(date: string) {
  const day = new Date(`${date}T00:00:00`)
  // Written as the date fields write it, with two-digit days and months.
  return Number.isNaN(day.getTime()) ? date : formatDate(day)
}

const filterGroups = computed<FilterGroup[]>(() => [
  {
    key: 'tag',
    label: t('browser.tags'),
    chips: tagIds.value.map((id) => ({ value: String(id), title: tags.tags.find((tag) => tag.id === id)?.name ?? `#${id}` })),
  },
  { key: 'kind', label: t('timeline.kinds'), chips: kinds.value.map((name) => ({ value: name, title: kindTitle(name) })) },
  { key: 'camera', label: t('timeline.cameras'), chips: cameras.value.map((name) => ({ value: name, title: name })) },
  { key: 'lens', label: t('timeline.lenses'), chips: lenses.value.map((name) => ({ value: name, title: name })) },
  {
    key: 'date',
    label: t('timeline.date'),
    chips: [
      ...(fromDate.value ? [{ value: 'from', title: t('timeline.fromDay', { day: dayTitle(fromDate.value) }) }] : []),
      ...(toDate.value ? [{ value: 'to', title: t('timeline.toDay', { day: dayTitle(toDate.value) }) }] : []),
    ],
  },
])

function removeChip(key: string, value: string) {
  if (key === 'tag') {
    setFilter({ tag: tagIds.value.filter((id) => String(id) !== value) })
  } else if (key === 'kind') {
    setFilter({ kind: kinds.value.filter((name) => name !== value) })
  } else if (key === 'camera') {
    setFilter({ camera: cameras.value.filter((name) => name !== value) })
  } else if (key === 'lens') {
    setFilter({ lens: lenses.value.filter((name) => name !== value) })
  } else {
    setFilter({ [value]: null })
  }
}

function clearGroup(key: string) {
  setFilter(key === 'date' ? { from: null, to: null } : { [key]: [] })
}

function photoCount(item: NameCount) {
  return { subtitle: t('common.photos', item.count) }
}

// Counts the request of the latest filters, so that an earlier answer arriving late is dropped.
let camerasRequest = 0

async function loadCameras(query: PhotoQuery) {
  const request = ++camerasRequest
  const { data } = await api.GET('/photos/cameras', { params: { query: { ...listFilters(query), from: query.from, to: query.to } } })
  if (data && request === camerasRequest) {
    kindList.value = data.kinds
    cameraList.value = data.cameras
    lensList.value = data.lenses
  }
}

// Uploads and processing change the counts after the lists were loaded, so a list opened again asks anew.
function onFieldMenu(open: boolean) {
  if (open) {
    void loadCameras(currentQuery())
  }
}

// The start of a day in the browser's time zone. The end of a range is exclusive, so it takes the next day.
function dayStart(date: string, addDays = 0) {
  const day = new Date(`${date}T00:00:00`)
  if (Number.isNaN(day.getTime())) {
    return undefined
  }
  day.setDate(day.getDate() + addDays)
  return day.toISOString()
}

function currentQuery(): PhotoQuery {
  return {
    withShared: withShared.value,
    tags: tagIds.value,
    cameras: cameras.value,
    lenses: lenses.value,
    kinds: kinds.value,
    from: fromDate.value ? dayStart(fromDate.value) : undefined,
    to: toDate.value ? dayStart(toDate.value, 1) : undefined,
  }
}

onMounted(() => {
  void tags.load()
  const query = currentQuery()
  void loadCameras(query)
  void photos.open(query)
})

watch(
  [() => route.query, withShared],
  () => {
    const query = currentQuery()
    void loadCameras(query)
    void photos.open(query)
  },
)

function onTagged() {
  if (tagIds.value.length > 0) {
    void loadCameras(currentQuery())
    void photos.reload()
  }
}

function onDrop(event: DragEvent) {
  dragging.value = false
  const files = Array.from(event.dataTransfer?.files ?? [])
  if (files.length > 0) {
    uploads.add(files)
  }
}
</script>

<template>
  <div class="timeline" @dragover.prevent="dragging = true" @dragleave.self="dragging = false" @drop.prevent="onDrop">
    <PhotoBrowser ref="browser" mode="library" @tagged="onTagged">
      <template #toolbar>
        <div class="filters">
          <v-btn-toggle
            :model-value="withShared ? 'all' : 'mine'"
            mandatory
            divided
            variant="outlined"
            density="compact"
            :disabled="savingScope"
            class="scope"
            @update:model-value="setWithShared($event === 'all')"
          >
            <v-btn v-if="xs" value="mine" icon="fa-user" :aria-label="t('timeline.mine')" />
            <v-btn v-else value="mine" :text="t('timeline.mine')" />
            <v-btn v-if="xs" value="all" icon="fa-users" :aria-label="t('timeline.all')" />
            <v-btn v-else value="all" :text="t('timeline.all')" />
          </v-btn-toggle>
          <AppAutocomplete
            :model-value="tagIds"
            :items="tags.tags"
            item-title="name"
            item-value="id"
            :placeholder="t('timeline.anyTag')"
            :aria-label="t('timeline.anyTag')"
            :counted="t('browser.tags')"
            density="compact"
            hide-details
            multiple
            clearable
            class="tag-filter"
            @update:model-value="setFilter({ tag: $event })"
          />
          <v-menu v-if="xs" :close-on-content-click="false">
            <template #activator="{ props: menu }">
              <v-btn
                v-bind="menu"
                icon="fa-filter"
                :color="moreFiltered ? 'primary' : undefined"
                variant="text"
                :aria-label="t('timeline.moreFilters')"
              />
            </template>
            <v-card class="filter-menu">
              <AppAutocomplete
                v-for="field in cameraFields"
                :key="field.key"
                :model-value="field.value"
                :items="field.items"
                item-title="title"
                item-value="name"
                :item-props="photoCount"
                :placeholder="field.label"
                :aria-label="field.label"
                :counted="field.counted"
                density="compact"
                clearable
                hide-details
                multiple
                @update:menu="onFieldMenu"
                @update:model-value="setFilter({ [field.key]: $event })"
              />
              <v-date-input
                v-for="field in dateFields"
                :key="field.key"
                :model-value="asDate(field.value)"
                :prefix="field.label"
                :aria-label="field.label"
                persistent-placeholder
                prepend-icon=""
                append-inner-icon="fa-regular-calendar-days"
                density="compact"
                hide-details
                @update:model-value="setFilter({ [field.key]: asDay($event) })"
              />
            </v-card>
          </v-menu>
          <template v-else>
            <AppAutocomplete
              v-for="field in cameraFields"
              :key="field.key"
              :model-value="field.value"
              :items="field.items"
              item-title="title"
              item-value="name"
              :item-props="photoCount"
              :placeholder="field.label"
              :aria-label="field.label"
              :counted="field.counted"
              density="compact"
              hide-details
              multiple
              class="camera-filter"
              @update:menu="onFieldMenu"
              @update:model-value="setFilter({ [field.key]: $event })"
            />
            <v-date-input
              v-for="field in dateFields"
              :key="field.key"
              :model-value="asDate(field.value)"
              :prefix="field.label"
              :aria-label="field.label"
              persistent-placeholder
              prepend-icon=""
              append-inner-icon="fa-regular-calendar-days"
              density="compact"
              hide-details
              class="date-filter"
              @update:model-value="setFilter({ [field.key]: asDay($event) })"
            />
          </template>
        </div>
        <FilterChips :groups="filterGroups" @remove="removeChip" @clear="clearGroup" @clear-all="router.replace({ query: {} })" />
      </template>
      <template #empty>
        <EmptyState v-if="filtered" icon="fa-magnifying-glass" :title="t('timeline.noMatch')" :text="t('timeline.noMatchText')">
          <template #actions>
            <v-btn :text="t('filters.clearAll')" variant="text" color="primary" @click="router.replace({ query: {} })" />
          </template>
        </EmptyState>
        <EmptyState v-else icon="fa-images" :title="t('timeline.none')" :text="t('timeline.noneText')">
          <template #actions>
            <v-btn :text="t('timeline.upload')" color="primary" prepend-icon="fa-upload" @click="pickUploads" />
          </template>
        </EmptyState>
      </template>
    </PhotoBrowser>

    <div v-if="dragging" class="drop-hint">{{ t('timeline.drop') }}</div>
  </div>
</template>

<style scoped>
.timeline {
  position: relative;
  height: 100%;
}

.filters {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 8px;
  padding: 8px;
}

.scope {
  flex: none;
}

.tag-filter {
  flex: 1 1 200px;
  max-width: 480px;
}

.camera-filter {
  flex: 1 1 200px;
  max-width: 360px;
}

.date-filter {
  flex: 0 1 196px;
}

.filter-menu {
  display: flex;
  flex-direction: column;
  gap: 8px;
  width: min(320px, calc(100vw - 24px));
  padding: 12px;
}

@media (max-width: 599.98px) {
  .filters {
    flex-wrap: nowrap;
    gap: 4px;
  }

  .tag-filter {
    flex-basis: 0;
    min-width: 0;
  }
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
