import { createVuetify } from 'vuetify'

const primary = '#c026d3'

export const vuetify = createVuetify({
  theme: {
    defaultTheme: 'system',
    themes: {
      light: { colors: { primary } },
      dark: { colors: { primary: '#e879f9' } },
    },
  },
})
