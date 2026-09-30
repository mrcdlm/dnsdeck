import i18n from 'i18next'
import { initReactI18next } from 'react-i18next'

import de from '@/locales/de.json'
import en from '@/locales/en.json'

export type Lang = 'de' | 'en'
export const languages: Lang[] = ['de', 'en']

const STORAGE_KEY = 'dnsdeck.lang'

/** Gespeicherte Wahl, sonst bevorzugte Browsersprache (de* → Deutsch), sonst Englisch. */
function detect(): Lang {
  try {
    const saved = localStorage.getItem(STORAGE_KEY)
    if (saved === 'de' || saved === 'en') return saved
  } catch {
    // Speicher nicht verfügbar (z. B. privates Fenster)
  }
  const preferred = navigator.languages?.[0] ?? navigator.language ?? ''
  return preferred.toLowerCase().startsWith('de') ? 'de' : 'en'
}

void i18n.use(initReactI18next).init({
  resources: { de: { translation: de }, en: { translation: en } },
  lng: detect(),
  fallbackLng: 'en',
  interpolation: { escapeValue: false }, // React escaped selbst
  returnNull: false,
})
document.documentElement.lang = i18n.language

export function currentLang(): Lang {
  return i18n.language === 'de' ? 'de' : 'en'
}

/** Sprache wechseln und im Browser merken. */
export function setLang(lang: Lang) {
  void i18n.changeLanguage(lang)
  document.documentElement.lang = lang
  try {
    localStorage.setItem(STORAGE_KEY, lang)
  } catch {
    // ignorieren
  }
}

export default i18n
