<script setup lang="ts">
import { ref, watch } from 'vue'

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
  <v-dialog v-model="open" max-width="420">
    <v-card :title="title">
      <v-form @submit.prevent="submit">
        <v-card-text>
          <v-alert v-if="error" type="error" density="compact" class="mb-4" :text="error" />
          <v-text-field v-model="name" :label="label" autofocus />
        </v-card-text>
        <v-card-actions>
          <v-spacer />
          <v-btn text="Cancel" @click="open = false" />
          <v-btn type="submit" color="primary" :text="action" :loading="saving" />
        </v-card-actions>
      </v-form>
    </v-card>
  </v-dialog>
</template>
