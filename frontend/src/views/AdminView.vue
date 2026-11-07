<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'

import { api, type MetadataSettings, type PreviewSettings } from '../api/client'
import AppFormGroup from '../components/AppFormGroup.vue'
import { useNoticesStore } from '../stores/notices'

const formats = [
  { title: 'WebP', value: 'webp' },
  { title: 'JPEG', value: 'jpeg' },
]

const notices = useNoticesStore()

const settings = ref<PreviewSettings | null>(null)
const metadata = ref<MetadataSettings | null>(null)
const error = ref('')
const saving = ref(false)

function range(min: number, max: number) {
  return (value: unknown) => {
    const n = Number(value)
    return (value !== '' && Number.isInteger(n) && n >= min && n <= max) || `Enter a whole number from ${min} to ${max}`
  }
}

const rules = {
  quality: [range(1, 100)],
  effort: [range(0, 6)],
  gridEdge: [range(200, 800)],
  viewerEdge: [range(1024, 4096)],
}

const valid = computed(() => {
  const s = settings.value
  if (!s) {
    return false
  }
  const checks: [unknown, (value: unknown) => true | string][] = [
    [s.webpQuality, rules.quality[0]],
    [s.webpEffort, rules.effort[0]],
    [s.jpegQuality, rules.quality[0]],
    [s.gridEdge, rules.gridEdge[0]],
    [s.viewerEdge, rules.viewerEdge[0]],
  ]
  return checks.every(([value, rule]) => rule(value) === true)
})

onMounted(async () => {
  const [previews, meta] = await Promise.all([api.GET('/settings/previews'), api.GET('/settings/metadata')])
  const failure = previews.error ?? meta.error
  if (failure) {
    error.value = failure.error
    return
  }
  settings.value = previews.data!
  metadata.value = meta.data!
})

async function save() {
  saving.value = true
  const [previews, meta] = await Promise.all([
    api.PUT('/settings/previews', { body: settings.value! }),
    api.PUT('/settings/metadata', { body: metadata.value! }),
  ])
  saving.value = false
  error.value = (previews.error ?? meta.error)?.error ?? ''
  if (previews.data) {
    settings.value = previews.data
  }
  if (meta.data) {
    metadata.value = meta.data
  }
  if (previews.data && meta.data) {
    notices.notify('Settings saved')
  }
}
</script>

<template>
  <v-container class="admin">
    <h1 class="mb-4">Administration</h1>

    <v-alert v-if="error" type="error" density="compact" class="mb-4" :title="error" />

    <v-form v-if="settings && metadata" @submit.prevent="save">
      <h2 class="mb-2">Previews</h2>
      <v-alert
        type="info"
        density="compact"
        class="mb-6"
        title="These settings apply to new photos"
        text="Photos that are already processed keep their previews."
      />

      <AppFormGroup v-slot="field" label="Preview format" required>
        <v-select v-bind="field" v-model="settings.format" variant="outlined" :items="formats" density="compact" />
      </AppFormGroup>

      <template v-if="settings.format === 'webp'">
        <AppFormGroup v-slot="field" label="WebP quality" required>
          <v-text-field
            v-bind="field"
            v-model.number="settings.webpQuality"
            :rules="rules.quality"
            type="number"
            hint="From 1 to 100. Higher looks better and takes more space."
            density="compact"
          />
        </AppFormGroup>
        <AppFormGroup v-slot="field" label="WebP compression effort" required>
          <v-text-field
            v-bind="field"
            v-model.number="settings.webpEffort"
            :rules="rules.effort"
            type="number"
            hint="From 0 to 6. Higher makes smaller files, but takes longer."
            density="compact"
          />
        </AppFormGroup>
      </template>
      <AppFormGroup v-else v-slot="field" label="JPEG quality" required>
        <v-text-field
          v-bind="field"
          v-model.number="settings.jpegQuality"
          :rules="rules.quality"
          type="number"
          hint="From 1 to 100. Higher looks better and takes more space."
          density="compact"
        />
      </AppFormGroup>

      <AppFormGroup v-slot="field" label="Grid preview size" required>
        <v-text-field
          v-bind="field"
          v-model.number="settings.gridEdge"
          :rules="rules.gridEdge"
          data-testid="grid-edge"
          type="number"
          suffix="px"
          hint="Shortest side, from 200 to 800 pixels."
          density="compact"
        />
      </AppFormGroup>
      <AppFormGroup v-slot="field" label="Viewer preview size" required>
        <v-text-field
          v-bind="field"
          v-model.number="settings.viewerEdge"
          :rules="rules.viewerEdge"
          type="number"
          suffix="px"
          hint="Longest side, from 1024 to 4096 pixels."
          density="compact"
        />
      </AppFormGroup>

      <h2 class="mb-2">Photo details</h2>
      <v-switch
        v-model="metadata.showLocation"
        data-testid="show-location"
        label="Show where photos were taken"
        hint="Shows the GPS location in the viewer to everyone who can see the photo, including users it is shared with. Downloaded originals always keep it."
        persistent-hint
        color="primary"
        density="compact"
        class="mb-4"
      />

      <v-btn type="submit" color="primary" text="Save" :disabled="!valid" :loading="saving" />
    </v-form>
  </v-container>
</template>

<style scoped>
.admin {
  height: 100%;
  overflow-y: auto;
  max-width: 600px;
}
</style>
