import { useQueryClient } from '@tanstack/react-query'
import { Monitor, Moon, Sun } from 'lucide-react'
import { useTranslation } from 'react-i18next'

import { Button } from '@/components/ui/button'
import { languages, setLang, type Lang } from '@/i18n'
import { themePrefs, useTheme, type ThemePref } from '@/lib/theme'
import { cn } from '@/lib/utils'

const themeIcon: Record<ThemePref, typeof Sun> = { system: Monitor, light: Sun, dark: Moon }

/** Sprachumschalter DE | EN. Server-Meldungen sind sprachabhängig → neu laden. */
export function LanguageSwitch() {
  const { t, i18n } = useTranslation()
  const qc = useQueryClient()
  const current = i18n.language as Lang

  return (
    <div className="bg-muted flex rounded-md p-0.5" role="group" aria-label={t('prefs.language')}>
      {languages.map((l) => (
        <button
          key={l}
          type="button"
          aria-pressed={current === l}
          title={t(`prefs.languageName.${l}`)}
          onClick={() => {
            if (current === l) return
            setLang(l)
            void qc.invalidateQueries({ predicate: (q) => q.queryKey[0] !== 'session' })
          }}
          className={cn(
            'rounded px-2 py-0.5 text-xs font-medium uppercase transition-colors',
            current === l ? 'bg-background text-foreground shadow-xs' : 'text-muted-foreground hover:text-foreground',
          )}
        >
          {l}
        </button>
      ))}
    </div>
  )
}

/** Hell/Dunkel/System – wechselt reihum. */
export function ThemeSwitch() {
  const { t } = useTranslation()
  const [pref, setPref] = useTheme()
  const Icon = themeIcon[pref]
  const next = themePrefs[(themePrefs.indexOf(pref) + 1) % themePrefs.length]
  const label = t('prefs.theme', { mode: t(`prefs.themeMode.${pref}`) })

  return (
    <Button
      type="button"
      variant="ghost"
      size="icon"
      className="size-8"
      aria-label={label}
      title={`${label} – ${t('prefs.themeNext', { mode: t(`prefs.themeMode.${next}`) })}`}
      onClick={() => setPref(next)}
    >
      <Icon />
    </Button>
  )
}

export function Preferences({ className }: { className?: string }) {
  return (
    <div className={cn('flex items-center gap-1', className)}>
      <LanguageSwitch />
      <ThemeSwitch />
    </div>
  )
}
