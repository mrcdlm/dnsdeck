import { AlertTriangle, Network, RefreshCw } from 'lucide-react'
import { useState } from 'react'
import { Trans, useTranslation } from 'react-i18next'

import { UptimeBar } from '@/components/UptimeBar'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { Skeleton } from '@/components/ui/skeleton'
import type { Tunnel } from '@/lib/api'
import { absoluteTime, relativeTime, useNow } from '@/lib/format'
import { useRefreshTunnels, useTunnels } from '@/lib/queries'
import { bucketColor, bucketKey, clientVersions, colos, tunnelStatusVariant } from '@/lib/tunnels'
import { cn } from '@/lib/utils'

type Range = '24h' | '7d'
const ranges: Range[] = ['24h', '7d']

function Timestamp({ label, iso, now }: { label: string; iso?: string; now: number }) {
  if (!iso) return null
  return (
    <div className="flex justify-between gap-2">
      <dt className="text-muted-foreground">{label}</dt>
      <dd title={absoluteTime(iso)}>{relativeTime(iso, now)}</dd>
    </div>
  )
}

function TunnelCard({ tunnel, range, now }: { tunnel: Tunnel; range: Range; now: number }) {
  const { t } = useTranslation()
  const up = tunnel.status === 'healthy' || tunnel.status === 'degraded'
  const versions = clientVersions(tunnel)

  return (
    <Card className="gap-4">
      <CardHeader>
        <div className="flex items-start justify-between gap-2">
          <CardTitle className="min-w-0 text-base [overflow-wrap:anywhere]">{tunnel.name}</CardTitle>
          <Badge variant={tunnelStatusVariant[tunnel.status]} title={t(`tunnel.hint.${tunnel.status}`)}>
            {t(`tunnel.status.${tunnel.status}`)}
          </Badge>
        </div>
      </CardHeader>
      <CardContent className="flex flex-col gap-4">
        <div className="flex flex-wrap items-center gap-2 text-sm">
          <span className="font-medium">{t('tunnels.connections', { count: tunnel.connections.length })}</span>
          {colos(tunnel).map((c) => (
            <Badge key={c} variant="outline" className="font-mono" title={t('tunnels.colo')}>
              {c}
            </Badge>
          ))}
        </div>

        <UptimeBar uptime={tunnel.uptime[range]} label={t(`tunnels.range.${range}`)} observedSince={tunnel.first_seen_at} />

        <dl className="grid gap-1 text-xs">
          <Timestamp label={t('tunnels.statusSince')} iso={tunnel.status_since} now={now} />
          {up ? (
            <Timestamp label={t('tunnels.connectedSince')} iso={tunnel.conns_active_at} now={now} />
          ) : (
            <Timestamp label={t('tunnels.disconnectedSince')} iso={tunnel.conns_inactive_at} now={now} />
          )}
          {versions.length > 0 && (
            <div className="flex justify-between gap-2">
              <dt className="text-muted-foreground">cloudflared</dt>
              <dd className="font-mono">{versions.join(', ')}</dd>
            </div>
          )}
          <div className="flex justify-between gap-2">
            <dt className="text-muted-foreground">{t('tunnels.id')}</dt>
            <dd className="truncate font-mono" title={tunnel.id}>
              {tunnel.id}
            </dd>
          </div>
        </dl>
      </CardContent>
    </Card>
  )
}

function Legend() {
  const { t } = useTranslation()
  return (
    <div className="text-muted-foreground flex flex-wrap gap-x-4 gap-y-1 text-xs">
      {(['healthy', 'degraded', 'down', 'inactive', ''] as const).map((s) => (
        <span key={s} className="flex items-center gap-1.5">
          <span className={cn('size-2.5 rounded-[2px]', bucketColor[s])} />
          {t(`tunnel.bucket.${bucketKey(s)}`)}
        </span>
      ))}
    </div>
  )
}

export function Tunnels() {
  const { t } = useTranslation()
  const tunnels = useTunnels()
  const refresh = useRefreshTunnels()
  const now = useNow()
  const [range, setRange] = useState<Range>('24h')

  const o = tunnels.data
  const code = <code className="font-mono" />

  return (
    <div className="flex flex-col gap-6">
      <div className="flex flex-wrap items-end justify-between gap-3">
        <div>
          <h1 className="text-2xl font-semibold tracking-tight">{t('nav.tunnels')}</h1>
          <p className="text-muted-foreground text-sm">
            {t('tunnels.subtitle')}
            {o?.last_poll && (
              <span title={absoluteTime(o.last_poll)}>
                {' · '}
                {t('tunnels.polled', { time: relativeTime(o.last_poll, now) })}
              </span>
            )}
          </p>
        </div>
        <div className="flex gap-2">
          <div className="bg-muted flex rounded-md p-0.5" role="group" aria-label={t('tunnels.period')}>
            {ranges.map((r) => (
              <button
                key={r}
                type="button"
                onClick={() => setRange(r)}
                aria-pressed={range === r}
                className={cn(
                  'rounded px-3 py-1 text-sm font-medium transition-colors',
                  range === r ? 'bg-background shadow-xs' : 'text-muted-foreground hover:text-foreground',
                )}
              >
                {t(`tunnels.range.${r}`)}
              </button>
            ))}
          </div>
          <Button
            variant="outline"
            onClick={() => refresh.mutate()}
            disabled={refresh.isPending || !o?.configured}
            aria-label={t('tunnels.pollNow')}
          >
            <RefreshCw className={cn(refresh.isPending && 'animate-spin')} />
            <span className="hidden sm:inline">{t('tunnels.pollNow')}</span>
          </Button>
        </div>
      </div>

      {o && !o.configured && (
        <div className="border-warning/40 bg-warning/10 flex gap-3 rounded-lg border p-4 text-sm">
          <AlertTriangle className="text-warning mt-0.5 size-4 shrink-0" />
          <div>
            <p className="font-medium">{t('tunnels.notConfigured')}</p>
            <p className="text-muted-foreground">
              <Trans i18nKey="tunnels.notConfiguredHint" components={{ code }} />
            </p>
          </div>
        </div>
      )}
      {o?.error && (
        <div className="border-destructive/40 bg-destructive/10 text-destructive rounded-lg border p-4 text-sm">
          {t('tunnels.lastPollFailed', { error: o.error })}
          {o.tunnels.length > 0 && <span className="text-muted-foreground"> {t('tunnels.showingLast')}</span>}
        </div>
      )}

      {tunnels.isPending ? (
        <div className="grid gap-4 md:grid-cols-2">
          <Skeleton className="h-64" />
          <Skeleton className="h-64" />
        </div>
      ) : tunnels.isError ? (
        <p className="text-destructive text-sm">{t('tunnels.loadError', { error: tunnels.error.message })}</p>
      ) : o && o.configured && o.tunnels.length === 0 ? (
        <Card className="items-center p-10 text-center">
          <Network className="text-muted-foreground size-8" />
          <p className="text-muted-foreground text-sm">{o.last_poll ? t('tunnels.empty') : t('tunnels.firstPoll')}</p>
        </Card>
      ) : (
        <>
          <div className="grid gap-4 md:grid-cols-2">
            {o?.tunnels.map((tn) => <TunnelCard key={tn.id} tunnel={tn} range={range} now={now} />)}
          </div>
          {o && o.tunnels.length > 0 && <Legend />}
        </>
      )}
    </div>
  )
}
