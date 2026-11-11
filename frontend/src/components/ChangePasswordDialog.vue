<script setup lang="ts">
import { ref, watch } from 'vue'

import { api } from '../api/client'
import { apiError } from '../i18n/apiErrors'
import AppDialog from './AppDialog.vue'
import AppFormGroup from './AppFormGroup.vue'
import { t } from '../i18n'

const open = defineModel<boolean>({ required: true })

const currentPassword = ref('')
const newPassword = ref('')
const error = ref('')
const saving = ref(false)

watch(open, () => {
  currentPassword.value = ''
  newPassword.value = ''
  error.value = ''
})

async function save() {
  saving.value = true
  const { error: failure } = await api.POST('/me/password', {
    body: { currentPassword: currentPassword.value, newPassword: newPassword.value },
  })
  saving.value = false
  if (failure) {
    error.value = apiError(failure)
    return
  }
  open.value = false
}
</script>

<template>
  <AppDialog v-model="open" :title="t('changePassword.title')" :action="t('changePassword.title')" :error="error" :loading="saving" @submit="save">
    <AppFormGroup v-slot="field" :label="t('changePassword.current')" required>
      <v-text-field v-bind="field" v-model="currentPassword" type="password" autocomplete="current-password" />
    </AppFormGroup>
    <AppFormGroup v-slot="field" :label="t('changePassword.new')" required>
      <v-text-field
        v-bind="field"
        v-model="newPassword"
        type="password"
        autocomplete="new-password"
        :hint="t('changePassword.hint')"
      />
    </AppFormGroup>
  </AppDialog>
</template>
