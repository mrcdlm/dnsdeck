import { AlertTriangle, ArrowLeft, ChevronRight, Globe, ListTree, Network, RefreshCw } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { Link } from 'react-router'

import { CopyButton } from '@/components/CopyButton'
import { StatusBadge } from '@/components/StatusBadge'
import { UpdateLogList } from '@/components/UpdateLogList'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Card, CardAction, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card'
import { Skeleton } from '@/components/ui/skeleton'
import type { Family, FamilyState } from '@/lib/api'
import { absoluteTime, relativeTime, useNow } from '@/lib/format'
import { useIP, useIPHistory, useRecords, useRefreshIP, useTunnels } from '@/lib/queries'
import { cn } from '@/lib/utils'

const familyLabel: Record<Family, string> = { ipv4: 'IPv4', ipv6: 'IPv6' }

function FamilyPanel({ family, state, now }: { family: Family; state: FamilyState; now: number }) {
  const { t } = useTranslation()
  const label = familyLabel[family]
  const failed = state.sources.filter((s) => s.error)

  return (
    <div className="flex min-w-0 flex-col gap-3 rounded-lg border p-5">
      <div className="flex items-center justify-between gap-2">
        <span className="text-muted-foreground text-sm font-medium">{label}</span>
        <StatusBadge status={state.status} />
      </div>

      {state.ip ? (
        <div className="flex items-start gap-1">
          <span
            className={cn(
              'min-w-0 font-mono font-semibold tracking-tight break-all',
              family === 'ipv4' ? 'text-3xl sm:text-4xl' : 'text-xl sm:text-2xl',
            )}
          >
            {state.ip}
          </span>
          <CopyButton value={state.ip} label={t('ip.copy', { family: label })} />
        </div>
      ) : (
        <span className="text-muted-foreground text-2xl font-semibold">—</span>
      )}

      <div className="text-muted-foreground flex flex-col gap-1 text-sm">
        {state.since && (
          <span title={absoluteTime(state.since)}>
            {t('ip.since', { date: absoluteTime(state.since), relative: relativeTime(state.since, now) })}
          </span>
        )}
        {state.responses > 0 && <span>{t('ip.agreement', { votes: state.votes, count: state.responses })}</span>}
        {state.message && state.status !== 'ok' && (
          <span className={cn('flex items-center gap-1.5', state.status === 'unconfirmed' && 'text-warning')}>
            {state.status === 'unconfirmed' && <AlertTriangle className="size-3.5 shrink-0" />}
            {state.message}
          </span>
        )}
        {failed.length > 0 && state.status !== 'unavailable' && (
          <details className="text-xs">
            <summary className="cursor-pointer select-none">{t('ip.sourcesFailed', { count: failed.length })}</summary>
            <ul className="mt-1 flex flex-col gap-0.5">
              {failed.map((s) => (
                <li key={s.source} className="break-all">
                  <span className="font-medium">{s.source}:</span> {s.error}
                </li>
              ))}
            </ul>
          </details>
        )}
      </div>
    </div>
  )
}

function IPCard() {
  const { t } = useTranslation()
  const ip = useIP()
  const refresh = useRefreshIP()
  const now = useNow()

  return (
    <Card>
      <CardHeader>
        <CardTitle className="flex items-center gap-2 text-lg">
          <Globe className="size-5" />
          {t('ip.title')}
        </CardTitle>
        <CardDescription>
          {ip.data?.last_checked ? (
            <span title={absoluteTime(ip.data.last_checked)}>
              {t('ip.lastChecked', { time: relativeTime(ip.data.last_checked, now) })}
            </span>
          ) : (
            t('ip.notChecked')
          )}
        </CardDescription>
        <CardAction>
          <Button
            variant="outline"
            onClick={() => refresh.mutate()}
            disabled={refresh.isPending}
            aria-label={t('common.refreshNow')}
            title={t('ip.refreshHint')}
          >
            <RefreshCw className={cn(refresh.isPending && 'animate-spin')} />
            <span className="hidden sm:inline">{t('common.refreshNow')}</span>
          </Button>
        </CardAction>
      </CardHeader>
      <CardContent className="flex flex-col gap-4">
        {refresh.isError && (
          <p className="text-destructive text-sm" role="alert">
            {t('common.refreshFailed', { error: refresh.error.message })}
          </p>
        )}
        {ip.isPending ? (
          <div className="grid gap-4 md:grid-cols-2">
            <Skeleton className="h-36" />
            <Skeleton className="h-36" />
          </div>
        ) : ip.isError ? (
          <p className="text-destructive text-sm">{t('ip.loadError', { error: ip.error.message })}</p>
        ) : (
          <div className="grid gap-4 md:grid-cols-2">
            <FamilyPanel family="ipv4" state={ip.data.ipv4} now={now} />
            <FamilyPanel family="ipv6" state={ip.data.ipv6} now={now} />
          </div>
        )}
      </CardContent>
    </Card>
  )
}

function IPChangesCard() {
  const { t } = useTranslation()
  const history = useIPHistory(10)
  const now = useNow()

  return (
    <Card>
      <CardHeader>
        <CardTitle className="text-lg">{t('dashboard.ipChanges')}</CardTitle>
        <CardDescription>{t('dashboard.ipChangesHint')}</CardDescription>
      </CardHeader>
      <CardContent>
        {history.isPending ? (
          <div className="flex flex-col gap-2">
            <Skeleton className="h-10" />
            <Skeleton className="h-10" />
            <Skeleton className="h-10" />
          </div>
        ) : history.isError ? (
          <p className="text-destructive text-sm">{t('history.loadError', { error: history.error.message })}</p>
        ) : history.data.length === 0 ? (
          <p className="text-muted-foreground text-sm">{t('dashboard.ipChangesEmpty')}</p>
        ) : (
          <ul className="divide-y">
            {history.data.map((c) => (
              <li key={c.id} className="flex flex-wrap items-center gap-x-3 gap-y-1 py-3 first:pt-0 last:pb-0">
                <Badge variant="outline" className="w-12">
                  {familyLabel[c.family]}
                </Badge>
                <span className="min-w-0 font-mono text-sm break-all">{c.ip}</span>
                {c.previous_ip && (
                  <span className="text-muted-foreground flex min-w-0 items-center gap-1 font-mono text-xs break-all">
                    <ArrowLeft className="size-3 shrink-0" />
                    {c.previous_ip}
                  </span>
                )}
                <span className="text-muted-foreground ml-auto text-xs" title={absoluteTime(c.detected_at)}>
                  {relativeTime(c.detected_at, now)}
                </span>
              </li>
            ))}
          </ul>
        )}
      </CardContent>
    </Card>
  )
}

function RecordsTile() {
  const { t } = useTranslation()
  const records = useRecords()
  const counts = { ok: 0, error: 0, other: 0 }
  for (const r of records.data ?? []) {
    if (r.status === 'ok') counts.ok++
    else if (r.status === 'error') counts.error++
    else counts.other++
  }
  const total = records.data?.length ?? 0

  return (
    <Link
      to="/records"
      className="group bg-card hover:bg-accent/40 flex items-center gap-4 rounded-xl border p-5 transition-colors"
    >
      <span className="bg-muted grid size-10 shrink-0 place-items-center rounded-lg">
        <ListTree className="size-5" />
      </span>
      <div className="min-w-0 flex-1">
        <div className="text-muted-foreground text-sm">{t('dashboard.records')}</div>
        {records.isPending ? (
          <Skeleton className="mt-1 h-6 w-32" />
        ) : total === 0 ? (
          <div className="font-medium">{t('dashboard.noRecords')}</div>
        ) : (
          <div className="flex flex-wrap items-center gap-2 font-medium">
            <span>{t('dashboard.managed', { count: total })}</span>
            {counts.ok > 0 && <Badge variant="success">{t('dashboard.upToDate', { count: counts.ok })}</Badge>}
            {counts.error > 0 && <Badge variant="destructive">{t('dashboard.errors', { count: counts.error })}</Badge>}
            {counts.other > 0 && <Badge variant="secondary">{t('dashboard.other', { count: counts.other })}</Badge>}
          </div>
        )}
      </div>
      <ChevronRight className="text-muted-foreground size-4 transition-transform group-hover:translate-x-0.5" />
    </Link>
  )
}

function TunnelsTile() {
  const { t } = useTranslation()
  const tunnels = useTunnels()
  const o = tunnels.data
  const counts = { up: 0, degraded: 0, down: 0 }
  for (const tn of o?.tunnels ?? []) {
    if (tn.status === 'healthy') counts.up++
    else if (tn.status === 'degraded') counts.degraded++
    else counts.down++
  }
  const total = o?.tunnels.length ?? 0

  return (
    <Link
      to="/tunnels"
      className="group bg-card hover:bg-accent/40 flex items-center gap-4 rounded-xl border p-5 transition-colors"
    >
      <span className="bg-muted grid size-10 shrink-0 place-items-center rounded-lg">
        <Network className="size-5" />
      </span>
      <div className="min-w-0 flex-1">
        <div className="text-muted-foreground text-sm">{t('dashboard.tunnels')}</div>
        {tunnels.isPending ? (
          <Skeleton className="mt-1 h-6 w-32" />
        ) : !o?.configured ? (
          <div className="text-muted-foreground font-medium">{t('dashboard.notConfigured')}</div>
        ) : total === 0 ? (
          <div className="font-medium">{t('dashboard.noTunnels')}</div>
        ) : (
          <div className="flex flex-wrap items-center gap-2 font-medium">
            <span>{t('dashboard.monitored', { count: total })}</span>
            {counts.up > 0 && <Badge variant="success">{t('dashboard.connected', { count: counts.up })}</Badge>}
            {counts.degraded > 0 && (
              <Badge variant="warning">{t('dashboard.degraded', { count: counts.degraded })}</Badge>
            )}
            {counts.down > 0 && <Badge variant="destructive">{t('dashboard.down', { count: counts.down })}</Badge>}
          </div>
        )}
        {o?.error && <div className="text-destructive mt-1 text-xs">{t('tunnels.lastPollFailedShort')}</div>}
      </div>
      <ChevronRight className="text-muted-foreground size-4 transition-transform group-hover:translate-x-0.5" />
    </Link>
  )
}

export function Dashboard() {
  const { t } = useTranslation()
  return (
    <div className="flex flex-col gap-6">
      <div>
        <h1 className="text-2xl font-semibold tracking-tight">{t('nav.dashboard')}</h1>
        <p className="text-muted-foreground text-sm">{t('dashboard.subtitle')}</p>
      </div>
      <IPCard />
      <div className="grid gap-4 md:grid-cols-2">
        <RecordsTile />
        <TunnelsTile />
      </div>
      <div className="grid gap-6 lg:grid-cols-2">
        <IPChangesCard />
        <Card>
          <CardHeader>
            <CardTitle className="text-lg">{t('dashboard.dnsUpdates')}</CardTitle>
            <CardDescription>{t('dashboard.dnsUpdatesHint')}</CardDescription>
          </CardHeader>
          <CardContent>
            <UpdateLogList limit={5} />
          </CardContent>
        </Card>
      </div>
    </div>
  )
}
