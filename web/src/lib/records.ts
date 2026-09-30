import type { RecordStatus, UpdateResult, UpdateTrigger } from './api'

/** TTL-Auswahl in Sekunden (1 = automatisch); Beschriftung über ttlLabel. */
export const ttlOptions = [1, 60, 120, 300, 600, 1800, 3600, 86400]

export function formatTTL(ttl: number): string {
  if (ttl === 1) return 'Auto'
  if (ttl % 86400 === 0) return `${ttl / 86400} d`
  if (ttl % 3600 === 0) return `${ttl / 3600} h`
  if (ttl % 60 === 0) return `${ttl / 60} min`
  return `${ttl} s`
}

type BadgeVariant = 'success' | 'warning' | 'destructive' | 'secondary' | 'outline'

// Beschriftungen: record.status.*, update.result.*, update.trigger.* (locales)
export const recordStatusVariant: Record<RecordStatus, BadgeVariant> = {
  ok: 'success',
  error: 'destructive',
  pending: 'secondary',
  skipped: 'warning',
  paused: 'outline',
}

export const updateResultVariant: Record<UpdateResult, BadgeVariant> = {
  created: 'success',
  adopted: 'secondary',
  updated: 'success',
  recovered: 'success',
  error: 'destructive',
}

export const updateResults: UpdateResult[] = ['created', 'adopted', 'updated', 'recovered', 'error']
export type { UpdateTrigger }

/** Teil des Namens vor der Zone ("" = Zone selbst). */
export function subdomainOf(name: string, zone: string): string {
  if (name === zone) return ''
  return name.endsWith('.' + zone) ? name.slice(0, -(zone.length + 1)) : name
}
