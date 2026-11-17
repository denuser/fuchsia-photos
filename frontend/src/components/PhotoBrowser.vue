<script setup lang="ts">
import { computed, onMounted, ref, shallowRef, watch } from 'vue'
import { useRouter } from 'vue-router'
import { useDisplay } from 'vuetify'

import type { Photo } from '../api/client'
import { useAuthStore } from '../stores/auth'
import { type NoticeType, useNoticesStore } from '../stores/notices'
import { usePhotosStore } from '../stores/photos'
import { useUsersStore } from '../stores/users'
import { apiError } from '../i18n/apiErrors'
import AddToAlbumDialog from './AddToAlbumDialog.vue'
import ConfirmDialog from './ConfirmDialog.vue'
import EmptyState from './EmptyState.vue'
import PhotoGrid from './PhotoGrid.vue'
import PhotoViewer from './PhotoViewer.vue'
import ShareDialog from './ShareDialog.vue'
import TagDialog from './TagDialog.vue'
import { t } from '../i18n'

// The grid, the viewer and the bulk actions shared by all lists of photos. A
// library list edits the user's photos, and the trash restores or purges them.
// An administrator's list shares or trashes the photos of any user. Photos
// shared with the user are read-only, also where a library list shows them
// next to the user's own.
// Extra actions on the selection, such as an album's, come before the
// browser's own; each runs on the selected ids and reports done when it ends.
export interface ExtraAction {
  key: string
  icon: string
  text: string
  action: () => Promise<void>
  done: string
}

const props = defineProps<{
  mode: 'library' | 'trash' | 'admin' | 'shared'
  extraActions?: (ids: number[]) => ExtraAction[]
}>()
const emit = defineEmits<{ tagged: [] }>()

const router = useRouter()
const { xs } = useDisplay()
const auth = useAuthStore()
const photos = usePhotosStore()
const notices = useNoticesStore()
const users = useUsersStore()

const opened = shallowRef<Photo | null>(null)
const selected = shallowRef<ReadonlySet<number>>(new Set())
const anchor = ref<number | null>(null)
const busy = ref(false)

const tagDialog = ref(false)
const albumDialog = ref(false)
const purgeDialog = ref(false)
const shareDialog = ref(false)
const viewerTargets = ref<number[]>([])

const selectedIds = computed(() => [...selected.value])
// The tag and share dialogs serve both the selection and the photo in the viewer.
const tagTargets = computed(() => (viewerTargets.value.length > 0 ? viewerTargets.value : selectedIds.value))
const dialogOpen = computed(() => tagDialog.value || albumDialog.value || purgeDialog.value || shareDialog.value)
const canManage = computed(() => props.mode === 'library' || props.mode === 'admin')

// Whether the user can select a photo and act on it. Only an administrator's
// list manages other users' photos.
function selectable(photo: Photo) {
  if (props.mode === 'library') {
    return photo.ownerId === auth.user?.id
  }
  return props.mode !== 'shared'
}

const viewerCanTag = computed(() => props.mode === 'library' && opened.value !== null && selectable(opened.value))
const viewerCanManage = computed(() => canManage.value && opened.value !== null && selectable(opened.value))

// The viewer steps through the loaded photos around the open one, and the
// months next to it load as it gets there.
const viewerPhotos = computed(() => (opened.value ? photos.around(opened.value) : []))

watch(opened, (photo) => {
  if (photo) {
    photos.loadAround(photo)
  }
})

onMounted(() => {
  photos.connect()
  void users.load()
})

function clearSelection() {
  selected.value = new Set()
  anchor.value = null
}

// With range, selects everything between the last toggled photo and this one.
function toggle(photo: Photo, range: boolean) {
  const next = new Set(selected.value)
  const list = photos.photos
  const from = anchor.value === null ? -1 : list.findIndex((p) => p.id === anchor.value)
  if (range && from >= 0) {
    const to = list.findIndex((p) => p.id === photo.id)
    for (const p of list.slice(Math.min(from, to), Math.max(from, to) + 1)) {
      if (selectable(p)) {
        next.add(p.id)
      }
    }
  } else if (next.has(photo.id)) {
    next.delete(photo.id)
  } else {
    next.add(photo.id)
  }
  anchor.value = photo.id
  selected.value = next
}

function say(message: string, type: NoticeType = 'success') {
  notices.notify(message, type)
}

// Runs an action on the selection and reports what happened.
async function run(action: () => Promise<void>, done: string) {
  busy.value = true
  try {
    await action()
    clearSelection()
    say(done)
  } catch (failure) {
    say(apiError((failure as Error).message), 'error')
  } finally {
    busy.value = false
  }
}

function trashSelected() {
  const ids = selectedIds.value
  void run(() => photos.trash(ids), t('browser.trashed', ids.length))
}

function restoreSelected() {
  const ids = selectedIds.value
  void run(() => photos.restore(ids), t('browser.restored', ids.length))
}

function favoriteSelected(value: boolean) {
  const ids = selectedIds.value
  void run(() => photos.favorite(ids, value), t(value ? 'browser.favorited' : 'browser.unfavorited', ids.length))
}

function purgeSelected() {
  const ids = selectedIds.value
  void run(() => photos.purge(ids), t('browser.deleted', ids.length))
}

function openSelectionTags() {
  viewerTargets.value = []
  tagDialog.value = true
}

function openViewerTags(photo: Photo) {
  viewerTargets.value = [photo.id]
  tagDialog.value = true
}

function onTagged() {
  if (viewerTargets.value.length === 0) {
    clearSelection()
  }
  say(t('browser.tagsSaved'))
  emit('tagged')
}

function openSelectionSharing() {
  viewerTargets.value = []
  shareDialog.value = true
}

function openViewerSharing(photo: Photo) {
  viewerTargets.value = [photo.id]
  shareDialog.value = true
}

function onShared() {
  if (viewerTargets.value.length === 0) {
    clearSelection()
  }
  say(t('browser.sharingSaved'))
}

function onAddedToAlbum(album: string) {
  const n = selected.value.size
  clearSelection()
  say(t('browser.addedToAlbum', { album }, n))
}

// Stays in the viewer on a neighbouring photo, if there is one.
async function trashOpened(photo: Photo) {
  const ready = viewerPhotos.value.filter((p) => p.status === 'ready')
  const index = ready.findIndex((p) => p.id === photo.id)
  const neighbour = ready[index + 1] ?? ready[index - 1] ?? null
  try {
    await photos.trash([photo.id])
    opened.value = neighbour
    say(t('browser.trashed', 1))
  } catch (failure) {
    say(apiError((failure as Error).message), 'error')
  }
}

function findSimilar(photo: Photo) {
  opened.value = null
  void router.push({ path: '/search', query: { similar: String(photo.id) } })
}

interface SelectionAction {
  key: string
  icon: string
  text: string
  click: () => void
  danger?: boolean
  // Shows progress on the button while the action runs; the others are disabled.
  spins?: boolean
}

// The actions on the selection: buttons in the bar, or, on a phone, where they
// do not fit beside the count, items of a menu.
const selectionActions = computed<SelectionAction[]>(() => {
  const ids = selectedIds.value
  const list: SelectionAction[] = (props.extraActions?.(ids) ?? []).map((extra) => ({
    key: extra.key,
    icon: extra.icon,
    text: extra.text,
    click: () => void run(extra.action, extra.done),
  }))
  if (canManage.value) {
    if (props.mode === 'library') {
      list.push(
        { key: 'album', icon: 'fa-book-open', text: t('browser.addToAlbum'), click: () => (albumDialog.value = true) },
        { key: 'tags', icon: 'fa-tag', text: t('browser.tags'), click: openSelectionTags },
      )
      // On the Favorites page every photo is one already.
      if (!photos.query.favorites) {
        list.push({ key: 'favorite', icon: 'fa-star', text: t('browser.addToFavorites'), click: () => favoriteSelected(true), spins: true })
      }
      list.push({
        key: 'unfavorite',
        icon: 'fa-regular-star',
        text: t('browser.removeFromFavorites'),
        click: () => favoriteSelected(false),
        spins: true,
      })
    }
    list.push(
      { key: 'share', icon: 'fa-share-nodes', text: t('browser.share'), click: openSelectionSharing },
      { key: 'trash', icon: 'fa-trash-can', text: t('browser.moveToTrash'), click: trashSelected, spins: true },
    )
  } else {
    list.push(
      { key: 'restore', icon: 'fa-trash-can-arrow-up', text: t('browser.restore'), click: restoreSelected, spins: true },
      { key: 'delete', icon: 'fa-trash-can', text: t('common.delete'), click: () => (purgeDialog.value = true), danger: true },
    )
  }
  return list
})

defineExpose({ clearSelection, say })
</script>

<template>
  <div class="browser">
    <slot name="header" />
    <v-toolbar v-if="selected.size > 0" density="compact" color="surface-variant" class="selection-bar flex-grow-0">
      <v-btn icon="fa-xmark" :aria-label="t('browser.clearSelection')" @click="clearSelection" />
      <v-toolbar-title>{{ t('selection.count', { n: selected.size }) }}</v-toolbar-title>
      <v-menu v-if="xs">
        <template #activator="{ props: menu }">
          <v-btn v-bind="menu" icon="fa-ellipsis-vertical" :loading="busy" :aria-label="t('browser.selectionActions')" />
        </template>
        <v-list density="compact">
          <v-list-item
            v-for="action in selectionActions"
            :key="action.key"
            :title="action.text"
            :prepend-icon="action.icon"
            :base-color="action.danger ? 'error' : undefined"
            :disabled="busy"
            @click="action.click"
          />
        </v-list>
      </v-menu>
      <template v-else>
        <v-btn
          v-for="action in selectionActions"
          :key="action.key"
          :prepend-icon="action.icon"
          :text="action.text"
          :color="action.danger ? 'error' : undefined"
          :loading="action.spins && busy"
          :disabled="busy && !action.spins"
          @click="action.click"
        />
      </template>
    </v-toolbar>
    <slot v-else name="toolbar" />

    <div class="content">
      <EmptyState
        v-if="photos.failedToOpen"
        icon="fa-circle-exclamation"
        :title="t('browser.loadFailed')"
        :text="t('browser.loadFailedText')"
        danger
      >
        <template #actions>
          <v-btn :text="t('common.retry')" color="primary" @click="photos.reload()" />
        </template>
      </EmptyState>
      <slot v-else-if="photos.empty" name="empty" />
      <PhotoGrid
        v-else
        :sections="photos.sections"
        :by-month="photos.byMonth"
        :selected="selected"
        :selectable="selectable"
        @open="opened = $event"
        @toggle="toggle"
        @want="photos.want"
        @retry="photos.retry"
      />
    </div>

    <PhotoViewer
      v-if="opened"
      :photos="viewerPhotos"
      :photo="opened"
      :can-tag="viewerCanTag"
      :can-manage="viewerCanManage"
      :can-find-similar="props.mode !== 'trash'"
      :paused="dialogOpen"
      @close="opened = null"
      @navigate="opened = $event"
      @trash="trashOpened"
      @tags="openViewerTags"
      @share="openViewerSharing"
      @similar="findSimilar"
    />

    <TagDialog v-model="tagDialog" :photo-ids="tagTargets" @done="onTagged" />
    <ShareDialog v-model="shareDialog" :photo-ids="tagTargets" @done="onShared" />
    <AddToAlbumDialog v-model="albumDialog" :photo-ids="selectedIds" @done="onAddedToAlbum" />
    <ConfirmDialog
      v-model="purgeDialog"
      :title="t('browser.purgeTitle')"
      :text="t('browser.purgeText', selected.size)"
      :action="t('common.delete')"
      @confirm="purgeSelected"
    />
  </div>
</template>

<style scoped>
.browser {
  position: relative;
  display: flex;
  flex-direction: column;
  height: 100%;
}

/* The close icon lines up with the photos below (24px, 16px on a phone; the
   icon sits 16px inside its button), and the last action ends where they end. */
.selection-bar :deep(.v-toolbar__content) {
  padding-inline: 8px 24px;
}

.selection-bar :deep(.v-toolbar__content > .v-btn:first-child) {
  margin-inline-start: 0;
}

.selection-bar :deep(.v-toolbar__content > .v-btn:last-child) {
  margin-inline-end: 0;
}

@media (max-width: 599.98px) {
  .selection-bar :deep(.v-toolbar__content) {
    padding-inline: 0 16px;
  }
}

.content {
  display: flex;
  flex-direction: column;
  flex: 1;
  min-height: 0;
}
</style>
