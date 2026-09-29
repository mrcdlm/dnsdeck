import type { RecordStatus, UpdateResult, UpdateTrigger } from './api'

export const ttlOptions: { value: number; label: string }[] = [
  { value: 1, label: 'Automatisch' },
  { value: 60, label: '1 Minute' },
  { value: 120, label: '2 Minuten' },
  { value: 300, label: '5 Minuten' },
  { value: 600, label: '10 Minuten' },
  { value: 1800, label: '30 Minuten' },
  { value: 3600, label: '1 Stunde' },
  { value: 86400, label: '1 Tag' },
]

export function formatTTL(ttl: number): string {
  if (ttl === 1) return 'Auto'
  if (ttl % 86400 === 0) return `${ttl / 86400} d`
  if (ttl % 3600 === 0) return `${ttl / 3600} h`
  if (ttl % 60 === 0) return `${ttl / 60} min`
  return `${ttl} s`
}

type BadgeVariant = 'success' | 'warning' | 'destructive' | 'secondary' | 'outline'

export const recordStatus: Record<RecordStatus, { label: string; variant: BadgeVariant }> = {
  ok: { label: 'Aktuell', variant: 'success' },
  error: { label: 'Fehler', variant: 'destructive' },
  pending: { label: 'Ausstehend', variant: 'secondary' },
  skipped: { label: 'Übersprungen', variant: 'warning' },
  paused: { label: 'Pausiert', variant: 'outline' },
}

export const updateResult: Record<UpdateResult, { label: string; variant: BadgeVariant }> = {
  created: { label: 'Angelegt', variant: 'success' },
  adopted: { label: 'Übernommen', variant: 'secondary' },
  updated: { label: 'Aktualisiert', variant: 'success' },
  error: { label: 'Fehler', variant: 'destructive' },
}

export const updateTrigger: Record<UpdateTrigger, string> = {
  scheduled: 'automatisch',
  manual: 'manuell',
  record_saved: 'nach Speichern',
}

/** Teil des Namens vor der Zone ("" = Zone selbst). */
export function subdomainOf(name: string, zone: string): string {
  if (name === zone) return ''
  return name.endsWith('.' + zone) ? name.slice(0, -(zone.length + 1)) : name
}
