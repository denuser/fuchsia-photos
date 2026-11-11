// The interface language. Messages live in en.ts, which is the source, and ru.ts, which must have
// the same keys: the type check fails on a missing or extra one. Vuetify's own texts come from its
// locales through the vue-i18n adapter, so they follow the same language.
import { computed } from 'vue'
import { createI18n } from 'vue-i18n'
import { en as vuetifyEn, ru as vuetifyRu } from 'vuetify/locale'

import type { Language as LanguageSetting } from '../api/client'
import en from './en'
import ru from './ru'

export type Language = Exclude<LanguageSetting, 'auto'>

export const languages: Language[] = ['en', 'ru']

type Messages = typeof en

// Russian has three plural forms: 1, 21 альбом; 2–4, 22 альбома; 5–20, 11, 0 альбомов. Messages
// list them in that order, or give one form for all, as "{n} фото" does.
const russianPlural = new Intl.PluralRules('ru')

function ruPlural(choice: number, choicesLength: number) {
  if (choicesLength === 1) {
    return 0
  }
  const form = { one: 0, few: 1 }[russianPlural.select(choice) as 'one' | 'few'] ?? 2
  return Math.min(form, choicesLength - 1)
}

// The first of the browser's languages that the interface supports, or English.
export function browserLanguage(): Language {
  for (const tag of navigator.languages ?? [navigator.language]) {
    const language = tag.split('-')[0].toLowerCase()
    if ((languages as string[]).includes(language)) {
      return language as Language
    }
  }
  return 'en'
}

export const i18n = createI18n({
  legacy: false,
  locale: browserLanguage(),
  fallbackLocale: 'en',
  messages: {
    en: { ...en, $vuetify: vuetifyEn },
    ru: { ...ru, $vuetify: vuetifyRu },
  },
  pluralRules: { ru: ruPlural },
})

// The dotted path of every message, such as 'common.cancel'.
type Leaves<T, Prefix extends string = ''> = {
  [K in keyof T & string]: T[K] extends string ? `${Prefix}${K}` : Leaves<T[K], `${Prefix}${K}.`>
}[keyof T & string]

export type MessageKey = Leaves<Messages>

type Named = Record<string, unknown>
type Rest = [] | [plural: number] | [named: Named] | [named: Named, plural: number] | [named: Named, options: { locale: Language }]

// vue-i18n's own t takes any string, so a mistyped key would show up only in the browser. This one
// takes only the keys of en.ts. It reads the current language, so templates update when it changes.
export function t(key: MessageKey, ...rest: Rest): string {
  return (i18n.global.t as (key: string, ...rest: unknown[]) => string)(key, ...rest)
}

// The locales for Intl and dates. English keeps the browser's region when the browser is set to an
// English variant, so that en-GB shows days before months; otherwise it is en-US.
export const intlLocales: Record<Language, string> = {
  en: navigator.languages?.find((tag) => tag.toLowerCase().startsWith('en-')) ?? 'en-US',
  ru: 'ru-RU',
}

export const intlLocale = computed(() => intlLocales[i18n.global.locale.value as Language])

export function setLanguage(setting: LanguageSetting | undefined) {
  const chosen = !setting || setting === 'auto' ? browserLanguage() : setting
  i18n.global.locale.value = chosen
  document.documentElement.lang = chosen
}

// A date as the date fields write it, such as 31.12.2026 or 12/31/2026.
export function formatDate(value: string | Date) {
  return new Date(value).toLocaleDateString(intlLocale.value, { year: 'numeric', month: '2-digit', day: '2-digit' })
}

// A date and time in short form, such as 31.12.2026, 14:05.
export function formatDateTime(value: string | Date) {
  return new Date(value).toLocaleString(intlLocale.value, { dateStyle: 'short', timeStyle: 'short' })
}
