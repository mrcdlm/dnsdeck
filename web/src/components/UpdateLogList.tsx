import { ArrowRight } from 'lucide-react'
import { useTranslation } from 'react-i18next'

import { Badge } from '@/components/ui/badge'
import { Skeleton } from '@/components/ui/skeleton'
import { absoluteTime, relativeTime, useNow } from '@/lib/format'
import { useUpdates } from '@/lib/queries'
import { updateResultVariant } from '@/lib/records'

export function UpdateLogList({ limit, empty }: { limit: number; empty?: string }) {
  const { t } = useTranslation()
  const updates = useUpdates(limit)
  const now = useNow()

  if (updates.isPending) {
    return (
      <div className="flex flex-col gap-2">
        <Skeleton className="h-12" />
        <Skeleton className="h-12" />
      </div>
    )
  }
  if (updates.isError) {
    return <p className="text-destructive text-sm">{t('updates.loadError', { error: updates.error.message })}</p>
  }
  if (updates.data.length === 0) {
    return <p className="text-muted-foreground text-sm">{empty ?? t('updates.empty')}</p>
  }

  return (
    <ul className="divide-y">
      {updates.data.map((e) => (
        <li key={e.id} className="flex flex-col gap-1 py-3 first:pt-0 last:pb-0">
          <div className="flex flex-wrap items-center gap-x-2 gap-y-1">
            <Badge variant={updateResultVariant[e.result]}>{t(`update.result.${e.result}`)}</Badge>
            <span className="min-w-0 font-medium break-all">{e.record_name}</span>
            <Badge variant="outline">{e.record_type}</Badge>
            <span className="text-muted-foreground ml-auto text-xs" title={absoluteTime(e.created_at)}>
              {relativeTime(e.created_at, now)} · {t(`update.trigger.${e.trigger}`)}
            </span>
          </div>
          {(e.old_ip || e.new_ip) && (
            <div className="text-muted-foreground flex flex-wrap items-center gap-1.5 font-mono text-xs">
              {e.old_ip !== e.new_ip && (
                <>
                  <span className="break-all">{e.old_ip || '—'}</span>
                  <ArrowRight className="size-3 shrink-0" />
                </>
              )}
              <span className="text-foreground break-all">{e.new_ip || '—'}</span>
            </div>
          )}
          {e.message && (
            <p className={e.result === 'error' ? 'text-destructive text-xs break-words' : 'text-muted-foreground text-xs'}>
              {e.message}
            </p>
          )}
        </li>
      ))}
    </ul>
  )
}
