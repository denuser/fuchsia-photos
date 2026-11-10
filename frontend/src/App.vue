<script setup lang="ts">
import { computed, provide, ref, watch, watchEffect } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { useDisplay, useTheme } from 'vuetify'

import logo from './assets/logo.png'
import AppearanceMenu from './components/AppearanceMenu.vue'
import AppToasts from './components/AppToasts.vue'
import UploadPanel from './components/UploadPanel.vue'
import { useAppearanceStore } from './stores/appearance'
import { useAuthStore } from './stores/auth'
import { usePhotosStore } from './stores/photos'
import { pickUploadsKey, useUploadsStore } from './stores/uploads'

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
  { to: '/', icon: 'fa-images', title: 'Photos', exact: true },
  { to: '/search', icon: 'fa-magnifying-glass', title: 'Search' },
  { to: '/albums', icon: 'fa-book-open', title: 'Albums' },
  { to: '/shared', icon: 'fa-users', title: 'Shared with me' },
  { to: '/tags', icon: 'fa-tags', title: 'Tags' },
  { to: '/trash', icon: 'fa-trash-can', title: 'Trash' },
  ...(auth.isAdmin ? [{ to: '/all', icon: 'fa-user-shield', title: 'All photos' }] : []),
])
const fileInput = ref<HTMLInputElement>()
provide(pickUploadsKey, () => fileInput.value?.click())

function chosen(event: Event) {
  const input = event.target as HTMLInputElement
  uploads.add(Array.from(input.files ?? []))
  input.value = ''
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
          :aria-label="navShownInFull ? 'Collapse menu' : 'Expand menu'"
          :aria-expanded="navShownInFull"
          @click="toggleNav"
        />
      </template>
      <v-app-bar-title>
        <router-link to="/" class="brand">
          <img :src="logo" alt="" class="brand-logo" />
          Fuchsia Photos
        </router-link>
      </v-app-bar-title>

      <v-chip v-if="photos.pending > 0" class="mr-2" size="small" prepend-icon="fa-gear">
        Processing {{ photos.pending }}
      </v-chip>

      <v-btn
        v-if="xs"
        color="primary"
        icon="fa-upload"
        aria-label="Upload"
        @click="fileInput?.click()"
      />
      <v-btn v-else color="primary" variant="flat" prepend-icon="fa-upload" class="mr-2" @click="fileInput?.click()">
        Upload
      </v-btn>
      <input
        ref="fileInput"
        type="file"
        multiple
        accept="image/jpeg,image/png,image/webp,image/heic,image/heif,.heic,.heif"
        hidden
        @change="chosen"
      />

      <AppearanceMenu />

      <v-menu>
        <template #activator="{ props }">
          <v-btn v-bind="props" icon="fa-circle-user" aria-label="Account" />
        </template>
        <v-list density="compact">
          <v-list-item :subtitle="auth.isAdmin ? 'Administrator' : 'User'" :title="auth.user.login" />
          <v-divider />
          <v-list-item v-if="auth.isAdmin" to="/users" prepend-icon="fa-users" title="Users" />
          <v-list-item v-if="auth.isAdmin" to="/import" prepend-icon="fa-file-import" title="Import from folder" />
          <v-list-item v-if="auth.isAdmin" to="/admin" prepend-icon="fa-gear" title="Administration" />
          <v-list-item to="/profile" prepend-icon="fa-user-gear" title="Profile" />
          <v-list-item prepend-icon="fa-right-from-bracket" title="Log out" @click="logout" />
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
