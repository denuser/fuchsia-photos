<script setup lang="ts">
import { ref, watch } from 'vue'
import AppDialog from './AppDialog.vue'
import AppFormGroup from './AppFormGroup.vue'

const open = defineModel<boolean>({ required: true })

// save returns an error message, or null on success.
const props = defineProps<{ title: string; label: string; action: string; initial?: string; save: (name: string) => Promise<string | null> }>()

const name = ref('')
const error = ref('')
const saving = ref(false)

watch(open, () => {
  name.value = props.initial ?? ''
  error.value = ''
})

async function submit() {
  saving.value = true
  const failure = await props.save(name.value)
  saving.value = false
  if (failure) {
    error.value = failure
    return
  }
  open.value = false
}
</script>

<template>
  <AppDialog v-model="open" :title="title" :action="action" :error="error" :loading="saving" @submit="submit">
    <AppFormGroup v-slot="field" :label="label" required>
      <v-text-field v-bind="field" v-model="name" autofocus />
    </AppFormGroup>
  </AppDialog>
</template>
