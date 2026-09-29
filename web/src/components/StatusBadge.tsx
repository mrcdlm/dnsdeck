import { Badge } from '@/components/ui/badge'
import type { IPStatus } from '@/lib/api'

const config: Record<IPStatus, { label: string; variant: 'success' | 'warning' | 'secondary' }> = {
  ok: { label: 'Bestätigt', variant: 'success' },
  unconfirmed: { label: 'Unbestätigt', variant: 'warning' },
  unavailable: { label: 'Nicht verfügbar', variant: 'secondary' },
  pending: { label: 'Ausstehend', variant: 'secondary' },
}

export function StatusBadge({ status }: { status: IPStatus }) {
  const c = config[status]
  return <Badge variant={c.variant}>{c.label}</Badge>
}
