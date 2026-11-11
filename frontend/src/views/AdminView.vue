<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'

import { api, type MetadataSettings, type PreviewSettings } from '../api/client'
import { apiError } from '../i18n/apiErrors'
import AppFormGroup from '../components/AppFormGroup.vue'
import AppPageHeader from '../components/AppPageHeader.vue'
import MLStatus from '../components/MLStatus.vue'
import QueueStatus from '../components/QueueStatus.vue'
import { useNoticesStore } from '../stores/notices'
import { t } from '../i18n'

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
    return (value !== '' && Number.isInteger(n) && n >= min && n <= max) || t('admin.range', { min, max })
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
    error.value = apiError(failure)
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
  const failure = previews.error ?? meta.error
  error.value = failure ? apiError(failure) : ''
  if (previews.data) {
    settings.value = previews.data
  }
  if (meta.data) {
    metadata.value = meta.data
  }
  if (previews.data && meta.data) {
    notices.notify(t('admin.saved'))
  }
}
</script>

<template>
  <div class="page">
    <AppPageHeader :title="t('admin.title')" :description="t('admin.description')" />
    <div class="page-section">

      <v-alert v-if="error" type="error" density="compact" class="mb-4" :title="error" />

      <v-form v-if="settings && metadata" class="settings" @submit.prevent="save">
        <h2 class="mb-2">{{ t('admin.previews') }}</h2>
        <v-alert
          type="info"
          density="compact"
          class="mb-6"
          :title="t('admin.newPhotos')"
          :text="t('admin.newPhotosText')"
        />

        <AppFormGroup v-slot="field" :label="t('admin.format')" required>
          <v-select v-bind="field" v-model="settings.format" variant="outlined" :items="formats" density="compact" />
        </AppFormGroup>

        <template v-if="settings.format === 'webp'">
          <AppFormGroup v-slot="field" :label="t('admin.webpQuality')" required>
            <v-text-field
              v-bind="field"
              v-model.number="settings.webpQuality"
              :rules="rules.quality"
              type="number"
              :hint="t('admin.qualityHint')"
              density="compact"
            />
          </AppFormGroup>
          <AppFormGroup v-slot="field" :label="t('admin.webpEffort')" required>
            <v-text-field
              v-bind="field"
              v-model.number="settings.webpEffort"
              :rules="rules.effort"
              type="number"
              :hint="t('admin.effortHint')"
              density="compact"
            />
          </AppFormGroup>
        </template>
        <AppFormGroup v-else v-slot="field" :label="t('admin.jpegQuality')" required>
          <v-text-field
            v-bind="field"
            v-model.number="settings.jpegQuality"
            :rules="rules.quality"
            type="number"
            :hint="t('admin.qualityHint')"
            density="compact"
          />
        </AppFormGroup>

        <AppFormGroup v-slot="field" :label="t('admin.gridEdge')" required>
          <v-text-field
            v-bind="field"
            v-model.number="settings.gridEdge"
            :rules="rules.gridEdge"
            data-testid="grid-edge"
            type="number"
            suffix="px"
            :hint="t('admin.gridEdgeHint')"
            density="compact"
          />
        </AppFormGroup>
        <AppFormGroup v-slot="field" :label="t('admin.viewerEdge')" required>
          <v-text-field
            v-bind="field"
            v-model.number="settings.viewerEdge"
            :rules="rules.viewerEdge"
            type="number"
            suffix="px"
            :hint="t('admin.viewerEdgeHint')"
            density="compact"
          />
        </AppFormGroup>

        <h2 class="mb-2">{{ t('profile.details') }}</h2>
        <v-switch
          v-model="metadata.showLocation"
          data-testid="show-location"
          :label="t('admin.showLocation')"
          :hint="t('admin.showLocationHint')"
          persistent-hint
          color="primary"
          density="compact"
          class="mb-4"
        />

        <v-btn type="submit" color="primary" :text="t('common.save')" :disabled="!valid" :loading="saving" />
      </v-form>

      <MLStatus class="mt-10" />
      <QueueStatus class="mt-10" />
    </div>
  </div>
</template>

<style scoped>
.settings {
  max-width: 600px;
}
</style>
