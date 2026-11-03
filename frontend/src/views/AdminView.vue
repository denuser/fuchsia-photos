<script setup lang="ts">
import { computed, nextTick, onMounted, ref, watch } from 'vue'

import { api, type MetadataSettings, type PreviewSettings } from '../api/client'

const formats = [
  { title: 'WebP', value: 'webp' },
  { title: 'JPEG', value: 'jpeg' },
]

const settings = ref<PreviewSettings | null>(null)
const metadata = ref<MetadataSettings | null>(null)
const error = ref('')
const saved = ref(false)
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

watch([settings, metadata], () => (saved.value = false), { deep: true })

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
    // The new values also trigger the watcher above, which must run first.
    await nextTick()
    saved.value = true
  }
}
</script>

<template>
  <v-container class="admin">
    <h1 class="text-h5 mb-4">Administration</h1>

    <v-alert v-if="error" type="error" density="compact" class="mb-4" :text="error" />

    <v-form v-if="settings && metadata" @submit.prevent="save">
      <h2 class="text-h6 mb-2">Previews</h2>
      <v-alert
        type="info"
        variant="tonal"
        density="compact"
        class="mb-6"
        text="These settings apply to new photos. Photos that are already processed keep their previews."
      />

      <v-select v-model="settings.format" :items="formats" label="Preview format" density="compact" class="mb-2" />

      <template v-if="settings.format === 'webp'">
        <v-text-field
          v-model.number="settings.webpQuality"
          :rules="rules.quality"
          type="number"
          label="WebP quality"
          hint="From 1 to 100. Higher looks better and takes more space."
          persistent-hint
          density="compact"
          class="mb-2"
        />
        <v-text-field
          v-model.number="settings.webpEffort"
          :rules="rules.effort"
          type="number"
          label="WebP compression effort"
          hint="From 0 to 6. Higher makes smaller files, but takes longer."
          persistent-hint
          density="compact"
          class="mb-2"
        />
      </template>
      <v-text-field
        v-else
        v-model.number="settings.jpegQuality"
        :rules="rules.quality"
        type="number"
        label="JPEG quality"
        hint="From 1 to 100. Higher looks better and takes more space."
        persistent-hint
        density="compact"
        class="mb-2"
      />

      <v-text-field
        v-model.number="settings.gridEdge"
        :rules="rules.gridEdge"
        data-testid="grid-edge"
        type="number"
        label="Grid preview size"
        suffix="px"
        hint="Longest side, from 200 to 800 pixels."
        persistent-hint
        density="compact"
        class="mb-2"
      />
      <v-text-field
        v-model.number="settings.viewerEdge"
        :rules="rules.viewerEdge"
        type="number"
        label="Viewer preview size"
        suffix="px"
        hint="Longest side, from 1024 to 4096 pixels."
        persistent-hint
        density="compact"
        class="mb-6"
      />

      <h2 class="text-h6 mb-2">Photo details</h2>
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

      <div class="d-flex align-center ga-4">
        <v-btn type="submit" color="primary" text="Save" :disabled="!valid" :loading="saving" />
        <span v-if="saved" class="text-body-2 text-medium-emphasis">Saved</span>
      </div>
    </v-form>
  </v-container>
</template>

<style scoped>
.admin {
  height: calc(100vh - var(--v-layout-top, 0px));
  overflow-y: auto;
  max-width: 600px;
}
</style>
