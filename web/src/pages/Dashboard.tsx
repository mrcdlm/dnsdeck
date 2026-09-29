import { AlertTriangle, ArrowLeft, ChevronRight, Globe, ListTree, Network, RefreshCw } from 'lucide-react'
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
import { useIP, useIPHistory, useRecords, useRefreshIP } from '@/lib/queries'
import { cn } from '@/lib/utils'

const familyLabel: Record<Family, string> = { ipv4: 'IPv4', ipv6: 'IPv6' }

function FamilyPanel({ family, state, now }: { family: Family; state: FamilyState; now: number }) {
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
          <CopyButton value={state.ip} label={`${label}-Adresse kopieren`} />
        </div>
      ) : (
        <span className="text-muted-foreground text-2xl font-semibold">—</span>
      )}

      <div className="text-muted-foreground flex flex-col gap-1 text-sm">
        {state.since && (
          <span title={absoluteTime(state.since)}>
            seit {absoluteTime(state.since)} ({relativeTime(state.since, now)})
          </span>
        )}
        {state.responses > 0 && (
          <span>
            {state.votes} von {state.responses} Quellen einig
          </span>
        )}
        {state.message && state.status !== 'ok' && (
          <span className={cn('flex items-center gap-1.5', state.status === 'unconfirmed' && 'text-warning')}>
            {state.status === 'unconfirmed' && <AlertTriangle className="size-3.5 shrink-0" />}
            {state.message}
          </span>
        )}
        {failed.length > 0 && state.status !== 'unavailable' && (
          <details className="text-xs">
            <summary className="cursor-pointer select-none">
              {failed.length} Quelle{failed.length > 1 ? 'n' : ''} nicht erreichbar
            </summary>
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
  const ip = useIP()
  const refresh = useRefreshIP()
  const now = useNow()

  return (
    <Card>
      <CardHeader>
        <CardTitle className="flex items-center gap-2 text-lg">
          <Globe className="size-5" />
          Öffentliche IP
        </CardTitle>
        <CardDescription>
          {ip.data?.last_checked ? (
            <span title={absoluteTime(ip.data.last_checked)}>
              Zuletzt geprüft {relativeTime(ip.data.last_checked, now)}
            </span>
          ) : (
            'Noch nicht geprüft'
          )}
        </CardDescription>
        <CardAction>
          <Button
            variant="outline"
            onClick={() => refresh.mutate()}
            disabled={refresh.isPending}
            aria-label="Jetzt aktualisieren"
            title="IP prüfen und alle Records abgleichen"
          >
            <RefreshCw className={cn(refresh.isPending && 'animate-spin')} />
            <span className="hidden sm:inline">Jetzt aktualisieren</span>
          </Button>
        </CardAction>
      </CardHeader>
      <CardContent className="flex flex-col gap-4">
        {refresh.isError && (
          <p className="text-destructive text-sm" role="alert">
            Aktualisierung fehlgeschlagen: {refresh.error.message}
          </p>
        )}
        {ip.isPending ? (
          <div className="grid gap-4 md:grid-cols-2">
            <Skeleton className="h-36" />
            <Skeleton className="h-36" />
          </div>
        ) : ip.isError ? (
          <p className="text-destructive text-sm">IP-Status konnte nicht geladen werden: {ip.error.message}</p>
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
  const history = useIPHistory(10)
  const now = useNow()

  return (
    <Card>
      <CardHeader>
        <CardTitle className="text-lg">Letzte IP-Wechsel</CardTitle>
        <CardDescription>Die zehn jüngsten Änderungen der öffentlichen Adressen</CardDescription>
      </CardHeader>
      <CardContent>
        {history.isPending ? (
          <div className="flex flex-col gap-2">
            <Skeleton className="h-10" />
            <Skeleton className="h-10" />
            <Skeleton className="h-10" />
          </div>
        ) : history.isError ? (
          <p className="text-destructive text-sm">Verlauf konnte nicht geladen werden: {history.error.message}</p>
        ) : history.data.length === 0 ? (
          <p className="text-muted-foreground text-sm">Noch keine IP-Wechsel protokolliert.</p>
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
        <div className="text-muted-foreground text-sm">DNS-Records</div>
        {records.isPending ? (
          <Skeleton className="mt-1 h-6 w-32" />
        ) : total === 0 ? (
          <div className="font-medium">Noch keine Records</div>
        ) : (
          <div className="flex flex-wrap items-center gap-2 font-medium">
            <span>{total} verwaltet</span>
            {counts.ok > 0 && <Badge variant="success">{counts.ok} aktuell</Badge>}
            {counts.error > 0 && <Badge variant="destructive">{counts.error} Fehler</Badge>}
            {counts.other > 0 && <Badge variant="secondary">{counts.other} sonstige</Badge>}
          </div>
        )}
      </div>
      <ChevronRight className="text-muted-foreground size-4 transition-transform group-hover:translate-x-0.5" />
    </Link>
  )
}

function TunnelsTile() {
  return (
    <div className="bg-card/50 text-muted-foreground flex items-center gap-4 rounded-xl border border-dashed p-5">
      <span className="bg-muted grid size-10 shrink-0 place-items-center rounded-lg">
        <Network className="size-5" />
      </span>
      <div>
        <div className="text-sm">Cloudflare Tunnels</div>
        <div className="text-sm">kommt in Meilenstein 3</div>
      </div>
    </div>
  )
}

export function Dashboard() {
  return (
    <div className="flex flex-col gap-6">
      <div>
        <h1 className="text-2xl font-semibold tracking-tight">Dashboard</h1>
        <p className="text-muted-foreground text-sm">Öffentliche Adressen, DNS-Records und letzte Änderungen</p>
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
            <CardTitle className="text-lg">Letzte DNS-Updates</CardTitle>
            <CardDescription>Änderungen an den verwalteten Records</CardDescription>
          </CardHeader>
          <CardContent>
            <UpdateLogList limit={5} />
          </CardContent>
        </Card>
      </div>
    </div>
  )
}
