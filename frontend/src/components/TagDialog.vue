<script setup lang="ts">
import { computed, ref, shallowRef, watch } from 'vue'

import { api, type TagRef } from '../api/client'
import { inBatches } from '../stores/photos'
import { useTagsStore } from '../stores/tags'
import AppAutocomplete from './AppAutocomplete.vue'

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
const title = computed(() => (single.value ? 'Tags of this photo' : `Tags of ${props.photoIds.length} photos`))
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
      throw new Error(failure.error)
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
    error.value = (failure as Error).message
  } finally {
    saving.value = false
  }
}
</script>

<template>
  <v-dialog v-model="open" max-width="480">
    <v-card :title="title">
      <!-- Not a form: Enter belongs to the fields, where it confirms a tag. -->
      <div>
        <v-card-text>
          <v-alert v-if="error" type="error" density="compact" class="mb-4" :text="error" />
          <div v-if="single && current.length > 0" class="d-flex flex-wrap ga-1 mb-4">
            <v-chip v-for="tag in current" :key="tag.id" size="small">{{ tag.name }}</v-chip>
          </div>
          <v-combobox
            v-model="toAdd"
            :items="tags.tags.map((t) => t.name)"
            label="Add tags"
            hint="Choose a tag or type a new one, then press Enter."
            persistent-hint
            multiple
            chips
            closable-chips
            variant="outlined"
            class="mb-2"
          />
          <AppAutocomplete
            v-model="toRemove"
            :items="removable"
            item-title="name"
            item-value="id"
            label="Remove tags"
            multiple
            :disabled="removable.length === 0"
          />
        </v-card-text>
        <v-card-actions>
          <v-spacer />
          <v-btn text="Cancel" @click="open = false" />
          <v-btn color="primary" text="Save" :loading="saving" @click="save" />
        </v-card-actions>
      </div>
    </v-card>
  </v-dialog>
</template>
