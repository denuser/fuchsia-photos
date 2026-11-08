<script setup lang="ts">
import { computed, ref } from 'vue'

import { api, type ViewerField } from '../api/client'
import AppPageHeader from '../components/AppPageHeader.vue'
import ChangePasswordDialog from '../components/ChangePasswordDialog.vue'
import { viewerFields } from '../lib/viewerFields'
import { useAuthStore } from '../stores/auth'

const auth = useAuthStore()

const passwordDialog = ref(false)
const saving = ref(false)
const error = ref('')

const shown = computed(() => viewerFields.map((f) => f.value).filter((f) => !auth.user?.viewerHiddenFields.includes(f)))

type Changes = { viewerShowsOriginals?: boolean; viewerHiddenFields?: ViewerField[]; navigationHiddenOnPhones?: boolean; gridSquares?: boolean }

// Each change is saved at once, like the switch on the "Shared with me" page.
async function save(body: Changes) {
  saving.value = true
  error.value = ''
  const { data, error: failure } = await api.PATCH('/me', { body })
  saving.value = false
  if (!data) {
    error.value = failure?.error ?? 'The setting could not be saved'
    return
  }
  auth.user = data
}

function setShown(value: ViewerField[] | null) {
  const visible = value ?? []
  void save({ viewerHiddenFields: viewerFields.map((f) => f.value).filter((f) => !visible.includes(f)) })
}
</script>

<template>
  <div v-if="auth.user" class="page">
    <AppPageHeader :title="auth.user.login" :description="auth.isAdmin ? 'Administrator' : 'User'" />
    <div class="page-section profile">

      <v-alert v-if="error" type="error" density="compact" class="mb-4" :title="error" />

      <h2 class="mb-2">Photo grid</h2>
      <v-switch
        :model-value="auth.user.gridSquares"
        :disabled="saving"
        label="Show square previews"
        hint="All previews are the same size, cropped to a square. The size follows the grid preview size an administrator sets."
        persistent-hint
        color="primary"
        density="compact"
        class="mb-6"
        @update:model-value="save({ gridSquares: Boolean($event) })"
      />

      <h2 class="mb-2">Viewer</h2>
      <v-switch
        :model-value="auth.user.viewerShowsOriginals"
        :disabled="saving"
        label="Load originals when zooming in"
        hint="Zooming in then shows the photo at full resolution, otherwise it enlarges the preview. Originals can be much larger. Photos the browser cannot show, such as HEIC outside Safari, keep their previews."
        persistent-hint
        color="primary"
        density="compact"
        class="mb-4"
        @update:model-value="save({ viewerShowsOriginals: Boolean($event) })"
      />

      <h3 class="mb-1">Photo details</h3>
      <div class="text-medium-emphasis mb-2">
        Shown under the photo name when the photo has them. A location is shown only if an administrator allows it.
      </div>
      <div class="fields mb-6">
        <v-checkbox
          v-for="field in viewerFields"
          :key="field.value"
          :model-value="shown"
          :value="field.value"
          :label="field.title"
          :disabled="saving"
          density="compact"
          hide-details
          @update:model-value="setShown"
        />
      </div>

      <h2 class="mb-2">Phone</h2>
      <v-switch
        :model-value="auth.user.navigationHiddenOnPhones"
        :disabled="saving"
        label="Hide the menu on a phone"
        hint="The menu button opens it over the page, so the photos take the whole width of the screen."
        persistent-hint
        color="primary"
        density="compact"
        class="mb-6"
        @update:model-value="save({ navigationHiddenOnPhones: Boolean($event) })"
      />

      <h2 class="mb-2">Password</h2>
      <v-btn prepend-icon="mdi-lock-reset" variant="tonal" text="Change password" @click="passwordDialog = true" />
      <ChangePasswordDialog v-model="passwordDialog" />
    </div>
  </div>
</template>

<style scoped>
.profile {
  max-width: 768px;
}

.fields {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(220px, 1fr));
}
</style>
