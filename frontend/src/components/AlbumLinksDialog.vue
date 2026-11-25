<script setup lang="ts">
// The public links of an album: each one a copyable address, with its expiry and whether it has a
// password, and a form to make another. Revoking closes a link at once.
import { computed, ref, watch } from 'vue'

import { api, type AlbumLink } from '../api/client'
import { apiError } from '../i18n/apiErrors'
import AppDialog from './AppDialog.vue'
import AppFormGroup from './AppFormGroup.vue'
import ConfirmDialog from './ConfirmDialog.vue'
import CopyField from './CopyField.vue'
import { formatDate, t } from '../i18n'

const open = defineModel<boolean>({ required: true })
const props = defineProps<{ albumId: number }>()

const links = ref<AlbumLink[]>([])
const loaded = ref(false)
const error = ref('')
const saving = ref(false)

const expiry = ref<number | null>(null)
const password = ref('')
const revoking = ref<AlbumLink>()
const revokeOpen = ref(false)

const expiryChoices = computed(() => [
  { title: t('publicLinks.never'), value: null },
  ...[7, 30, 90, 365].map((n) => ({ title: t('publicLinks.days', { n }), value: n })),
])

// Without PUBLIC_URL on the server the address is built from the page the owner is on. That may be
// a home address that does not work from outside, which the dialog says.
const needsOrigin = computed(() => links.value.some((l) => !l.url))
function address(link: AlbumLink) {
  return link.url || location.origin + link.path
}

watch(open, async (isOpen) => {
  if (!isOpen) {
    return
  }
  loaded.value = false
  error.value = ''
  expiry.value = null
  password.value = ''
  const { data, error: failure } = await api.GET('/albums/{id}/links', { params: { path: { id: props.albumId } } })
  if (failure) {
    error.value = apiError(failure)
    return
  }
  links.value = data
  loaded.value = true
})

async function create() {
  saving.value = true
  const { data, error: failure } = await api.POST('/albums/{id}/links', {
    params: { path: { id: props.albumId } },
    body: { ...(expiry.value ? { expiresInDays: expiry.value } : {}), ...(password.value ? { password: password.value } : {}) },
  })
  saving.value = false
  if (failure) {
    error.value = apiError(failure)
    return
  }
  error.value = ''
  links.value = [...links.value, data]
  password.value = ''
}

function startRevoke(link: AlbumLink) {
  revoking.value = link
  revokeOpen.value = true
}

async function revoke() {
  const link = revoking.value
  if (!link) {
    return
  }
  const { error: failure } = await api.DELETE('/albums/{id}/links/{linkId}', { params: { path: { id: props.albumId, linkId: link.id } } })
  if (failure) {
    error.value = apiError(failure)
    return
  }
  links.value = links.value.filter((l) => l.id !== link.id)
}
</script>

<template>
  <AppDialog v-model="open" :title="t('publicLinks.title')" :error="error" :cancel="t('common.close')" :max-width="560">
    <p class="mb-4">{{ t('publicLinks.hint') }}</p>

    <div v-if="loaded && links.length === 0" class="text-medium-emphasis mb-4">{{ t('publicLinks.none') }}</div>
    <div v-for="link in links" :key="link.id" class="mb-4" data-testid="album-link">
      <CopyField :value="address(link)" :label="t('publicLinks.link')" />
      <div class="d-flex align-center text-medium-emphasis">
        <span class="flex-grow-1">
          {{ link.expiresAt ? t('publicLinks.expires', { date: formatDate(link.expiresAt) }) : t('publicLinks.neverExpires') }}
          <template v-if="link.hasPassword"> · {{ t('publicLinks.withPassword') }}</template>
        </span>
        <v-btn variant="text" size="small" color="error" :text="t('publicLinks.revoke')" @click="startRevoke(link)" />
      </div>
    </div>
    <div v-if="needsOrigin" class="text-medium-emphasis mb-4">{{ t('publicLinks.noPublicUrl') }}</div>

    <h3 class="mb-2">{{ t('publicLinks.new') }}</h3>
    <AppFormGroup v-slot="field" :label="t('publicLinks.expiry')">
      <v-select v-bind="field" v-model="expiry" :items="expiryChoices" />
    </AppFormGroup>
    <AppFormGroup v-slot="field" :label="t('publicLinks.password')">
      <v-text-field v-bind="field" v-model="password" type="password" autocomplete="new-password" :hint="t('publicLinks.passwordHint')" />
    </AppFormGroup>
    <v-btn color="primary" variant="flat" prepend-icon="fa-link" :loading="saving" :text="t('publicLinks.create')" data-testid="create-album-link" @click="create" />
  </AppDialog>
  <ConfirmDialog v-model="revokeOpen" :title="t('publicLinks.revokeTitle')" :text="t('publicLinks.revokeText')" :action="t('publicLinks.revoke')" @confirm="revoke" />
</template>
