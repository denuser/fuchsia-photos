<script setup lang="ts">
// Turning two-factor sign-in on: the secret as a picture and as text, a code to confirm it, then the
// recovery codes, which the server shows only once.
import { ref, watch } from 'vue'

import { api } from '../api/client'
import { apiError } from '../i18n/apiErrors'
import AppDialog from './AppDialog.vue'
import AppFormGroup from './AppFormGroup.vue'
import CopyField from './CopyField.vue'
import { t } from '../i18n'

const open = defineModel<boolean>({ required: true })
const emit = defineEmits<{ enabled: [] }>()

const secret = ref<{ secret: string; qr: string } | null>(null)
const code = ref('')
const recoveryCodes = ref<string[]>([])
const error = ref('')
const saving = ref(false)

watch(open, async (isOpen) => {
  code.value = ''
  error.value = ''
  recoveryCodes.value = []
  secret.value = null
  if (!isOpen) {
    return
  }
  const { data, error: failure } = await api.POST('/me/2fa/setup')
  if (failure) {
    error.value = apiError(failure)
    return
  }
  secret.value = data
})

async function confirm() {
  saving.value = true
  const { data, error: failure } = await api.POST('/me/2fa/enable', { body: { code: code.value } })
  saving.value = false
  if (failure) {
    error.value = apiError(failure)
    return
  }
  error.value = ''
  recoveryCodes.value = data.recoveryCodes
  emit('enabled')
}
</script>

<template>
    <AppDialog
    v-if="recoveryCodes.length === 0"
    v-model="open"
    :title="t('twoFactor.setupTitle')"
    :action="t('twoFactor.confirm')"
    :error="error"
    :loading="saving"
    :disabled="!secret || code.length < 6"
    @submit="confirm"
  >
    <template v-if="secret">
      <p class="mb-4">{{ t('twoFactor.scan') }}</p>
      <div class="d-flex justify-center mb-4">
        <img :src="secret.qr" width="224" height="224" alt="" class="twoFactorQr" />
      </div>
      <AppFormGroup v-slot="field" :label="t('twoFactor.key')">
        <CopyField :id="field.id" :value="secret.secret" :label="t('twoFactor.key')" />
      </AppFormGroup>
      <AppFormGroup v-slot="field" :label="t('twoFactor.code')" required>
        <v-text-field v-bind="field" v-model="code" inputmode="numeric" autocomplete="one-time-code" autofocus />
      </AppFormGroup>
    </template>
  </AppDialog>
  <!-- The recovery codes are shown once, so only Done closes this step. -->
  <AppDialog v-else v-model="open" :title="t('twoFactor.recoveryTitle')" :action="t('twoFactor.done')" persistent @submit="open = false">
    <p class="mb-4">{{ t('twoFactor.recoveryHint') }}</p>
    <ul class="twoFactorCodes" data-testid="recovery-codes">
      <li v-for="c in recoveryCodes" :key="c">{{ c }}</li>
    </ul>
  </AppDialog>
</template>

<style scoped>
.twoFactorQr {
  /* The picture is black on white whatever the theme: dark mode would make it unreadable. */
  background: #fff;
  border-radius: 4px;
}

.twoFactorCodes {
  list-style: none;
  padding: 0;
  display: grid;
  grid-template-columns: 1fr 1fr;
  gap: 4px 16px;
  font-family: ui-monospace, SFMono-Regular, Menlo, Consolas, 'Liberation Mono', monospace;
  user-select: all;
}
</style>
