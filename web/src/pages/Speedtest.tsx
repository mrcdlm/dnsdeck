import { ArrowDown, ArrowUp, CalendarClock, Gauge, Loader2, MapPin, Network, Server, Waves } from 'lucide-react'
import { type ReactNode } from 'react'
import { useTranslation } from 'react-i18next'
import { Link } from 'react-router'
import {
  CartesianGrid,
  Line,
  LineChart,
  ResponsiveContainer,
  Tooltip,
  XAxis,
  YAxis,
  type TooltipContentProps,
} from 'recharts'
import type { NameType, ValueType } from 'recharts/types/component/DefaultTooltipContent'

import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Card, CardAction, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card'
import { Skeleton } from '@/components/ui/skeleton'
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from '@/components/ui/table'
import type { SpeedtestResult } from '@/lib/api'
import { absoluteTime, relativeTime, useNow } from '@/lib/format'
import { useRunSpeedtest, useSpeedtest } from '@/lib/queries'
import { formatBytes, formatInterval, formatMbps, formatMs, place, succeeded } from '@/lib/speedtest'
import { cn } from '@/lib/utils'

const LIMIT = 100

function Stat({ icon, label, value, unit }: { icon: ReactNode; label: string; value?: string; unit: string }) {
  return (
    <div className="flex min-w-0 flex-col gap-1 rounded-lg border p-4">
      <span className="text-muted-foreground flex items-center gap-1.5 text-sm">
        {icon}
        {label}
      </span>
      <span className="flex items-baseline gap-1.5">
        <span className="text-3xl font-semibold tracking-tight tabular-nums">{value ?? '—'}</span>
        <span className="text-muted-foreground text-sm">{unit}</span>
      </span>
    </div>
  )
}

/** Sekunden seit Beginn der laufenden Messung. */
function Elapsed({ since }: { since?: string }) {
  const now = useNow(1000)
  if (!since) return null
  return <span className="tabular-nums">{Math.max(0, Math.round((now - new Date(since).getTime()) / 1000))} s</span>
}

function LatestCard({ result, running, since }: { result?: SpeedtestResult; running: boolean; since?: string }) {
  const { t } = useTranslation()
  const now = useNow()
  const ok = result && succeeded(result)
  const where = result && place(result)

  return (
    <Card>
      <CardHeader>
        <CardTitle className="text-lg">{t('speedtest.latest')}</CardTitle>
        {result && (
          <CardDescription title={absoluteTime(result.started_at)}>
            {relativeTime(result.started_at, now)} · {t(`speedtest.trigger.${result.trigger}`)}
          </CardDescription>
        )}
      </CardHeader>
      <CardContent className="flex flex-col gap-4">
        {running && (
          <div className="bg-muted/50 flex items-center gap-3 rounded-lg border p-3 text-sm" role="status">
            <Loader2 className="size-4 shrink-0 animate-spin" aria-hidden />
            <span className="font-medium">{t('speedtest.running')}</span>
            <Elapsed since={since} />
            <span className="text-muted-foreground hidden sm:inline">{t('speedtest.runningHint')}</span>
          </div>
        )}
        {!result ? (
          !running && <p className="text-muted-foreground py-6 text-center text-sm">{t('speedtest.empty')}</p>
        ) : (
          <>
            {!ok && (
              <p className="text-destructive text-sm" role="alert">
                {t('speedtest.failed')}
                {result.error && `: ${result.error}`}
              </p>
            )}
            <div className={cn('grid grid-cols-2 gap-3 lg:grid-cols-4', !ok && 'opacity-50')}>
              <Stat
                icon={<ArrowDown className="text-chart-download size-4" aria-hidden />}
                label={t('speedtest.download')}
                value={result.download_mbps !== undefined ? formatMbps(result.download_mbps) : undefined}
                unit={t('speedtest.mbps')}
              />
              <Stat
                icon={<ArrowUp className="text-chart-upload size-4" aria-hidden />}
                label={t('speedtest.upload')}
                value={result.upload_mbps !== undefined ? formatMbps(result.upload_mbps) : undefined}
                unit={t('speedtest.mbps')}
              />
              <Stat
                icon={<Gauge className="text-chart-latency size-4" aria-hidden />}
                label={t('speedtest.latency')}
                value={result.latency_ms !== undefined ? formatMs(result.latency_ms) : undefined}
                unit={t('speedtest.ms')}
              />
              <Stat
                icon={<Waves className="text-muted-foreground size-4" aria-hidden />}
                label={t('speedtest.jitter')}
                value={result.jitter_ms !== undefined ? formatMs(result.jitter_ms) : undefined}
                unit={t('speedtest.ms')}
              />
            </div>
            <div className="text-muted-foreground flex flex-col gap-1 text-sm sm:flex-row sm:flex-wrap sm:gap-x-5">
              {where && (
                <span className="flex items-center gap-1.5">
                  <MapPin className="size-4 shrink-0" aria-hidden />
                  {t('speedtest.location', { place: where })}
                </span>
              )}
              {result.colo && (
                <span className="flex items-center gap-1.5" title={t('speedtest.serverHint')}>
                  <Server className="size-4 shrink-0" aria-hidden />
                  {t('speedtest.server', { colo: result.colo })}
                </span>
              )}
              {result.ip && (
                <span className="flex items-center gap-1.5">
                  <Network className="size-4 shrink-0" aria-hidden />
                  <span className="font-mono text-xs whitespace-nowrap">{t('speedtest.via', { ip: result.ip })}</span>
                </span>
              )}
            </div>
          </>
        )}
      </CardContent>
    </Card>
  )
}

interface Point {
  t: number
  download: number | null
  upload: number | null
  latency: number | null
}

const seriesLabel = {
  download: 'speedtest.download',
  upload: 'speedtest.upload',
  latency: 'speedtest.latency',
} as const

const axisTick = { fill: 'var(--muted-foreground)', fontSize: 12 }

function useTimeTick() {
  const { i18n } = useTranslation()
  const fmt = new Intl.DateTimeFormat(i18n.language === 'de' ? 'de-DE' : 'en-US', {
    day: '2-digit',
    month: '2-digit',
    hour: '2-digit',
    minute: '2-digit',
  })
  return (v: number) => fmt.format(new Date(v))
}

function ChartTooltip({ active, payload, label }: TooltipContentProps<ValueType, NameType>) {
  const { t } = useTranslation()
  if (!active || !payload?.length) return null
  return (
    <div className="bg-popover text-popover-foreground rounded-md border px-3 py-2 text-xs shadow-md">
      <div className="text-muted-foreground mb-1">{absoluteTime(new Date(Number(label)).toISOString())}</div>
      {payload.map((p) => (
        <div key={String(p.dataKey)} className="flex items-center gap-2">
          <span className="h-0.5 w-3 rounded-full" style={{ background: p.color }} aria-hidden />
          <span>{t(seriesLabel[p.dataKey as keyof typeof seriesLabel])}</span>
          <span className="ml-auto pl-3 font-medium tabular-nums">
            {p.value === null || p.value === undefined
              ? '—'
              : p.dataKey === 'latency'
                ? `${formatMs(Number(p.value))} ${t('speedtest.ms')}`
                : `${formatMbps(Number(p.value))} ${t('speedtest.mbps')}`}
          </span>
        </div>
      ))}
    </div>
  )
}

/** Beschriftung am letzten Punkt einer Linie (Kennung nicht nur über Farbe). */
function EndLabel({ index, x, y, last, text }: { index?: number; x?: number; y?: number; last: number; text: string }) {
  if (index !== last || x === undefined || y === undefined) return null
  return (
    <text x={x + 8} y={y} dy={4} fontSize={12} fill="var(--muted-foreground)">
      {text}
    </text>
  )
}

function lastIndex(points: Point[], key: keyof Point): number {
  for (let i = points.length - 1; i >= 0; i--) if (points[i][key] !== null) return i
  return -1
}

function HistoryCharts({ results }: { results: SpeedtestResult[] }) {
  const { t } = useTranslation()
  const tick = useTimeTick()
  // älteste zuerst; fehlgeschlagene Messungen erscheinen als Lücke
  const points: Point[] = [...results].reverse().map((r) => ({
    t: new Date(r.started_at).getTime(),
    download: r.download_mbps ?? null,
    upload: r.upload_mbps ?? null,
    latency: r.latency_ms ?? null,
  }))
  const okCount = results.filter(succeeded).length
  const showDots = points.length <= 30

  if (okCount < 2) {
    return <p className="text-muted-foreground py-6 text-center text-sm">{t('speedtest.chartTooFew')}</p>
  }

  const xAxis = (
    <XAxis
      dataKey="t"
      type="number"
      scale="time"
      domain={['dataMin', 'dataMax']}
      tickFormatter={tick}
      tick={axisTick}
      tickLine={false}
      axisLine={{ stroke: 'var(--border)' }}
      minTickGap={40}
    />
  )
  const grid = <CartesianGrid vertical={false} stroke="var(--border)" />
  const cursor = { stroke: 'var(--muted-foreground)', strokeDasharray: '3 3' }
  const dot = showDots ? { r: 3, strokeWidth: 0 } : false
  const activeDot = { r: 5, strokeWidth: 2, stroke: 'var(--card)' }

  return (
    <div className="flex flex-col gap-6">
      <div className="flex flex-col gap-2">
        <div className="flex flex-wrap items-center gap-x-4 gap-y-1 text-sm">
          <span className="font-medium">{t('speedtest.throughput')}</span>
          <span className="text-muted-foreground">({t('speedtest.mbps')})</span>
          <span className="ml-auto flex items-center gap-4">
            <span className="flex items-center gap-1.5">
              <span className="bg-chart-download h-0.5 w-4 rounded-full" aria-hidden />
              {t('speedtest.download')}
            </span>
            <span className="flex items-center gap-1.5">
              <span className="bg-chart-upload h-0.5 w-4 rounded-full" aria-hidden />
              {t('speedtest.upload')}
            </span>
          </span>
        </div>
        <div className="h-64" role="img" aria-label={t('speedtest.chartLabel')}>
          <ResponsiveContainer width="100%" height="100%">
            <LineChart data={points} margin={{ top: 8, right: 72, bottom: 0, left: 0 }}>
              {grid}
              {xAxis}
              <YAxis
                tick={axisTick}
                tickLine={false}
                axisLine={false}
                width={44}
                tickFormatter={(v: number) => formatMbps(v)}
              />
              <Tooltip content={ChartTooltip} cursor={cursor} />
              <Line
                type="monotone"
                dataKey="download"
                stroke="var(--chart-download)"
                fill="var(--chart-download)"
                strokeWidth={2}
                dot={dot}
                activeDot={activeDot}
                connectNulls={false}
                isAnimationActive={false}
                label={<EndLabel last={lastIndex(points, 'download')} text={t('speedtest.download')} />}
              />
              <Line
                type="monotone"
                dataKey="upload"
                stroke="var(--chart-upload)"
                fill="var(--chart-upload)"
                strokeWidth={2}
                dot={dot}
                activeDot={activeDot}
                connectNulls={false}
                isAnimationActive={false}
                label={<EndLabel last={lastIndex(points, 'upload')} text={t('speedtest.upload')} />}
              />
            </LineChart>
          </ResponsiveContainer>
        </div>
      </div>

      <div className="flex flex-col gap-2">
        <div className="flex items-center gap-2 text-sm">
          <span className="font-medium">{t('speedtest.latencyChart')}</span>
          <span className="text-muted-foreground">({t('speedtest.ms')})</span>
        </div>
        <div className="h-36" role="img" aria-label={t('speedtest.latencyChartLabel')}>
          <ResponsiveContainer width="100%" height="100%">
            <LineChart data={points} margin={{ top: 8, right: 72, bottom: 0, left: 0 }}>
              {grid}
              {xAxis}
              <YAxis
                tick={axisTick}
                tickLine={false}
                axisLine={false}
                width={44}
                tickFormatter={(v: number) => formatMs(v)}
              />
              <Tooltip content={ChartTooltip} cursor={cursor} />
              <Line
                type="monotone"
                dataKey="latency"
                stroke="var(--chart-latency)"
                fill="var(--chart-latency)"
                strokeWidth={2}
                dot={dot}
                activeDot={activeDot}
                connectNulls={false}
                isAnimationActive={false}
              />
            </LineChart>
          </ResponsiveContainer>
        </div>
      </div>
    </div>
  )
}

function ResultsTable({ results }: { results: SpeedtestResult[] }) {
  const { t } = useTranslation()
  const now = useNow()
  const num = (v: number | undefined, f: (n: number) => string) => (v === undefined ? '—' : f(v))
  return (
    <div className="overflow-x-auto">
      <Table>
        <TableHeader>
          <TableRow className="hover:bg-transparent">
            <TableHead>{t('speedtest.time')}</TableHead>
            <TableHead className="text-right">
              {t('speedtest.download')}
              <span className="block font-normal normal-case sm:hidden">Mbit/s</span>
            </TableHead>
            <TableHead className="text-right">
              {t('speedtest.upload')}
              <span className="block font-normal normal-case sm:hidden">Mbit/s</span>
            </TableHead>
            <TableHead className="text-right">
              {t('speedtest.latency')}
              <span className="block font-normal normal-case sm:hidden">ms</span>
            </TableHead>
            <TableHead className="hidden text-right sm:table-cell">{t('speedtest.jitter')}</TableHead>
            <TableHead className="hidden md:table-cell">{t('speedtest.serverCol')}</TableHead>
            <TableHead className="hidden text-right lg:table-cell">{t('speedtest.data')}</TableHead>
          </TableRow>
        </TableHeader>
        <TableBody>
          {results.map((r) => (
            <TableRow key={r.id}>
              <TableCell className="whitespace-nowrap">
                <span title={absoluteTime(r.started_at)}>{relativeTime(r.started_at, now)}</span>
                <Badge variant="secondary" className="ml-2 hidden align-middle sm:inline-flex">
                  {t(`speedtest.trigger.${r.trigger}`)}
                </Badge>
              </TableCell>
              {succeeded(r) ? (
                <>
                  <TableCell className="text-right tabular-nums">
                    {num(r.download_mbps, formatMbps)}{' '}
                    <span className="text-muted-foreground hidden text-xs sm:inline">Mbit/s</span>
                  </TableCell>
                  <TableCell className="text-right tabular-nums">
                    {num(r.upload_mbps, formatMbps)}{' '}
                    <span className="text-muted-foreground hidden text-xs sm:inline">Mbit/s</span>
                  </TableCell>
                  <TableCell className="text-right tabular-nums">
                    {num(r.latency_ms, formatMs)}{' '}
                    <span className="text-muted-foreground hidden text-xs sm:inline">ms</span>
                  </TableCell>
                  <TableCell className="hidden text-right tabular-nums sm:table-cell">
                    {num(r.jitter_ms, formatMs)}{' '}
                    <span className="text-muted-foreground hidden text-xs sm:inline">ms</span>
                  </TableCell>
                </>
              ) : (
                <TableCell colSpan={4} className="text-destructive max-w-[20rem] text-xs [overflow-wrap:anywhere]">
                  {r.error || t('speedtest.failed')}
                </TableCell>
              )}
              <TableCell
                className="text-muted-foreground hidden text-xs md:table-cell"
                title={t('speedtest.serverHint')}
              >
                {r.colo ?? '—'}
              </TableCell>
              <TableCell className="text-muted-foreground hidden text-right text-xs whitespace-nowrap lg:table-cell">
                {r.download_bytes + r.upload_bytes > 0 ? formatBytes(r.download_bytes + r.upload_bytes) : '—'}
              </TableCell>
            </TableRow>
          ))}
        </TableBody>
      </Table>
    </div>
  )
}

export function Speedtest() {
  const { t } = useTranslation()
  const data = useSpeedtest(LIMIT)
  const run = useRunSpeedtest()
  const o = data.data
  const running = (o?.running ?? false) || run.isPending
  const error = run.isError ? run.error.message : undefined

  return (
    <div className="flex flex-col gap-6">
      <div className="flex flex-wrap items-end justify-between gap-3">
        <div>
          <h1 className="text-2xl font-semibold tracking-tight">{t('nav.speedtest')}</h1>
          <p className="text-muted-foreground text-sm">{t('speedtest.subtitle')}</p>
        </div>
        <Button onClick={() => run.mutate()} disabled={running || !o?.available}>
          {running ? <Loader2 className="animate-spin" /> : <Gauge />}
          {running ? t('speedtest.running') : t('speedtest.run')}
        </Button>
      </div>
      {error && (
        <p className="text-destructive text-sm" role="alert">
          {error}
        </p>
      )}

      {data.isPending ? (
        <Skeleton className="h-56" />
      ) : data.isError ? (
        <p className="text-destructive text-sm">{t('speedtest.loadError', { error: data.error.message })}</p>
      ) : (
        <>
          {!o!.available && <p className="text-muted-foreground text-sm">{t('speedtest.unavailable')}</p>}
          <LatestCard result={o!.results[0]} running={running} since={o!.running_since} />

          <Card>
            <CardHeader>
              <CardTitle className="text-lg">{t('speedtest.history')}</CardTitle>
              <CardDescription>{t('speedtest.historyHint', { count: LIMIT })}</CardDescription>
              <CardAction>
                <span className="text-muted-foreground flex items-center gap-1.5 text-xs">
                  <CalendarClock className="size-4" aria-hidden />
                  {o!.interval_seconds > 0
                    ? t('speedtest.schedule', {
                        interval: formatInterval(t, o!.interval_seconds),
                      })
                    : t('speedtest.scheduleOff')}
                  <Link to="/einstellungen" className="text-foreground underline-offset-2 hover:underline">
                    {t('speedtest.scheduleChange')}
                  </Link>
                </span>
              </CardAction>
            </CardHeader>
            <CardContent>
              <HistoryCharts results={o!.results} />
            </CardContent>
          </Card>

          {o!.results.length > 0 && (
            <Card className="gap-0 pb-0">
              <CardHeader className="pb-4">
                <CardTitle className="text-lg">{t('speedtest.table')}</CardTitle>
              </CardHeader>
              <ResultsTable results={o!.results} />
            </Card>
          )}
        </>
      )}
    </div>
  )
}
