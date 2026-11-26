<script setup lang="ts">
import { computed, provide, ref, watch, watchEffect } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { useDisplay, useTheme } from 'vuetify'

import logo from './assets/logo.png'
import AppearanceMenu from './components/AppearanceMenu.vue'
import AppToasts from './components/AppToasts.vue'
import UploadDialog, { type AlbumChoice } from './components/UploadDialog.vue'
import UploadPanel from './components/UploadPanel.vue'
import { api } from './api/client'
import { apiError } from './i18n/apiErrors'
import { useNoticesStore } from './stores/notices'
import { ACCEPT, sift } from './lib/uploadFiles'
import { useAppearanceStore } from './stores/appearance'
import { useAuthStore } from './stores/auth'
import { usePhotosStore } from './stores/photos'
import { pickUploadsKey, useUploadsStore, type UploadAlbum } from './stores/uploads'
import { t } from './i18n'

const auth = useAuthStore()
const photos = usePhotosStore()
const uploads = useUploadsStore()
const appearance = useAppearanceStore()
const theme = useTheme()
const route = useRoute()
const router = useRouter()
// On a phone the upload button is an icon, to leave room in the app bar.
const { xs } = useDisplay()

watchEffect(() => theme.change(appearance.theme))

const navExpanded = ref(localStorage.getItem('navExpanded') === 'true')

watch(navExpanded, (value) => localStorage.setItem('navExpanded', String(value)))

// On a phone the navigation can stay hidden until the menu button opens it
// over the page, as a profile setting chooses. Elsewhere it is a permanent rail.
const navOverlay = computed(() => xs.value && (auth.user?.navigationHiddenOnPhones ?? true))
const navOpen = ref(false)

watch(
  () => route.fullPath,
  () => (navOpen.value = false),
)

function toggleNav() {
  if (navOverlay.value) {
    navOpen.value = !navOpen.value
  } else {
    navExpanded.value = !navExpanded.value
  }
}

const navShownInFull = computed(() => (navOverlay.value ? navOpen.value : navExpanded.value))

const navItems = computed(() => [
  { to: '/', icon: 'fa-images', title: t('nav.photos'), exact: true },
  { to: '/search', icon: 'fa-magnifying-glass', title: t('nav.search') },
  { to: '/albums', icon: 'fa-book-open', title: t('nav.albums') },
  { to: '/memories', icon: 'fa-clock-rotate-left', title: t('nav.memories') },
  { to: '/map', icon: 'fa-map-location-dot', title: t('nav.map') },
  { to: '/favorites', icon: 'fa-star', title: t('nav.favorites') },
  { to: '/shared', icon: 'fa-users', title: t('nav.shared') },
  { to: '/tags', icon: 'fa-tags', title: t('nav.tags') },
  { to: '/people', icon: 'fa-users', title: t('people.title') },
  { to: '/similar', icon: 'fa-clone', title: t('nav.similar') },
  { to: '/archive', icon: 'fa-box-archive', title: t('nav.archive') },
  { to: '/trash', icon: 'fa-trash-can', title: t('nav.trash') },
  ...(auth.isAdmin ? [{ to: '/all', icon: 'fa-user-shield', title: t('nav.all') }] : []),
])
const fileInput = ref<HTMLInputElement>()
const folderInput = ref<HTMLInputElement>()
provide(pickUploadsKey, () => uploads.openDialog())

// The album chosen in the upload dialog, for the files the picker hands over
// next. Files set on the input any other way go into the library only.
let picked: AlbumChoice = null

function pick(choice: AlbumChoice, folder: boolean) {
  picked = choice
  const input = folder ? folderInput.value : fileInput.value
  input?.click()
}

async function chosen(event: Event) {
  const input = event.target as HTMLInputElement
  const all = Array.from(input.files ?? [])
  const { files, skipped } = input === folderInput.value ? sift(all) : { files: all, skipped: [] }
  input.value = ''
  const choice = picked
  picked = null
  const album = await albumFor(choice)
  if (album !== undefined) {
    uploads.add(files, album, skipped)
  }
}

// Creates a new album when a name was typed. Undefined means it failed and
// nothing is uploaded, rather than the files landing outside the album.
async function albumFor(choice: AlbumChoice): Promise<UploadAlbum | null | undefined> {
  if (typeof choice !== 'string') {
    return choice
  }
  const { data, error } = await api.POST('/albums', { body: { name: choice } })
  if (error) {
    useNoticesStore().notify(t('uploadDialog.createFailed', { error: apiError(error) }), 'error')
    return undefined
  }
  return { id: data.id, name: data.name }
}

async function logout() {
  await auth.logout()
  photos.reset()
  await router.push('/login')
}

window.addEventListener('beforeunload', (event) => {
  if (uploads.running) {
    event.preventDefault()
  }
})
</script>

<template>
  <v-app>
    <v-app-bar v-if="auth.user" density="comfortable">
      <template #prepend>
        <v-app-bar-nav-icon
          :aria-label="navShownInFull ? t('nav.collapse') : t('nav.expand')"
          :aria-expanded="navShownInFull"
          @click="toggleNav"
        />
      </template>
      <v-app-bar-title>
        <router-link to="/" class="brand">
          <img :src="logo" alt="" class="brand-logo" />
          Magellanica Photos
        </router-link>
      </v-app-bar-title>

      <v-chip v-if="photos.pending > 0" class="mr-2" size="small" prepend-icon="fa-gear">
        {{ t('nav.processing', { n: photos.pending }) }}
      </v-chip>

      <v-btn
        v-if="xs"
        color="primary"
        icon="fa-upload"
        :aria-label="t('nav.upload')"
        @click="uploads.openDialog()"
      />
      <v-btn v-else color="primary" variant="flat" prepend-icon="fa-upload" class="mr-2" @click="uploads.openDialog()">
        {{ t('nav.upload') }}
      </v-btn>
      <input
        ref="fileInput"
        data-testid="upload-files"
        type="file"
        multiple
        :accept="ACCEPT"
        hidden
        @change="chosen"
      />
      <input ref="folderInput" data-testid="upload-folder" type="file" webkitdirectory hidden @change="chosen" />

      <AppearanceMenu />

      <v-menu>
        <template #activator="{ props }">
          <v-btn v-bind="props" icon="fa-circle-user" :aria-label="t('nav.account')" />
        </template>
        <v-list density="compact">
          <v-list-item :subtitle="auth.isAdmin ? t('roles.admin') : t('roles.user')" :title="auth.user.login" />
          <v-divider />
          <v-list-item v-if="auth.isAdmin" to="/users" prepend-icon="fa-users" :title="t('users.title')" />
          <v-list-item v-if="auth.isAdmin" to="/activity" prepend-icon="fa-clock-rotate-left" :title="t('activity.title')" />
          <v-list-item v-if="auth.isAdmin" to="/import" prepend-icon="fa-file-import" :title="t('import.title')" />
          <v-list-item v-if="auth.isAdmin" to="/library-folders" prepend-icon="fa-folder-open" :title="t('library.title')" />
          <v-list-item v-if="auth.isAdmin" to="/admin" prepend-icon="fa-gear" :title="t('admin.title')" />
          <v-list-item to="/profile" prepend-icon="fa-user-gear" :title="t('nav.profile')" />
          <v-list-item prepend-icon="fa-right-from-bracket" :title="t('nav.logout')" @click="logout" />
        </v-list>
      </v-menu>
    </v-app-bar>

    <v-navigation-drawer
      v-if="auth.user"
      :model-value="navOverlay ? navOpen : true"
      :permanent="!navOverlay"
      :temporary="navOverlay"
      :rail="!navOverlay && !navExpanded"
      @update:model-value="navOpen = $event"
    >
      <v-list nav density="compact">
        <v-list-item
          v-for="item in navItems"
          :key="item.to"
          :to="item.to"
          :exact="item.exact"
          :prepend-icon="item.icon"
          :title="item.title"
          :aria-label="item.title"
        >
          <v-tooltip activator="parent" location="end" :disabled="navShownInFull">{{ item.title }}</v-tooltip>
        </v-list-item>
      </v-list>
    </v-navigation-drawer>

    <v-main>
      <div v-if="auth.user" class="page-panel">
        <router-view :key="$route.path" />
      </div>
      <router-view v-else :key="$route.path" />
    </v-main>

    <UploadPanel v-if="auth.user" />
    <UploadDialog v-if="auth.user" @pick="pick" />
    <AppToasts />
  </v-app>
</template>

<style>
html {
  overflow: hidden;
}

.brand {
  display: inline-flex;
  align-items: center;
  gap: 8px;
  padding: 4px;
  border-radius: var(--radius-small);
  color: inherit;
  text-decoration: none;
}

/* The app bar title clips what sticks out of it, so the focus ring is drawn inside. */
.brand:focus-visible {
  outline-offset: -2px;
}

.brand-logo {
  width: 32px;
  height: 32px;
}
</style>
