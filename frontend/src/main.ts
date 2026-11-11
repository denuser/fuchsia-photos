import '@fontsource-variable/red-hat-display'
import '@fontsource-variable/red-hat-text'
import 'vuetify/styles'
import './styles/patternfly.css'

import { createPinia } from 'pinia'
import { createApp } from 'vue'

import { setUnauthorizedHandler } from './api/client'
import App from './App.vue'
import { i18n } from './i18n'
import { vuetify } from './plugins/vuetify'
import { router } from './router'
import { useAuthStore } from './stores/auth'
import { usePhotosStore } from './stores/photos'

const app = createApp(App).use(createPinia()).use(i18n).use(router).use(vuetify)

setUnauthorizedHandler(() => {
  useAuthStore().clear()
  usePhotosStore().reset()
  void router.push('/login')
})

app.mount('#app')
