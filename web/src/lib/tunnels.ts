import type { BucketStatus, Tunnel, TunnelStatus } from './api'
import { formatNumber } from './format'

type BadgeVariant = 'success' | 'warning' | 'destructive' | 'secondary'

// Beschriftungen: tunnel.status.*, tunnel.hint.*, tunnel.bucket.* (locales)
export const tunnelStatusVariant: Record<TunnelStatus, BadgeVariant> = {
  healthy: 'success',
  degraded: 'warning',
  down: 'destructive',
  inactive: 'secondary',
}

/** Schlüssel für Balken-Stücke ('' = keine Daten). */
export function bucketKey(b: BucketStatus) {
  return b === '' ? 'none' : b
}

export const bucketColor: Record<BucketStatus, string> = {
  healthy: 'bg-success',
  degraded: 'bg-warning',
  down: 'bg-destructive',
  inactive: 'bg-muted-foreground/40',
  '': 'bg-muted',
}

export function formatPercent(p: number | null): string {
  if (p === null) return '—'
  // 99,95 % soll nicht als 100 % erscheinen
  const v = p >= 99.995 ? 100 : Math.floor(p * 100) / 100
  return `${formatNumber(v)} %`
}

/** Eindeutige Rechenzentren (Colos) der aktiven Verbindungen, z. B. ["FRA06", "AMS01"]. */
export function colos(t: Tunnel): string[] {
  return [...new Set(t.connections.map((c) => c.colo_name.toUpperCase()))].sort()
}

export function clientVersions(t: Tunnel): string[] {
  return [...new Set(t.connections.map((c) => c.client_version).filter(Boolean))].sort()
}
