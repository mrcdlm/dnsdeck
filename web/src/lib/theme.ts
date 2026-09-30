import { useSyncExternalStore } from 'react'

export type ThemePref = 'system' | 'light' | 'dark'
export const themePrefs: ThemePref[] = ['system', 'light', 'dark']

const STORAGE_KEY = 'dnsdeck.theme'

function readPref(): ThemePref {
  try {
    const v = localStorage.getItem(STORAGE_KEY)
    if (v === 'light' || v === 'dark' || v === 'system') return v
  } catch {
    // Speicher nicht verfügbar
  }
  return 'system'
}

// Ein globaler Zustand für alle Umschalter (Seitenleiste, mobiler Header, Login).
let pref: ThemePref = readPref()
const listeners = new Set<() => void>()
const media = window.matchMedia('(prefers-color-scheme: dark)')

function apply() {
  const dark = pref === 'dark' || (pref === 'system' && media.matches)
  document.documentElement.classList.toggle('dark', dark)
  document.documentElement.style.colorScheme = dark ? 'dark' : 'light'
}

// "system" folgt live der Betriebssystem-Einstellung.
media.addEventListener('change', () => pref === 'system' && apply())
apply()

export function setThemePref(p: ThemePref) {
  pref = p
  try {
    localStorage.setItem(STORAGE_KEY, p)
  } catch {
    // ignorieren
  }
  apply()
  listeners.forEach((l) => l())
}

function subscribe(l: () => void) {
  listeners.add(l)
  return () => listeners.delete(l)
}

export function useTheme(): [ThemePref, (p: ThemePref) => void] {
  return [useSyncExternalStore(subscribe, () => pref), setThemePref]
}
