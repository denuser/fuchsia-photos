import { readFileSync } from 'node:fs'
import { join } from 'node:path'

import vue from '@vitejs/plugin-vue'
import { defineConfig } from 'vite'
import vuetify from 'vite-plugin-vuetify'

// make dev-frontend passes a folder with a self-signed certificate, as sync apps such as FolderSync
// connect only over HTTPS.
const tls = process.env.DEV_TLS_DIR

export default defineConfig({
  plugins: [vue(), vuetify()],
  // vue-i18n feature flags: the Composition API only, and no devtools in production.
  define: {
    __VUE_I18N_FULL_INSTALL__: true,
    __VUE_I18N_LEGACY_API__: false,
    __INTLIFY_PROD_DEVTOOLS__: false,
  },
  server: {
    // Listen on all interfaces so phones on the local network can open the dev server.
    host: true,
    https: tls ? { cert: readFileSync(join(tls, 'cert.pem')), key: readFileSync(join(tls, 'key.pem')) } : undefined,
    proxy: {
      '/api': 'http://localhost:8080',
      '/webdav': 'http://localhost:8080',
    },
  },
})
