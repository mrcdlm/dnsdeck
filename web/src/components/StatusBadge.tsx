import { useTranslation } from 'react-i18next'

import { Badge } from '@/components/ui/badge'
import type { IPStatus } from '@/lib/api'

const variant: Record<IPStatus, 'success' | 'warning' | 'secondary'> = {
  ok: 'success',
  unconfirmed: 'warning',
  unavailable: 'secondary',
  pending: 'secondary',
}

export function StatusBadge({ status }: { status: IPStatus }) {
  const { t } = useTranslation()
  return <Badge variant={variant[status]}>{t(`ip.status.${status}`)}</Badge>
}
