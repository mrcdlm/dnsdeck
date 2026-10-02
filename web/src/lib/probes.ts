import type { Probe, ProbeStatus } from './api'

type BadgeVariant = 'success' | 'warning' | 'destructive' | 'secondary' | 'outline'

export const probeStatusVariant: Record<ProbeStatus, BadgeVariant> = {
  up: 'success',
  expiring: 'warning',
  tls_error: 'destructive',
  down: 'destructive',
  pending: 'secondary',
  paused: 'outline',
}

/** Ganze Tage bis zum Ablauf des Zertifikats (negativ = abgelaufen). */
export function certDaysLeft(p: Probe, now: number): number | undefined {
  if (!p.tls_not_after) return undefined
  return Math.floor((Date.parse(p.tls_not_after) - now) / 86_400_000)
}

/** Prüfung eines Records nach Record-ID. */
export function probesByRecord(probes: Probe[] | undefined): Map<number, Probe> {
  const m = new Map<number, Probe>()
  for (const p of probes ?? []) if (p.record_id !== undefined) m.set(p.record_id, p)
  return m
}
