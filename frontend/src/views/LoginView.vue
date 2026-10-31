<script setup lang="ts">
import { ref } from 'vue'
import { useRouter } from 'vue-router'

import { useAuthStore } from '../stores/auth'

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
    <v-card width="380" title="Fuchsia Photos" subtitle="Log in to your library">
      <v-form @submit.prevent="submit">
        <v-card-text>
          <v-alert v-if="error" type="error" density="compact" class="mb-4" :text="error" />
          <v-text-field v-model="login" label="Login" autocomplete="username" autofocus />
          <v-text-field v-model="password" label="Password" type="password" autocomplete="current-password" />
          <v-btn type="submit" color="primary" block text="Log in" :loading="submitting" :disabled="!login || !password" />
        </v-card-text>
      </v-form>
    </v-card>
  </v-container>
</template>
