// The interface language. Messages live in en.ts, which is the source, and one file per other
// language, which must have the same keys: the type check fails on a missing or extra one. Vuetify's
// own texts come from its locales through the vue-i18n adapter, so they follow the same language.
import { computed } from 'vue'
import { createI18n } from 'vue-i18n'
import {
  de as vuetifyDe,
  en as vuetifyEn,
  es as vuetifyEs,
  fr as vuetifyFr,
  it as vuetifyIt,
  ja as vuetifyJa,
  ko as vuetifyKo,
  pt as vuetifyPt,
  ru as vuetifyRu,
  zhHans as vuetifyZh,
} from 'vuetify/locale'

import type { Language as LanguageSetting } from '../api/client'
import de from './de'
import en from './en'
import es from './es'
import fr from './fr'
import it from './it'
import ja from './ja'
import ko from './ko'
import pt from './pt'
import ru from './ru'
import zh from './zh'

export type Language = Exclude<LanguageSetting, 'auto'>

// The codes are the primary subtags of browser languages: zh is Simplified Chinese, pt is Brazilian
// Portuguese. The order is the order of the profile's list.
export const languages: Language[] = ['en', 'de', 'es', 'fr', 'it', 'pt', 'ru', 'ja', 'ko', 'zh']

type Messages = typeof en

// Plural forms come from the language's CLDR rules. A message lists its forms in the order below, or
// gives one form for all, as Chinese, Japanese and Korean always do and "{n} фото" does. Russian has
// three: 1, 21 альбом; 2–4, 22 альбома; 5–20, 11, 0 альбомов. The others write "one | other"; French
// counts 0 as one. A category missing from the order, such as "many" for a million in French, takes
// the last form.
const pluralOrder: Partial<Record<Language, Intl.LDMLPluralRule[]>> = { ru: ['one', 'few', 'many'] }

function pluralRule(language: Language) {
  const rules = new Intl.PluralRules(language)
  const order = pluralOrder[language] ?? ['one', 'other']
  return (choice: number, choicesLength: number) => {
    if (choicesLength === 1) {
      return 0
    }
    const form = order.indexOf(rules.select(choice))
    return Math.min(form < 0 ? order.length - 1 : form, choicesLength - 1)
  }
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
    zh: { ...zh, $vuetify: vuetifyZh },
    es: { ...es, $vuetify: vuetifyEs },
    pt: { ...pt, $vuetify: vuetifyPt },
    de: { ...de, $vuetify: vuetifyDe },
    fr: { ...fr, $vuetify: vuetifyFr },
    ja: { ...ja, $vuetify: vuetifyJa },
    ko: { ...ko, $vuetify: vuetifyKo },
    it: { ...it, $vuetify: vuetifyIt },
  },
  pluralRules: Object.fromEntries(languages.map((language) => [language, pluralRule(language)])),
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
  zh: 'zh-CN',
  es: 'es-ES',
  pt: 'pt-BR',
  de: 'de-DE',
  fr: 'fr-FR',
  ja: 'ja-JP',
  ko: 'ko-KR',
  it: 'it-IT',
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
