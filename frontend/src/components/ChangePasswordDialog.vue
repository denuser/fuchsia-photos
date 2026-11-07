<script setup lang="ts">
import { ref, watch } from 'vue'

import { api } from '../api/client'
import AppFormGroup from './AppFormGroup.vue'

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
    error.value = failure.error
    return
  }
  open.value = false
}
</script>

<template>
  <v-dialog v-model="open" max-width="420">
    <v-card title="Change password">
      <v-form @submit.prevent="save">
        <v-card-text>
          <v-alert v-if="error" type="error" density="compact" class="mb-4" :text="error" />
          <AppFormGroup v-slot="field" label="Current password" required>
            <v-text-field v-bind="field" v-model="currentPassword" type="password" autocomplete="current-password" />
          </AppFormGroup>
          <AppFormGroup v-slot="field" label="New password" required>
            <v-text-field
              v-bind="field"
              v-model="newPassword"
              type="password"
              autocomplete="new-password"
              hint="Your other devices will be logged out."
            />
          </AppFormGroup>
        </v-card-text>
        <v-card-actions>
          <v-spacer />
          <v-btn text="Cancel" @click="open = false" />
          <v-btn type="submit" color="primary" text="Change password" :loading="saving" />
        </v-card-actions>
      </v-form>
    </v-card>
  </v-dialog>
</template>
