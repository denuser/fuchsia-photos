<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { useDisplay } from 'vuetify'

import logo from './assets/logo.png'
import UploadPanel from './components/UploadPanel.vue'
import { useAuthStore } from './stores/auth'
import { usePhotosStore } from './stores/photos'
import { useUploadsStore } from './stores/uploads'

const auth = useAuthStore()
const photos = usePhotosStore()
const uploads = useUploadsStore()
const route = useRoute()
const router = useRouter()
// On a phone the upload button is an icon, to leave room in the app bar.
const { xs } = useDisplay()

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
  { to: '/', icon: 'mdi-image-multiple-outline', title: 'Photos', exact: true },
  { to: '/albums', icon: 'mdi-image-album', title: 'Albums' },
  { to: '/shared', icon: 'mdi-account-multiple-outline', title: 'Shared with me' },
  { to: '/tags', icon: 'mdi-tag-multiple-outline', title: 'Tags' },
  { to: '/trash', icon: 'mdi-trash-can-outline', title: 'Trash' },
  ...(auth.isAdmin ? [{ to: '/all', icon: 'mdi-shield-account-outline', title: 'All photos' }] : []),
])
const fileInput = ref<HTMLInputElement>()

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
    <v-app-bar v-if="auth.user" density="comfortable" flat border="b">
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

      <v-chip v-if="photos.pending > 0" class="mr-2" size="small" prepend-icon="mdi-cog-outline">
        Processing {{ photos.pending }}
      </v-chip>

      <v-btn
        v-if="xs"
        color="primary"
        icon="mdi-upload"
        aria-label="Upload"
        @click="fileInput?.click()"
      />
      <v-btn v-else color="primary" variant="flat" prepend-icon="mdi-upload" class="mr-2" @click="fileInput?.click()">
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

      <v-menu>
        <template #activator="{ props }">
          <v-btn v-bind="props" icon="mdi-account-circle" aria-label="Account" />
        </template>
        <v-list density="compact">
          <v-list-item :subtitle="auth.isAdmin ? 'Administrator' : 'User'" :title="auth.user.login" />
          <v-divider />
          <v-list-item v-if="auth.isAdmin" to="/users" prepend-icon="mdi-account-multiple" title="Users" />
          <v-list-item v-if="auth.isAdmin" to="/import" prepend-icon="mdi-folder-arrow-down-outline" title="Import from folder" />
          <v-list-item v-if="auth.isAdmin" to="/admin" prepend-icon="mdi-cog-outline" title="Administration" />
          <v-list-item to="/profile" prepend-icon="mdi-account-cog-outline" title="Profile" />
          <v-list-item prepend-icon="mdi-logout" title="Log out" @click="logout" />
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
      <router-view :key="$route.path" />
    </v-main>

    <UploadPanel v-if="auth.user" />
  </v-app>
</template>

<style>
html {
  overflow: hidden;
}

.brand {
  display: flex;
  align-items: center;
  gap: 8px;
  color: inherit;
  text-decoration: none;
}

.brand-logo {
  width: 32px;
  height: 32px;
}
</style>
