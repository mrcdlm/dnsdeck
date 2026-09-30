import { AlertTriangle, Network, RefreshCw } from 'lucide-react'
import { useState } from 'react'

import { UptimeBar } from '@/components/UptimeBar'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { Skeleton } from '@/components/ui/skeleton'
import type { Tunnel } from '@/lib/api'
import { absoluteTime, relativeTime, useNow } from '@/lib/format'
import { useRefreshTunnels, useTunnels } from '@/lib/queries'
import { bucketColor, bucketLabel, clientVersions, colos, tunnelStatus } from '@/lib/tunnels'
import { cn } from '@/lib/utils'

type Range = '24h' | '7d'
const rangeLabel: Record<Range, string> = { '24h': '24 Stunden', '7d': '7 Tage' }

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
  const st = tunnelStatus[tunnel.status]
  const up = tunnel.status === 'healthy' || tunnel.status === 'degraded'
  const versions = clientVersions(tunnel)

  return (
    <Card className="gap-4">
      <CardHeader>
        <div className="flex items-start justify-between gap-2">
          <CardTitle className="min-w-0 text-base [overflow-wrap:anywhere]">{tunnel.name}</CardTitle>
          <Badge variant={st.variant} title={st.hint}>
            {st.label}
          </Badge>
        </div>
      </CardHeader>
      <CardContent className="flex flex-col gap-4">
        <div className="flex flex-wrap items-center gap-2 text-sm">
          <span className="font-medium">
            {tunnel.connections.length} {tunnel.connections.length === 1 ? 'Verbindung' : 'Verbindungen'}
          </span>
          {colos(tunnel).map((c) => (
            <Badge key={c} variant="outline" className="font-mono" title="Cloudflare-Rechenzentrum">
              {c}
            </Badge>
          ))}
        </div>

        <UptimeBar uptime={tunnel.uptime[range]} label={rangeLabel[range]} observedSince={tunnel.first_seen_at} />

        <dl className="grid gap-1 text-xs">
          <Timestamp label="Status seit (beobachtet)" iso={tunnel.status_since} now={now} />
          {up ? (
            <Timestamp label="Verbunden seit" iso={tunnel.conns_active_at} now={now} />
          ) : (
            <Timestamp label="Getrennt seit" iso={tunnel.conns_inactive_at} now={now} />
          )}
          {versions.length > 0 && (
            <div className="flex justify-between gap-2">
              <dt className="text-muted-foreground">cloudflared</dt>
              <dd className="font-mono">{versions.join(', ')}</dd>
            </div>
          )}
          <div className="flex justify-between gap-2">
            <dt className="text-muted-foreground">Tunnel-ID</dt>
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
  return (
    <div className="text-muted-foreground flex flex-wrap gap-x-4 gap-y-1 text-xs">
      {(['healthy', 'degraded', 'down', 'inactive', ''] as const).map((s) => (
        <span key={s} className="flex items-center gap-1.5">
          <span className={cn('size-2.5 rounded-[2px]', bucketColor[s])} />
          {bucketLabel[s]}
        </span>
      ))}
    </div>
  )
}

export function Tunnels() {
  const tunnels = useTunnels()
  const refresh = useRefreshTunnels()
  const now = useNow()
  const [range, setRange] = useState<Range>('24h')

  const o = tunnels.data

  return (
    <div className="flex flex-col gap-6">
      <div className="flex flex-wrap items-end justify-between gap-3">
        <div>
          <h1 className="text-2xl font-semibold tracking-tight">Tunnels</h1>
          <p className="text-muted-foreground text-sm">
            Cloudflare Tunnels deines Accounts
            {o?.last_poll && (
              <span title={absoluteTime(o.last_poll)}> · abgefragt {relativeTime(o.last_poll, now)}</span>
            )}
          </p>
        </div>
        <div className="flex gap-2">
          <div className="bg-muted flex rounded-md p-0.5" role="group" aria-label="Zeitraum">
            {(['24h', '7d'] as const).map((r) => (
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
                {rangeLabel[r]}
              </button>
            ))}
          </div>
          <Button
            variant="outline"
            onClick={() => refresh.mutate()}
            disabled={refresh.isPending || !o?.configured}
            aria-label="Jetzt abfragen"
          >
            <RefreshCw className={cn(refresh.isPending && 'animate-spin')} />
            <span className="hidden sm:inline">Jetzt abfragen</span>
          </Button>
        </div>
      </div>

      {o && !o.configured && (
        <div className="border-warning/40 bg-warning/10 flex gap-3 rounded-lg border p-4 text-sm">
          <AlertTriangle className="text-warning mt-0.5 size-4 shrink-0" />
          <div>
            <p className="font-medium">Tunnel-Monitoring ist nicht konfiguriert</p>
            <p className="text-muted-foreground">
              Setze <code className="font-mono">CF_API_TOKEN</code> und <code className="font-mono">CF_ACCOUNT_ID</code> in{' '}
              <code className="font-mono">deploy/.env</code> und starte den Container neu. Das Token braucht zusätzlich
              das Recht Account → Cloudflare Tunnel → Read.
            </p>
          </div>
        </div>
      )}
      {o?.error && (
        <div className="border-destructive/40 bg-destructive/10 text-destructive rounded-lg border p-4 text-sm">
          Letzte Abfrage fehlgeschlagen: {o.error}
          {o.tunnels.length > 0 && <span className="text-muted-foreground"> – angezeigt wird der letzte bekannte Stand.</span>}
        </div>
      )}

      {tunnels.isPending ? (
        <div className="grid gap-4 md:grid-cols-2">
          <Skeleton className="h-64" />
          <Skeleton className="h-64" />
        </div>
      ) : tunnels.isError ? (
        <p className="text-destructive text-sm">Tunnels konnten nicht geladen werden: {tunnels.error.message}</p>
      ) : o && o.configured && o.tunnels.length === 0 ? (
        <Card className="items-center p-10 text-center">
          <Network className="text-muted-foreground size-8" />
          <p className="text-muted-foreground text-sm">
            {o.last_poll ? 'Keine Tunnels im Account gefunden.' : 'Erste Abfrage läuft …'}
          </p>
        </Card>
      ) : (
        <>
          <div className="grid gap-4 md:grid-cols-2">
            {o?.tunnels.map((t) => <TunnelCard key={t.id} tunnel={t} range={range} now={now} />)}
          </div>
          {o && o.tunnels.length > 0 && <Legend />}
        </>
      )}
    </div>
  )
}
