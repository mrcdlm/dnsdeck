import type { BucketStatus, Tunnel, TunnelStatus } from './api'

type BadgeVariant = 'success' | 'warning' | 'destructive' | 'secondary'

export const tunnelStatus: Record<TunnelStatus, { label: string; variant: BadgeVariant; hint: string }> = {
  healthy: { label: 'Verbunden', variant: 'success', hint: 'healthy – alle Verbindungen aktiv' },
  degraded: { label: 'Eingeschränkt', variant: 'warning', hint: 'degraded – nicht alle Verbindungen aktiv' },
  down: { label: 'Getrennt', variant: 'destructive', hint: 'down – keine aktive Verbindung' },
  inactive: { label: 'Inaktiv', variant: 'secondary', hint: 'inactive – noch nie verbunden' },
}

export const bucketColor: Record<BucketStatus, string> = {
  healthy: 'bg-success',
  degraded: 'bg-warning',
  down: 'bg-destructive',
  inactive: 'bg-muted-foreground/40',
  '': 'bg-muted',
}

export const bucketLabel: Record<BucketStatus, string> = {
  healthy: 'verbunden',
  degraded: 'eingeschränkt',
  down: 'getrennt',
  inactive: 'inaktiv',
  '': 'keine Daten',
}

export function formatPercent(p: number | null): string {
  if (p === null) return '—'
  // 99,95 % soll nicht als 100 % erscheinen
  const v = p >= 99.995 ? 100 : Math.floor(p * 100) / 100
  return `${v.toLocaleString('de-DE', { maximumFractionDigits: 2 })} %`
}

/** Eindeutige Rechenzentren (Colos) der aktiven Verbindungen, z. B. ["FRA06", "AMS01"]. */
export function colos(t: Tunnel): string[] {
  return [...new Set(t.connections.map((c) => c.colo_name.toUpperCase()))].sort()
}

export function clientVersions(t: Tunnel): string[] {
  return [...new Set(t.connections.map((c) => c.client_version).filter(Boolean))].sort()
}
