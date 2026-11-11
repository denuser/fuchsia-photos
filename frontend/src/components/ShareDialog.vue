<script setup lang="ts">
import { computed, ref, shallowRef, watch } from 'vue'

import { api, type UserRef, type Visibility } from '../api/client'
import { inBatches } from '../stores/photos'
import { useUsersStore } from '../stores/users'
import { apiError } from '../i18n/apiErrors'
import AppAutocomplete from './AppAutocomplete.vue'
import AppDialog from './AppDialog.vue'
import AppFormGroup from './AppFormGroup.vue'
import { t } from '../i18n'

const open = defineModel<boolean>({ required: true })

// Shares either an album or photos.
const props = defineProps<{ photoIds?: number[]; albumId?: number }>()
const emit = defineEmits<{ done: [] }>()

const users = useUsersStore()

const KEEP = 'keep'

// One album or photo shows its current state. Several photos may differ from
// each other, so the dialog only describes the changes to make.
const single = computed(() => props.albumId !== undefined || props.photoIds?.length === 1)
const title = computed(() => {
  if (props.albumId !== undefined) {
    return t('share.album')
  }
  return single.value ? t('share.photo') : t('share.photos', { n: props.photoIds?.length ?? 0 })
})

const loaded = ref(false)
const error = ref('')
const saving = ref(false)

const initialVisibility = ref<Visibility>('private')
const initialUsers = shallowRef<UserRef[]>([])
const everyone = ref(false)
const chosen = ref<number[]>([])

const visibilityChoice = ref<Visibility | typeof KEEP>(KEEP)
const toRemove = ref<number[]>([])

const visibilityChoices = computed(() => [
  { title: t('share.keep'), value: KEEP },
  { title: t('share.private'), value: 'private' },
  { title: t('share.all'), value: 'all' },
])

// Someone disabled after being given access is not among the recipients, but stays on the list.
const candidates = computed(() => {
  const known = new Set(users.recipients.map((u) => u.id))
  return [...users.recipients, ...initialUsers.value.filter((u) => !known.has(u.id))]
})

watch(open, async (isOpen) => {
  if (!isOpen) {
    return
  }
  loaded.value = false
  error.value = ''
  initialVisibility.value = 'private'
  initialUsers.value = []
  everyone.value = false
  chosen.value = []
  visibilityChoice.value = KEEP
  toRemove.value = []
  await users.load()
  if (single.value) {
    const { data, error: failure } =
      props.albumId !== undefined
        ? await api.GET('/albums/{id}/sharing', { params: { path: { id: props.albumId } } })
        : await api.GET('/photos/{id}/sharing', { params: { path: { id: props.photoIds![0]! } } })
    if (!data) {
      error.value = apiError(failure, t('share.loadFailed'))
      return
    }
    initialVisibility.value = data.visibility
    initialUsers.value = data.users
    everyone.value = data.visibility === 'all'
    chosen.value = data.users.map((u) => u.id)
  }
  loaded.value = true
})

function change() {
  if (!single.value) {
    return {
      ...(visibilityChoice.value === KEEP ? {} : { visibility: visibilityChoice.value }),
      add: chosen.value,
      remove: toRemove.value,
    }
  }
  const before = new Set(initialUsers.value.map((u) => u.id))
  const after = new Set(chosen.value)
  const visibility: Visibility = everyone.value ? 'all' : 'private'
  return {
    ...(visibility === initialVisibility.value ? {} : { visibility }),
    add: chosen.value.filter((id) => !before.has(id)),
    remove: [...before].filter((id) => !after.has(id)),
  }
}

async function save() {
  saving.value = true
  error.value = ''
  try {
    const body = change()
    if (props.albumId !== undefined) {
      const { error: failure } = await api.POST('/albums/{id}/sharing', { params: { path: { id: props.albumId } }, body })
      if (failure) {
        throw new Error(apiError(failure))
      }
    } else {
      await inBatches(props.photoIds ?? [], (batch) => api.POST('/photos/sharing', { body: { ...body, ids: batch } }))
    }
    open.value = false
    emit('done')
  } catch (failure) {
    error.value = apiError((failure as Error).message)
  } finally {
    saving.value = false
  }
}
</script>

<template>
  <AppDialog
    v-model="open"
    :title="title"
    :action="t('common.save')"
    :error="error"
    :loading="saving"
    :disabled="single && !loaded"
    :form="false"
    max-width="480"
    @submit="save"
  >
    <template v-if="single">
      <v-switch v-model="everyone" :label="t('share.everyone')" color="primary" density="compact" hide-details class="mb-4" />
    </template>
    <AppFormGroup v-else v-slot="field" :label="t('share.visibility')">
      <v-select v-bind="field" v-model="visibilityChoice" variant="outlined" :items="visibilityChoices" />
    </AppFormGroup>
    <AppFormGroup v-slot="field" :label="t('share.with')">
      <AppAutocomplete v-bind="field" v-model="chosen" :items="candidates" item-title="login" item-value="id" multiple />
    </AppFormGroup>
    <AppFormGroup v-if="!single" v-slot="field" :label="t('share.stop')">
      <AppAutocomplete v-bind="field" v-model="toRemove" :items="users.everyone" item-title="login" item-value="id" multiple />
    </AppFormGroup>
    <div class="text-medium-emphasis">
      {{ t('share.note') }}
    </div>
  </AppDialog>
</template>
