<script setup lang="ts">
// A read-only field with a button that copies its value. Without a secure context, as on a NAS
// served over plain HTTP, the browser has no clipboard API, so the field falls back to the older
// copy command on its own selected text.
import { ref } from 'vue'

import { useNoticesStore } from '../stores/notices'
import { t } from '../i18n'

const props = defineProps<{ value: string; id?: string; label?: string }>()

const notices = useNoticesStore()
const field = ref<{ $el: HTMLElement }>()

async function copy() {
  const input = field.value?.$el.querySelector('input')
  try {
    if (navigator.clipboard) {
      await navigator.clipboard.writeText(props.value)
    } else {
      input?.select()
      if (!document.execCommand('copy')) {
        throw new Error('copy refused')
      }
    }
    notices.notify(t('copy.copied'))
  } catch {
    input?.select()
    notices.notify(t('copy.failed'), 'warning')
  }
}
</script>

<template>
  <v-text-field
    :id="id"
    ref="field"
    :model-value="value"
    readonly
    class="copy-field"
    @focus="($event.target as HTMLInputElement).select()"
  >
    <template #append-inner>
      <v-btn icon="fa-copy" variant="text" size="small" density="comfortable" :aria-label="label ? t('copy.named', { label }) : t('copy.copy')" @click="copy" />
    </template>
  </v-text-field>
</template>

<style scoped>
.copy-field :deep(input) {
  font-family: ui-monospace, SFMono-Regular, Menlo, Consolas, 'Liberation Mono', monospace;
}
</style>
