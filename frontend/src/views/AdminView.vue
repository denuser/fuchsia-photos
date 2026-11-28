<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'

import { api, type FaceSettings, type MetadataSettings, type PreviewSettings, type SignInOptions, type SignInSettings, type VideoSettings } from '../api/client'
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
const video = ref<VideoSettings | null>(null)
const faces = ref<FaceSettings | null>(null)
// Saving the face setting regroups every owner's faces, so it is sent only when it changed.
const savedRetryMinScore = ref(0)
const signIn = ref<SignInSettings | null>(null)
const signInOptions = ref<SignInOptions | null>(null)

const transcodeModes = computed(() =>
  (['view', 'upload', 'off'] as const).map((value) => ({ value, title: t(`admin.transcode.${value}`) })),
)
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

const retryScoreRule = (value: unknown) => {
  const n = Number(value)
  const min = faces.value?.retryFloor ?? 0.01
  return (value !== '' && n >= min && n < 1) || t('admin.range', { min, max: 0.99 })
}

const valid = computed(() => {
  const s = settings.value
  if (!s || !faces.value || retryScoreRule(faces.value.retryMinScore) !== true) {
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
  const [previews, meta, videos, signing, options, face] = await Promise.all([
    api.GET('/settings/previews'),
    api.GET('/settings/metadata'),
    api.GET('/settings/video'),
    api.GET('/settings/sign-in'),
    api.GET('/auth/options'),
    api.GET('/settings/faces'),
  ])
  const failure = previews.error ?? meta.error ?? videos.error ?? signing.error ?? options.error ?? face.error
  if (failure) {
    error.value = apiError(failure)
    return
  }
  settings.value = previews.data!
  metadata.value = meta.data!
  video.value = videos.data!
  signIn.value = signing.data!
  signInOptions.value = options.data!
  faces.value = face.data!
  savedRetryMinScore.value = face.data!.retryMinScore
})

async function save() {
  saving.value = true
  const retryMinScore = Number(faces.value!.retryMinScore)
  const [previews, meta, videos, signing, face] = await Promise.all([
    api.PUT('/settings/previews', { body: settings.value! }),
    api.PUT('/settings/metadata', { body: metadata.value! }),
    api.PUT('/settings/video', { body: { transcode: video.value!.transcode } }),
    api.PUT('/settings/sign-in', { body: signIn.value! }),
    retryMinScore === savedRetryMinScore.value
      ? Promise.resolve({ data: faces.value!, error: undefined })
      : api.PUT('/settings/faces', { body: { retryMinScore } }),
  ])
  saving.value = false
  const failure = previews.error ?? meta.error ?? videos.error ?? signing.error ?? face.error
  error.value = failure ? apiError(failure) : ''
  if (previews.data) {
    settings.value = previews.data
  }
  if (meta.data) {
    metadata.value = meta.data
  }
  if (videos.data) {
    video.value = videos.data
  }
  if (signing.data) {
    signIn.value = signing.data
  }
  if (face.data) {
    faces.value = face.data
    savedRetryMinScore.value = face.data.retryMinScore
  }
  if (previews.data && meta.data && videos.data && signing.data && face.data) {
    notices.notify(t('admin.saved'))
  }
}
</script>

<template>
  <div class="page">
    <AppPageHeader :title="t('admin.title')" :description="t('admin.description')" />
    <div class="page-section">

      <v-alert v-if="error" type="error" density="compact" class="mb-4" :title="error" />

      <v-form v-if="settings && metadata && video && faces && signIn && signInOptions" class="settings" @submit.prevent="save">
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

        <h2 class="mb-2">{{ t('admin.video') }}</h2>
        <v-alert
          v-if="video.hardware"
          type="info"
          density="compact"
          class="mb-6"
          data-testid="video-hardware"
          :title="t('admin.videoHardware', { device: video.hardware.device })"
          :text="video.hardware.driver + (video.hardware.lowPower ? ` · ${t('admin.videoLowPower')}` : '')"
        />
        <v-alert
          v-else
          type="warning"
          density="compact"
          class="mb-6"
          data-testid="video-hardware"
          :title="t('admin.videoNoHardware')"
          :text="video.unavailable"
        />
        <AppFormGroup v-slot="field" :label="t('admin.transcodeLabel')" required>
          <v-select
            v-bind="field"
            v-model="video.transcode"
            variant="outlined"
            :items="transcodeModes"
            :hint="t('admin.transcodeHint')"
            persistent-hint
            density="compact"
            data-testid="transcode"
          />
        </AppFormGroup>

        <h2 class="mb-2">{{ t('admin.faces') }}</h2>
        <AppFormGroup v-slot="field" :label="t('admin.retryMinScore')" required>
          <v-text-field
            v-bind="field"
            v-model.number="faces.retryMinScore"
            :rules="[retryScoreRule]"
            type="number"
            step="0.05"
            data-testid="retry-min-score"
            :hint="t('admin.retryMinScoreHint') + (faces.retryFloor ? ' ' + t('admin.retryFloor', { floor: faces.retryFloor }) : '')"
            persistent-hint
            density="compact"
          />
        </AppFormGroup>

        <h2 class="mb-2">{{ t('admin.signIn') }}</h2>
        <v-alert
          v-if="signInOptions.providers.length === 0"
          type="info"
          density="compact"
          class="mb-6"
          :title="t('admin.noProviders')"
          :text="t('admin.noProvidersText')"
        />
        <v-switch
          v-model="signIn.createUsers"
          data-testid="create-users"
          :label="t('admin.createUsers')"
          :hint="t('admin.createUsersHint')"
          persistent-hint
          color="primary"
          density="compact"
          class="mb-4"
        />
        <template v-if="signIn.createUsers">
          <AppFormGroup v-slot="field" :label="t('admin.allowed')">
            <v-combobox
              v-bind="field"
              v-model="signIn.allowed"
              multiple
              chips
              closable-chips
              hide-no-data
              variant="outlined"
              density="compact"
              :hint="t('admin.allowedHint')"
              persistent-hint
              data-testid="allowed"
            />
          </AppFormGroup>
          <v-alert
            v-if="signIn.allowed.length === 0"
            type="warning"
            density="compact"
            class="mb-4"
            :title="t('admin.openRegistration')"
            :text="t('admin.openRegistrationText')"
          />
        </template>
        <v-switch
          v-model="signIn.passwordAdminsOnly"
          data-testid="password-admins-only"
          :label="t('admin.passwordAdminsOnly')"
          :hint="t('admin.passwordAdminsOnlyHint')"
          persistent-hint
          color="primary"
          density="compact"
          class="mb-6"
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
