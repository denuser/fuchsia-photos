<script setup lang="ts">
import { computed } from 'vue'
import { useDisplay } from 'vuetify'

import { glassColors, useAppearanceStore, type Contrast, type Scheme } from '../stores/appearance'
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
      <template v-if="appearance.contrast === 'glass'">
        <v-divider />
        <section>
          <h2 id="glass-color-title">{{ t('appearance.glassColor') }}</h2>
          <div class="swatches" role="radiogroup" aria-labelledby="glass-color-title">
            <button
              v-for="item in glassColors"
              :key="item"
              type="button"
              role="radio"
              class="swatch"
              :class="`swatch--${item}`"
              :aria-checked="appearance.glassColor === item"
              :aria-label="t(`appearance.glassColors.${item}`)"
              :title="t(`appearance.glassColors.${item}`)"
              @click="appearance.glassColor = item"
            />
          </div>
        </section>
      </template>
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

.swatches {
  display: flex;
  flex-wrap: wrap;
  gap: 12px;
}

.swatch {
  width: 36px;
  height: 36px;
  border: 2px solid transparent;
  border-radius: 50%;
  outline: 2px solid transparent;
  outline-offset: 2px;
  cursor: pointer;
}

.swatch[aria-checked='true'] {
  outline-color: rgb(var(--v-theme-primary));
}

.swatch--fuchsia {
  background: linear-gradient(135deg, #c026d3, #5e40be);
}

.swatch--orchid {
  background: linear-gradient(135deg, #ba68c8, #6750a4);
}

.swatch--raspberry {
  background: linear-gradient(135deg, #db2777, #881337);
}

.swatch--twilight {
  background: linear-gradient(135deg, #c026d3, #312e81);
}

.swatch--rose {
  background: linear-gradient(135deg, #f472b6, #fda4af);
}

.swatch--plum {
  background: linear-gradient(135deg, #86198f, #3b0764);
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
