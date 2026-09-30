import 'i18next'

import type de from '@/locales/de.json'

// Schlüssel werden gegen die deutsche Datei geprüft (Tippfehler = Build-Fehler).
declare module 'i18next' {
  interface CustomTypeOptions {
    resources: { translation: typeof de }
    returnNull: false
  }
}
