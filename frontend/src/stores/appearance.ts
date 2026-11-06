import { defineStore } from 'pinia'
import { computed, ref, watch } from 'vue'

// The color scheme and contrast mode chosen in the app bar. The choice is kept
// in this browser only: each device can follow its own system settings.

export type Scheme = 'system' | 'light' | 'dark'
export type Contrast = 'system' | 'default' | 'high' | 'glass'

const schemes: Scheme[] = ['system', 'light', 'dark']
const contrasts: Contrast[] = ['system', 'default', 'high', 'glass']

function stored<T extends string>(key: string, allowed: T[]): T {
  const value = localStorage.getItem(key) as T | null
  return value && allowed.includes(value) ? value : allowed[0]!
}

const prefersDark = window.matchMedia('(prefers-color-scheme: dark)')
const prefersMoreContrast = window.matchMedia('(prefers-contrast: more)')

function themeName(scheme: Scheme, contrast: Contrast, systemDark: boolean, systemMore: boolean) {
  const dark = scheme === 'dark' || (scheme === 'system' && systemDark)
  const high = contrast === 'high' || (contrast === 'system' && systemMore)
  return `${dark ? 'dark' : 'light'}${high ? '-hc' : ''}`
}

function applyGlass(contrast: Contrast) {
  document.documentElement.classList.toggle('glass', contrast === 'glass')
}

// The theme to start with, read before the app mounts so the page does not
// flash in the wrong colors.
export function initialThemeName() {
  const contrast = stored('contrast', contrasts)
  applyGlass(contrast)
  return themeName(stored('scheme', schemes), contrast, prefersDark.matches, prefersMoreContrast.matches)
}

export const useAppearanceStore = defineStore('appearance', () => {
  const scheme = ref(stored('scheme', schemes))
  const contrast = ref(stored('contrast', contrasts))
  const systemDark = ref(prefersDark.matches)
  const systemMore = ref(prefersMoreContrast.matches)

  prefersDark.addEventListener('change', (event) => (systemDark.value = event.matches))
  prefersMoreContrast.addEventListener('change', (event) => (systemMore.value = event.matches))

  watch(scheme, (value) => localStorage.setItem('scheme', value))
  watch(contrast, (value) => {
    localStorage.setItem('contrast', value)
    applyGlass(value)
  })

  const theme = computed(() => themeName(scheme.value, contrast.value, systemDark.value, systemMore.value))

  return { scheme, contrast, theme }
})
