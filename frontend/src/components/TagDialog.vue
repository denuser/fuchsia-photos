<script setup lang="ts">
import { computed, ref, shallowRef, watch } from 'vue'

import { api, type TagRef } from '../api/client'
import { inBatches } from '../stores/photos'
import { useTagsStore } from '../stores/tags'
import { apiError } from '../i18n/apiErrors'
import AppAutocomplete from './AppAutocomplete.vue'
import AppDialog from './AppDialog.vue'
import AppFormGroup from './AppFormGroup.vue'
import { t } from '../i18n'

const open = defineModel<boolean>({ required: true })

const props = defineProps<{ photoIds: number[] }>()
const emit = defineEmits<{ done: [] }>()

const tags = useTagsStore()

const current = shallowRef<TagRef[]>([])
const toAdd = ref<string[]>([])
const toRemove = ref<number[]>([])
const error = ref('')
const saving = ref(false)

const single = computed(() => props.photoIds.length === 1)
const title = computed(() => (single.value ? t('tagDialog.single') : t('tagDialog.several', { n: props.photoIds.length })))
// One photo offers its own tags for removal. For several, any tag may be on some of them.
const removable = computed<TagRef[]>(() => (single.value ? current.value : tags.tags))

watch(open, async (isOpen) => {
  if (!isOpen) {
    return
  }
  toAdd.value = []
  toRemove.value = []
  error.value = ''
  current.value = []
  await tags.load()
  if (single.value) {
    const { data } = await api.GET('/photos/{id}/tags', { params: { path: { id: props.photoIds[0]! } } })
    current.value = data ?? []
  }
})

async function tagIds(names: string[]) {
  const ids: number[] = []
  for (const name of names) {
    const { data, error: failure } = await api.POST('/tags', { body: { name } })
    if (failure) {
      throw new Error(apiError(failure))
    }
    ids.push(data.id)
  }
  return ids
}

async function save() {
  saving.value = true
  error.value = ''
  try {
    const add = await tagIds(toAdd.value)
    const remove = toRemove.value
    if (add.length > 0 || remove.length > 0) {
      await inBatches(props.photoIds, (batch) => api.POST('/photos/tags', { body: { ids: batch, add, remove } }))
      void tags.load()
      emit('done')
    }
    open.value = false
  } catch (failure) {
    error.value = apiError((failure as Error).message)
  } finally {
    saving.value = false
  }
}
</script>

<template>
  <!-- Not a form: Enter belongs to the fields, where it confirms a tag. -->
  <AppDialog v-model="open" :title="title" :action="t('common.save')" :error="error" :loading="saving" :form="false" max-width="480" @submit="save">
    <div v-if="single && current.length > 0" class="d-flex flex-wrap ga-1 mb-4">
      <v-chip v-for="tag in current" :key="tag.id" size="small">{{ tag.name }}</v-chip>
    </div>
    <AppFormGroup v-slot="field" :label="t('tagDialog.add')">
      <v-combobox
        v-bind="field"
        v-model="toAdd"
        :items="tags.tags.map((tag) => tag.name)"
        :hint="t('tagDialog.addHint')"
        multiple
        chips
        closable-chips
        variant="outlined"
      />
    </AppFormGroup>
    <AppFormGroup v-slot="field" :label="t('tagDialog.remove')">
      <AppAutocomplete
        v-bind="field"
        v-model="toRemove"
        :items="removable"
        item-title="name"
        item-value="id"
        multiple
        :disabled="removable.length === 0"
      />
    </AppFormGroup>
  </AppDialog>
</template>
