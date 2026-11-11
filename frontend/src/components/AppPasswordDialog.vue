<script setup lang="ts">
// Creates an app password in two steps: a name for the device, then the password, which the server
// returns only once.
import { ref, watch } from 'vue'

import { api, type AppPassword } from '../api/client'
import { apiError } from '../i18n/apiErrors'
import AppDialog from './AppDialog.vue'
import AppFormGroup from './AppFormGroup.vue'
import CopyField from './CopyField.vue'
import { t } from '../i18n'

const open = defineModel<boolean>({ required: true })
const emit = defineEmits<{ created: [AppPassword] }>()

const name = ref('')
const password = ref('')
const error = ref('')
const saving = ref(false)

// Cleared on opening rather than closing, so the password stays while the dialog fades out.
watch(open, (value) => {
  if (value) {
    name.value = ''
    password.value = ''
    error.value = ''
  }
})

async function create() {
  saving.value = true
  error.value = ''
  const { data, error: failure } = await api.POST('/me/app-passwords', { body: { name: name.value } })
  saving.value = false
  if (!data) {
    error.value = apiError(failure, t('appPassword.createFailed'))
    return
  }
  password.value = data.password
  emit('created', data)
}
</script>

<template>
  <AppDialog
    v-model="open"
    :title="password ? t('appPassword.created') : t('appPassword.new')"
    :action="password ? undefined : t('common.create')"
    :cancel="password ? t('common.done') : t('common.cancel')"
    :error="error"
    :loading="saving"
    max-width="480"
    @submit="create"
  >
    <template v-if="!password">
      <AppFormGroup v-slot="field" :label="t('appPassword.device')" required>
        <v-text-field v-bind="field" v-model="name" autofocus :hint="t('appPassword.deviceHint')" />
      </AppFormGroup>
    </template>
    <template v-else>
      <v-alert type="info" density="compact" class="mb-4" :title="t('appPassword.copyNow')" />
      <AppFormGroup v-slot="field" :label="t('appPassword.password')">
        <CopyField v-bind="field" :value="password" :label="t('appPassword.passwordLower')" />
      </AppFormGroup>
    </template>
  </AppDialog>
</template>
