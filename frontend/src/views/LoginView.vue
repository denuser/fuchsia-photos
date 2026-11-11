<script setup lang="ts">
import { ref } from 'vue'
import { useRouter } from 'vue-router'

import logo from '../assets/logo.png'
import { useAuthStore } from '../stores/auth'
import AppFormGroup from '../components/AppFormGroup.vue'
import { t } from '../i18n'

const auth = useAuthStore()
const router = useRouter()

const login = ref('')
const password = ref('')
const error = ref('')
const submitting = ref(false)

async function submit() {
  submitting.value = true
  error.value = (await auth.login(login.value, password.value)) ?? ''
  submitting.value = false
  if (!error.value) {
    password.value = ''
    await router.push('/')
  }
}
</script>

<template>
  <v-container class="fill-height justify-center">
    <v-card width="380" title="Fuchsia Photos" :subtitle="t('login.subtitle')">
      <template #prepend>
        <v-avatar :image="logo" size="48" />
      </template>
      <v-form @submit.prevent="submit">
        <v-card-text>
          <v-alert v-if="error" type="error" density="compact" class="mb-4" :title="error" />
          <AppFormGroup v-slot="field" :label="t('login.login')" required>
            <v-text-field v-bind="field" v-model="login" autocomplete="username" autofocus />
          </AppFormGroup>
          <AppFormGroup v-slot="field" :label="t('login.password')" required>
            <v-text-field v-bind="field" v-model="password" type="password" autocomplete="current-password" />
          </AppFormGroup>
          <v-btn type="submit" color="primary" block :text="t('login.submit')" :loading="submitting" :disabled="!login || !password" />
        </v-card-text>
      </v-form>
    </v-card>
  </v-container>
</template>
