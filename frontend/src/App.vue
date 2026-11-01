<script setup lang="ts">
import { ref } from 'vue'
import { useRouter } from 'vue-router'

import ChangePasswordDialog from './components/ChangePasswordDialog.vue'
import UploadPanel from './components/UploadPanel.vue'
import { useAuthStore } from './stores/auth'
import { usePhotosStore } from './stores/photos'
import { useUploadsStore } from './stores/uploads'

const auth = useAuthStore()
const photos = usePhotosStore()
const uploads = useUploadsStore()
const router = useRouter()

const passwordDialog = ref(false)
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
      <v-app-bar-title>
        <router-link to="/" class="brand">Fuchsia Photos</router-link>
      </v-app-bar-title>

      <v-chip v-if="photos.pending > 0" class="mr-2" size="small" prepend-icon="mdi-cog-outline">
        Processing {{ photos.pending }}
      </v-chip>

      <v-btn color="primary" variant="flat" prepend-icon="mdi-upload" class="mr-2" @click="fileInput?.click()">
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
          <v-list-item prepend-icon="mdi-lock-reset" title="Change password" @click="passwordDialog = true" />
          <v-list-item prepend-icon="mdi-logout" title="Log out" @click="logout" />
        </v-list>
      </v-menu>
    </v-app-bar>

    <v-navigation-drawer v-if="auth.user" permanent rail>
      <v-list nav density="compact">
        <v-list-item to="/" exact prepend-icon="mdi-image-multiple-outline" title="Photos" aria-label="Photos" />
        <v-list-item to="/albums" prepend-icon="mdi-image-album" title="Albums" aria-label="Albums" />
        <v-list-item to="/shared" prepend-icon="mdi-account-multiple-outline" title="Shared with me" aria-label="Shared with me" />
        <v-list-item to="/tags" prepend-icon="mdi-tag-multiple-outline" title="Tags" aria-label="Tags" />
        <v-list-item to="/trash" prepend-icon="mdi-trash-can-outline" title="Trash" aria-label="Trash" />
        <v-list-item v-if="auth.isAdmin" to="/all" prepend-icon="mdi-shield-account-outline" title="All photos" aria-label="All photos" />
      </v-list>
    </v-navigation-drawer>

    <v-main>
      <router-view :key="$route.path" />
    </v-main>

    <UploadPanel v-if="auth.user" />
    <ChangePasswordDialog v-model="passwordDialog" />
  </v-app>
</template>

<style>
html {
  overflow: hidden;
}

.brand {
  color: inherit;
  text-decoration: none;
}
</style>
