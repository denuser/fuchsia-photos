<script setup lang="ts">
// A dialog laid out as a PatternFly modal: the title, an error alert, the content from the default
// slot, and a footer with the filled primary button on the left and a link-style Cancel after it.
// A destructive action makes the primary button red. The content sits in a form, so Enter in a
// field submits it, unless form is false: there Enter belongs to the fields, as in tag pickers.
// Without an action the footer has only the cancel button.
import { computed } from 'vue'
import { VForm } from 'vuetify/components'
import { t } from '../i18n'

const open = defineModel<boolean>({ required: true })

const props = withDefaults(
  defineProps<{
    title: string
    action?: string
    cancel?: string
    error?: string
    danger?: boolean
    loading?: boolean
    disabled?: boolean
    form?: boolean
    maxWidth?: number | string
  }>(),
  { action: undefined, cancel: undefined, error: '', form: true, maxWidth: 420 },
)
const emit = defineEmits<{ submit: [] }>()

const asForm = computed(() => props.form && props.action !== undefined)

function onClick() {
  if (!asForm.value) {
    emit('submit')
  }
}
</script>

<template>
  <v-dialog v-model="open" :max-width="maxWidth">
    <v-card :title="title">
      <component :is="asForm ? VForm : 'div'" @submit.prevent="emit('submit')">
        <v-card-text>
          <v-alert v-if="error" type="error" density="compact" class="mb-4" :title="error" />
          <slot />
        </v-card-text>
        <v-card-actions>
          <v-btn
            v-if="action !== undefined"
            :type="asForm ? 'submit' : 'button'"
            variant="flat"
            :color="danger ? 'error' : 'primary'"
            :text="action"
            :loading="loading"
            :disabled="disabled"
            @click="onClick"
          />
          <v-btn variant="text" color="primary" :text="cancel ?? t('common.cancel')" @click="open = false" />
        </v-card-actions>
      </component>
    </v-card>
  </v-dialog>
</template>
