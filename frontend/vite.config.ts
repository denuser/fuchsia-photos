import vue from '@vitejs/plugin-vue'
import { defineConfig } from 'vite'
import vuetify from 'vite-plugin-vuetify'

export default defineConfig({
  plugins: [vue(), vuetify()],
  server: {
    // Listen on all interfaces so phones on the local network can open the dev server.
    host: true,
    proxy: {
      '/api': 'http://localhost:8080',
    },
  },
})
