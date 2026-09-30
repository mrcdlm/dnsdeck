import { ArrowRight, Loader2 } from 'lucide-react'
import { useState, type ReactNode } from 'react'
import { useTranslation } from 'react-i18next'

import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Card } from '@/components/ui/card'
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select'
import { Skeleton } from '@/components/ui/skeleton'
import type { Family, UpdateResult } from '@/lib/api'
import { absoluteTime, relativeTime, useNow } from '@/lib/format'
import { useIPHistoryPages, useRecords, useTunnelHistoryPages, useTunnels, useUpdatePages } from '@/lib/queries'
import { updateResults, updateResultVariant } from '@/lib/records'
import { tunnelStatusVariant } from '@/lib/tunnels'
import { cn } from '@/lib/utils'

type Tab = 'updates' | 'ip' | 'tunnels'
const tabs: Tab[] = ['updates', 'ip', 'tunnels']
const ALL = 'all'

/** Gemeinsames Gerüst für eine blätterbare Liste. */
function PagedList<T>({
  query,
  empty,
  render,
}: {
  query: {
    data?: { pages: T[][] }
    isPending: boolean
    isError: boolean
    error: Error | null
    hasNextPage: boolean
    isFetchingNextPage: boolean
    fetchNextPage: () => unknown
  }
  empty: string
  render: (item: T) => ReactNode
}) {
  const { t } = useTranslation()
  if (query.isPending) {
    return (
      <div className="flex flex-col gap-2 p-6">
        <Skeleton className="h-10" />
        <Skeleton className="h-10" />
        <Skeleton className="h-10" />
      </div>
    )
  }
  if (query.isError) {
    return <p className="text-destructive p-6 text-sm">{t('history.loadError', { error: query.error?.message })}</p>
  }
  const items = query.data?.pages.flat() ?? []
  if (items.length === 0) {
    return <p className="text-muted-foreground p-6 text-sm">{empty}</p>
  }
  return (
    <>
      <ul className="divide-y">{items.map(render)}</ul>
      {query.hasNextPage && (
        <div className="border-t p-3 text-center">
          <Button variant="ghost" onClick={() => query.fetchNextPage()} disabled={query.isFetchingNextPage}>
            {query.isFetchingNextPage && <Loader2 className="animate-spin" />}
            {t('history.loadOlder')}
          </Button>
        </div>
      )}
    </>
  )
}

function When({ iso, now }: { iso: string; now: number }) {
  return (
    <span className="text-muted-foreground shrink-0 text-xs tabular-nums" title={relativeTime(iso, now)}>
      {absoluteTime(iso)}
    </span>
  )
}

function FilterSelect({
  label,
  value,
  onChange,
  options,
}: {
  label: string
  value: string
  onChange: (v: string) => void
  options: { value: string; label: string }[]
}) {
  return (
    <Select value={value} onValueChange={onChange}>
      <SelectTrigger className="w-full sm:w-56" aria-label={label}>
        <SelectValue />
      </SelectTrigger>
      <SelectContent>
        {options.map((o) => (
          <SelectItem key={o.value} value={o.value}>
            {o.label}
          </SelectItem>
        ))}
      </SelectContent>
    </Select>
  )
}

function UpdatesTab({ now }: { now: number }) {
  const { t } = useTranslation()
  const records = useRecords()
  const [recordId, setRecordId] = useState(ALL)
  const [result, setResult] = useState(ALL)
  const query = useUpdatePages({
    record_id: recordId === ALL ? undefined : Number(recordId),
    result: result === ALL ? undefined : (result as UpdateResult),
  })

  return (
    <>
      <div className="flex flex-col gap-2 border-b p-4 sm:flex-row">
        <FilterSelect
          label={t('history.filter.record')}
          value={recordId}
          onChange={setRecordId}
          options={[
            { value: ALL, label: t('history.filter.allRecords') },
            ...(records.data ?? []).map((r) => ({ value: String(r.id), label: `${r.name} (${r.type})` })),
          ]}
        />
        <FilterSelect
          label={t('history.filter.result')}
          value={result}
          onChange={setResult}
          options={[
            { value: ALL, label: t('history.filter.allResults') },
            ...updateResults.map((r) => ({ value: r, label: t(`update.result.${r}`) })),
          ]}
        />
      </div>
      <PagedList
        query={query}
        empty={t('history.emptyFiltered')}
        render={(e) => (
          <li key={e.id} className="flex flex-col gap-1 px-4 py-3 sm:px-6">
            <div className="flex flex-wrap items-center gap-x-2 gap-y-1">
              <Badge variant={updateResultVariant[e.result]}>{t(`update.result.${e.result}`)}</Badge>
              <span className="min-w-0 font-medium break-all">{e.record_name}</span>
              <Badge variant="outline">{e.record_type}</Badge>
              <span className="text-muted-foreground text-xs">{t(`update.trigger.${e.trigger}`)}</span>
              <span className="ml-auto">
                <When iso={e.created_at} now={now} />
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
              <p className={cn('text-xs break-words', e.result === 'error' ? 'text-destructive' : 'text-muted-foreground')}>
                {e.message}
              </p>
            )}
          </li>
        )}
      />
    </>
  )
}

function IPTab({ now }: { now: number }) {
  const { t } = useTranslation()
  const [family, setFamily] = useState(ALL)
  const query = useIPHistoryPages({ family: family === ALL ? undefined : (family as Family) })

  return (
    <>
      <div className="border-b p-4">
        <FilterSelect
          label={t('history.filter.family')}
          value={family}
          onChange={setFamily}
          options={[
            { value: ALL, label: t('history.filter.bothFamilies') },
            { value: 'ipv4', label: t('history.filter.onlyV4') },
            { value: 'ipv6', label: t('history.filter.onlyV6') },
          ]}
        />
      </div>
      <PagedList
        query={query}
        empty={t('dashboard.ipChangesEmpty')}
        render={(c) => (
          <li key={c.id} className="flex flex-wrap items-center gap-x-3 gap-y-1 px-4 py-3 sm:px-6">
            <Badge variant="outline" className="w-12">
              {c.family === 'ipv4' ? 'IPv4' : 'IPv6'}
            </Badge>
            <span className="text-muted-foreground font-mono text-xs break-all">
              {c.previous_ip ?? t('history.firstDetected')}
            </span>
            <ArrowRight className="text-muted-foreground size-3 shrink-0" />
            <span className="font-mono text-sm break-all">{c.ip}</span>
            <span className="ml-auto">
              <When iso={c.detected_at} now={now} />
            </span>
          </li>
        )}
      />
    </>
  )
}

function TunnelsTab({ now }: { now: number }) {
  const { t } = useTranslation()
  const tunnels = useTunnels()
  const [tunnelId, setTunnelId] = useState(ALL)
  const query = useTunnelHistoryPages({ tunnel_id: tunnelId === ALL ? undefined : tunnelId })

  return (
    <>
      <div className="border-b p-4">
        <FilterSelect
          label={t('history.filter.tunnel')}
          value={tunnelId}
          onChange={setTunnelId}
          options={[
            { value: ALL, label: t('history.filter.allTunnels') },
            ...(tunnels.data?.tunnels ?? []).map((tn) => ({ value: tn.id, label: tn.name })),
          ]}
        />
      </div>
      <PagedList
        query={query}
        empty={t('history.tunnelsEmpty')}
        render={(c) => (
          <li key={`${c.tunnel_id}-${c.at}`} className="flex flex-wrap items-center gap-x-3 gap-y-1 px-4 py-3 sm:px-6">
            <span className="font-medium">{c.tunnel_name}</span>
            <span className="text-muted-foreground text-xs">
              {c.from ? t(`tunnel.status.${c.from}`) : t('history.firstObserved')}
            </span>
            <ArrowRight className="text-muted-foreground size-3 shrink-0" />
            <Badge variant={tunnelStatusVariant[c.to]}>{t(`tunnel.status.${c.to}`)}</Badge>
            <span className="ml-auto">
              <When iso={c.at} now={now} />
            </span>
          </li>
        )}
      />
    </>
  )
}

export function History() {
  const { t } = useTranslation()
  const [tab, setTab] = useState<Tab>('updates')
  const now = useNow()

  return (
    <div className="flex flex-col gap-6">
      <div>
        <h1 className="text-2xl font-semibold tracking-tight">{t('nav.history')}</h1>
        <p className="text-muted-foreground text-sm">{t('history.subtitle')}</p>
      </div>

      <div className="bg-muted flex w-fit max-w-full overflow-x-auto rounded-md p-0.5" role="tablist">
        {tabs.map((id) => (
          <button
            key={id}
            type="button"
            role="tab"
            aria-selected={tab === id}
            onClick={() => setTab(id)}
            className={cn(
              'shrink-0 rounded px-3 py-1.5 text-sm font-medium transition-colors',
              tab === id ? 'bg-background shadow-xs' : 'text-muted-foreground hover:text-foreground',
            )}
          >
            {t(`history.tab.${id}`)}
          </button>
        ))}
      </div>

      <Card className="gap-0 py-0">
        {tab === 'updates' && <UpdatesTab now={now} />}
        {tab === 'ip' && <IPTab now={now} />}
        {tab === 'tunnels' && <TunnelsTab now={now} />}
      </Card>
    </div>
  )
}
