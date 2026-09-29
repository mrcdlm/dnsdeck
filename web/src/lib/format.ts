import { useEffect, useState } from 'react'

const rtf = new Intl.RelativeTimeFormat('de', { numeric: 'auto' })
const dtf = new Intl.DateTimeFormat('de-DE', { dateStyle: 'medium', timeStyle: 'short' })

const units: [Intl.RelativeTimeFormatUnit, number][] = [
  ['year', 365 * 24 * 3600],
  ['month', 30 * 24 * 3600],
  ['week', 7 * 24 * 3600],
  ['day', 24 * 3600],
  ['hour', 3600],
  ['minute', 60],
]

export function relativeTime(iso: string, now: number): string {
  const diff = (new Date(iso).getTime() - now) / 1000
  const abs = Math.abs(diff)
  if (abs < 45) return 'gerade eben'
  for (const [unit, secs] of units) {
    if (abs >= secs) return rtf.format(Math.round(diff / secs), unit)
  }
  return rtf.format(Math.round(diff / 60), 'minute')
}

export function absoluteTime(iso: string): string {
  return dtf.format(new Date(iso))
}

/** Liefert die aktuelle Zeit und aktualisiert sie regelmäßig (für relative Angaben). */
export function useNow(intervalMs = 30_000): number {
  const [now, setNow] = useState(() => Date.now())
  useEffect(() => {
    const id = setInterval(() => setNow(Date.now()), intervalMs)
    return () => clearInterval(id)
  }, [intervalMs])
  return now
}
