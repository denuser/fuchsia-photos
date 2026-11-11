<script setup lang="ts">
import { computed } from 'vue'
import { useDisplay } from 'vuetify'

import { useAppearanceStore, type Contrast, type Scheme } from '../stores/appearance'
import { t } from '../i18n'

const appearance = useAppearanceStore()
// On a phone the button is an icon alone, to leave room in the app bar.
const { xs } = useDisplay()

const schemes: Scheme[] = ['system', 'light', 'dark']
const contrasts: Contrast[] = ['system', 'default', 'high', 'glass']

const icon = computed(() => {
  if (appearance.scheme === 'system') {
    return 'fa-circle-half-stroke'
  }
  return appearance.scheme === 'dark' ? 'fa-moon' : 'fa-sun'
})
</script>

<template>
  <v-menu :close-on-content-click="false" location="bottom end">
    <template #activator="{ props }">
      <v-btn v-if="xs" v-bind="props" :icon="icon" :aria-label="t('appearance.title')" />
      <v-btn v-else v-bind="props" variant="outlined" class="mr-2 appearance-button" :aria-label="t('appearance.title')">
        <v-icon :icon="icon" />
        <v-icon icon="fa-caret-down" end />
      </v-btn>
    </template>
    <v-card class="appearance-menu">
      <section>
        <h2 id="scheme-title">{{ t('appearance.scheme') }}</h2>
        <v-btn-toggle v-model="appearance.scheme" mandatory divided variant="outlined" aria-labelledby="scheme-title">
          <v-btn v-for="item in schemes" :key="item" :value="item">{{ t(`appearance.schemes.${item}`) }}</v-btn>
        </v-btn-toggle>
      </section>
      <v-divider />
      <section>
        <h2 id="contrast-title">{{ t('appearance.contrast') }}</h2>
        <v-btn-toggle v-model="appearance.contrast" mandatory divided variant="outlined" aria-labelledby="contrast-title">
          <v-btn v-for="item in contrasts" :key="item" :value="item">{{ t(`appearance.contrasts.${item}`) }}</v-btn>
        </v-btn-toggle>
      </section>
    </v-card>
  </v-menu>
</template>

<style scoped>
.appearance-button {
  min-width: 0;
  padding-inline: 12px;
}

.appearance-menu section {
  padding: 16px 24px;
}

.appearance-menu h2 {
  margin-bottom: 12px;
  font-size: 1rem;
  font-weight: 600;
}

.appearance-menu .v-btn-toggle {
  max-width: 100%;
}

@media (max-width: 599.98px) {
  .appearance-menu section {
    padding: 12px 16px;
  }

  .appearance-menu .v-btn {
    padding-inline: 10px;
  }
}
</style>
