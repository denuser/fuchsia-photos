<script setup lang="ts">
// The first password of an account made by a provider. The server takes it only within minutes of the
// user signing in again at the provider, so there is no current password to ask for.
import { ref, watch } from 'vue'

import { api } from '../api/client'
import { apiError } from '../i18n/apiErrors'
import AppDialog from './AppDialog.vue'
import AppFormGroup from './AppFormGroup.vue'
import { t } from '../i18n'

const open = defineModel<boolean>({ required: true })
const emit = defineEmits<{ saved: [] }>()

const password = ref('')
const error = ref('')
const saving = ref(false)

watch(open, () => {
  password.value = ''
  error.value = ''
})

async function save() {
  saving.value = true
  const { error: failure } = await api.POST('/me/password', { body: { newPassword: password.value } })
  saving.value = false
  if (failure) {
    error.value = apiError(failure)
    return
  }
  open.value = false
  emit('saved')
}
</script>

<template>
  <AppDialog v-model="open" :title="t('profile.setPassword')" :action="t('profile.setPassword')" :error="error" :loading="saving" :disabled="!password" @submit="save">
    <AppFormGroup v-slot="field" :label="t('changePassword.new')" required>
      <v-text-field v-bind="field" v-model="password" type="password" autocomplete="new-password" autofocus :hint="t('changePassword.hint')" />
    </AppFormGroup>
  </AppDialog>
</template>
