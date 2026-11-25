<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'

import logo from '../assets/logo.png'
import { api, type SignInOptions } from '../api/client'
import { useAuthStore } from '../stores/auth'
import AppFormGroup from '../components/AppFormGroup.vue'
import { t } from '../i18n'

const auth = useAuthStore()
const router = useRouter()
const route = useRoute()

// A sign-in through a provider that failed comes back here with the reason in the address.
const reasons = ['expired', 'denied', 'unavailable', 'no_account', 'not_allowed', 'disabled', 'taken', 'failed'] as const
function providerError(): string {
  const code = String(route.query.error ?? '').replace(/^oidc_/, '')
  return (reasons as readonly string[]).includes(code) ? t(`login.oidcErrors.${code as (typeof reasons)[number]}`) : ''
}

const options = ref<SignInOptions>({ providers: [], passwordAdminsOnly: false })
const hasProviders = computed(() => options.value.providers.length > 0)

onMounted(async () => {
  const { data } = await api.GET('/auth/options')
  if (data) {
    options.value = data
  }
})

const login = ref('')
const password = ref('')
const error = ref(providerError())
const submitting = ref(false)

// With two-factor sign-in on, the password step returns a token and the form asks for a code.
const token = ref('')
const code = ref('')

async function submit() {
  submitting.value = true
  const result = await auth.login(login.value, password.value)
  error.value = result.error ?? ''
  submitting.value = false
  if (result.token) {
    token.value = result.token
    password.value = ''
  } else if (!error.value) {
    password.value = ''
    await router.push('/')
  }
}

async function submitCode() {
  submitting.value = true
  error.value = (await auth.loginTotp(token.value, code.value)) ?? ''
  submitting.value = false
  if (!error.value) {
    await router.push('/')
  }
}

function back() {
  token.value = ''
  code.value = ''
  error.value = ''
}
</script>

<template>
  <v-container class="fill-height justify-center loginForm">
    <v-card class="loginForm__card" title="Magellanica Photos" :subtitle="t('login.subtitle')">
      <template #prepend>
        <v-avatar :image="logo" size="48" />
      </template>
      <v-form v-if="token" @submit.prevent="submitCode">
        <v-card-text>
          <v-alert v-if="error" type="error" density="compact" class="mb-4" :title="error" />
          <div class="mb-4">{{ t('login.codeHint') }}</div>
          <AppFormGroup v-slot="field" :label="t('login.code')" required>
            <v-text-field v-bind="field" v-model="code" inputmode="text" autocomplete="one-time-code" autofocus data-testid="login-code" />
          </AppFormGroup>
          <v-btn type="submit" color="primary" block :text="t('login.submit')" :loading="submitting" :disabled="!code" />
          <v-btn variant="text" block class="mt-2" :text="t('login.back')" @click="back" />
        </v-card-text>
      </v-form>
      <v-form v-else @submit.prevent="submit">
        <v-card-text>
          <v-alert v-if="error" type="error" density="compact" class="mb-4" :title="error" />
          <AppFormGroup v-slot="field" :label="t('login.login')" required>
            <v-text-field v-bind="field" v-model="login" autocomplete="username" autofocus />
          </AppFormGroup>
          <AppFormGroup v-slot="field" :label="t('login.password')" required>
            <v-text-field v-bind="field" v-model="password" type="password" autocomplete="current-password" />
          </AppFormGroup>
          <v-btn type="submit" color="primary" block :text="t('login.submit')" :loading="submitting"
            :disabled="!login || !password" />
          <div v-if="options.passwordAdminsOnly" class="text-medium-emphasis mt-3">{{ t('login.passwordAdminsOnly') }}</div>
          <template v-if="hasProviders">
            <div class="loginForm__or text-medium-emphasis my-4">{{ t('login.or') }}</div>
            <!-- A plain link: the whole page goes to the provider and comes back. -->
            <v-btn
              v-for="provider in options.providers"
              :key="provider.id"
              :href="`/api/v1/auth/oidc/${provider.id}/login`"
              variant="outlined"
              block
              class="mb-2"
              data-testid="provider-login"
              :text="t('login.continueWith', { name: provider.name })"
            />
          </template>
        </v-card-text>
      </v-form>
    </v-card>
  </v-container>
</template>

<style scoped>
.loginForm {
  display: flex;
  flex-direction: column;
  align-items: center;
}

.loginForm__or {
  display: flex;
  align-items: center;
  gap: 12px;
}

.loginForm__or::before,
.loginForm__or::after {
  content: '';
  flex: 1;
  border-top: 1px solid rgb(var(--v-theme-on-surface), 0.2);
}

.loginForm__card {
  max-width: 380px;
  width: calc(100% - 20px);
}
</style>