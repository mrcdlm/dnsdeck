import type { DnsRecord } from '@/lib/api'

/** DNS-Verbreitung eines Records (Platzhalter bis zur Prüfung im Backend). */
export function PropagationBadge(_: { record: DnsRecord }) {
  return <span className="text-muted-foreground">—</span>
}
