import { createVuetify, type ThemeDefinition } from 'vuetify'
import { createVueI18nAdapter } from 'vuetify/locale/adapters/vue-i18n'
import { useI18n } from 'vue-i18n'

import { i18n, intlLocales } from '../i18n'
import { initialThemeName } from '../stores/appearance'
import { aliases, fontAwesome } from './icons'

// The colors follow the PatternFly 6 design tokens, with fuchsia in place of
// the PatternFly blue. Background is the page chrome around the main panel,
// surface is the panel itself, and surface-bright is for menus and dialogs.
// The "-hc" themes are the high contrast variants. Glass is not a theme of
// its own: it makes the surfaces of the light and dark themes translucent
// (see styles/patternfly.css).

const light: ThemeDefinition = {
  dark: false,
  colors: {
    background: '#f2f2f2',
    surface: '#ffffff',
    'surface-bright': '#ffffff',
    'surface-light': '#f2f2f2',
    'surface-variant': '#292929',
    'on-surface-variant': '#ffffff',
    primary: '#c026d3',
    'on-primary': '#ffffff',
    secondary: '#4d4d4d',
    error: '#b1380b',
    success: '#3d7317',
    warning: '#ffcc17',
    'on-warning': '#151515',
    info: '#5e40be',
    'on-background': '#151515',
    'on-surface': '#151515',
    'border-control': '#8c8c8c',
  },
  variables: {
    'border-color': '#e0e0e0',
    'border-opacity': 1,
    'high-emphasis-opacity': 1,
    'medium-emphasis-opacity': 0.78,
    // Keyboard focus is a ring (styles/patternfly.css), not a tinted overlay.
    'focus-opacity': 0,
  },
}

const dark: ThemeDefinition = {
  dark: true,
  colors: {
    background: '#151515',
    surface: '#292929',
    'surface-bright': '#383838',
    'surface-light': '#383838',
    'surface-variant': '#f2f2f2',
    'on-surface-variant': '#1f1f1f',
    primary: '#e879f9',
    'on-primary': '#1f1f1f',
    secondary: '#c7c7c7',
    error: '#f0561d',
    success: '#87bb62',
    warning: '#ffcc17',
    'on-warning': '#151515',
    info: '#b6a6e9',
    'on-background': '#ffffff',
    'on-surface': '#ffffff',
    'border-control': '#a3a3a3',
  },
  variables: {
    'border-color': '#4d4d4d',
    'border-opacity': 1,
    'high-emphasis-opacity': 1,
    'medium-emphasis-opacity': 0.78,
    // Keyboard focus is a ring (styles/patternfly.css), not a tinted overlay.
    'focus-opacity': 0,
  },
}

const lightHighContrast: ThemeDefinition = {
  dark: false,
  colors: {
    ...light.colors,
    primary: '#a21caf',
    secondary: '#383838',
    error: '#731f00',
    success: '#204d00',
    warning: '#73480b',
    'on-warning': '#ffffff',
    info: '#3d2785',
    'border-control': '#4d4d4d',
  },
  variables: {
    ...light.variables,
    'border-color': '#4d4d4d',
    'medium-emphasis-opacity': 0.85,
  },
}

const darkHighContrast: ThemeDefinition = {
  dark: true,
  colors: {
    ...dark.colors,
    surface: '#000000',
    'surface-bright': '#292929',
    'surface-light': '#292929',
    primary: '#f0abfc',
    error: '#f4784a',
    success: '#afdc8f',
    'border-control': '#c7c7c7',
  },
  variables: {
    ...dark.variables,
    'border-color': '#c7c7c7',
  },
}

// A focused field draws its outline in the accent, as PatternFly does.
const field = { variant: 'outlined', density: 'compact', color: 'primary' } as const

export const vuetify = createVuetify({
  theme: {
    defaultTheme: initialThemeName(),
    themes: { light, dark, 'light-hc': lightHighContrast, 'dark-hc': darkHighContrast },
  },
  // Vuetify's own texts and its dates follow the interface language. Date
  // fields and calendars take the input mask, month names and first weekday
  // from the locale: English keeps the browser's region when it has one.
  // The adapter's type takes any locale name, while ours knows only en and ru.
  locale: { adapter: createVueI18nAdapter({ i18n: i18n as unknown as Parameters<typeof createVueI18nAdapter>[0]['i18n'], useI18n }) },
  date: { locale: intlLocales },
  icons: { defaultSet: 'fa', aliases, sets: { fa: fontAwesome } },
  defaults: {
    VAppBar: { color: 'background', flat: true },
    VNavigationDrawer: { color: 'background', border: 0 },
    VBtn: { rounded: 'pill' },
    // Buttons in a toggle group join into one control, so they drop the pill.
    // Undefined rather than false: false adds `rounded-0`, which also squares
    // the outer corners of the end buttons, and the group's rounded clip then
    // cuts their border off. Without a class Vuetify's group styles square
    // the inner corners and let the end buttons inherit the outer radius.
    VBtnToggle: { rounded: 'lg', VBtn: { rounded: undefined } },
    VAlert: { variant: 'outlined' },
    VTextField: field,
    VTextarea: field,
    VSelect: field,
    VCombobox: field,
    VAutocomplete: field,
    VDateInput: field,
    VSwitch: { color: 'primary', inset: true },
    VCheckbox: { color: 'primary' },
    // PatternFly's compact tables: 8px above and below the cell text.
    VTable: { density: 'compact' },
    VTabs: { density: 'compact' },
  },
})
