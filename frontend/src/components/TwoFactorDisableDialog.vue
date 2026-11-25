<script setup lang="ts">
import { ref, watch } from 'vue'

import { api } from '../api/client'
import { apiError } from '../i18n/apiErrors'
import AppDialog from './AppDialog.vue'
import AppFormGroup from './AppFormGroup.vue'
import { t } from '../i18n'

const open = defineModel<boolean>({ required: true })
const emit = defineEmits<{ disabled: [] }>()

const password = ref('')
const code = ref('')
const error = ref('')
const saving = ref(false)

watch(open, () => {
  password.value = ''
  code.value = ''
  error.value = ''
})

async function save() {
  saving.value = true
  const { error: failure } = await api.POST('/me/2fa/disable', { body: { password: password.value, code: code.value } })
  saving.value = false
  if (failure) {
    error.value = apiError(failure)
    return
  }
  open.value = false
  emit('disabled')
}
</script>

<template>
  <AppDialog
    v-model="open"
    :title="t('twoFactor.disableTitle')"
    :action="t('twoFactor.disable')"
    :error="error"
    :loading="saving"
    :disabled="!password || !code"
    danger
    @submit="save"
  >
    <p class="mb-4">{{ t('twoFactor.disableHint') }}</p>
    <AppFormGroup v-slot="field" :label="t('login.password')" required>
      <v-text-field v-bind="field" v-model="password" type="password" autocomplete="current-password" autofocus />
    </AppFormGroup>
    <AppFormGroup v-slot="field" :label="t('login.code')" required>
      <v-text-field v-bind="field" v-model="code" autocomplete="one-time-code" />
    </AppFormGroup>
  </AppDialog>
</template>
