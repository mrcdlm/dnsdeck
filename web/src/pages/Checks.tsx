import { Info, Link2, Loader2, Lock, LockOpen, Pencil, Plus, RefreshCw, ShieldAlert, Trash2 } from 'lucide-react'
import { useState, type FormEvent } from 'react'
import { Trans, useTranslation } from 'react-i18next'

import { ProbeDialog } from '@/components/ProbeDialog'
import { UptimeBar } from '@/components/UptimeBar'
import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
} from '@/components/ui/alert-dialog'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Card } from '@/components/ui/card'
import { Input } from '@/components/ui/input'
import { Skeleton } from '@/components/ui/skeleton'
import { Switch } from '@/components/ui/switch'
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from '@/components/ui/table'
import type { Probe } from '@/lib/api'
import { absoluteTime, relativeTime, useNow } from '@/lib/format'
import { certDaysLeft, probeStatusVariant } from '@/lib/probes'
import { useDeleteProbe, useProbes, useRunProbe, useSaveProbe, useSettings } from '@/lib/queries'
import { cn } from '@/lib/utils'

type Range = '24h' | '7d'
const ranges: Range[] = ['24h', '7d']

interface RowProps {
  probe: Probe
  now: number
  warnDays: number
  range: Range
  onEdit: () => void
  onDelete: () => void
}

function History({ probe, range }: { probe: Probe; range: Range }) {
  const { t } = useTranslation()
  const uptime = probe.uptime?.[range]
  if (!uptime) return <span className="text-muted-foreground text-xs">—</span>
  return (
    <UptimeBar
      uptime={uptime}
      label={t(`tunnels.range.${range}`)}
      observedSince={probe.created_at}
      bucketTexts="checks.bucket"
      compact
    />
  )
}

function Address({ probe }: { probe: Probe }) {
  const { t } = useTranslation()
  return (
    <div className="min-w-0">
      <a
        href={probe.url}
        target="_blank"
        rel="noreferrer"
        className="font-medium [overflow-wrap:anywhere] hover:underline"
      >
        {probe.url}
      </a>
      {probe.record_name && (
        <div className="text-muted-foreground mt-0.5 flex items-center gap-1 text-xs">
          <Link2 className="size-3" aria-hidden />
          {t('checks.fromRecord')}
        </div>
      )}
      {probe.expected_status && (
        <div className="text-muted-foreground mt-0.5 text-xs">
          {t('checks.expectedShort', { codes: probe.expected_status })}
        </div>
      )}
    </div>
  )
}

function Status({ probe }: { probe: Probe }) {
  const { t } = useTranslation()
  return (
    <>
      <Badge variant={probeStatusVariant[probe.status]}>{t(`checks.status.${probe.status}`)}</Badge>
      {probe.message && (
        <p
          className={cn(
            'mt-1 text-xs [overflow-wrap:anywhere]',
            probe.status === 'down' || probe.status === 'tls_error' ? 'text-destructive' : 'text-muted-foreground',
            probe.fail_count > 0 && probe.status !== 'down' && 'text-warning',
          )}
        >
          {probe.message}
        </p>
      )}
    </>
  )
}

function Response({ probe }: { probe: Probe }) {
  if (probe.http_status === undefined) return <span className="text-muted-foreground">—</span>
  return (
    <span className="font-mono text-xs whitespace-nowrap">
      {probe.http_status}
      {probe.latency_ms !== undefined && <span className="text-muted-foreground"> · {probe.latency_ms} ms</span>}
    </span>
  )
}

function Certificate({ probe, now, warnDays }: Pick<RowProps, 'probe' | 'now' | 'warnDays'>) {
  const { t } = useTranslation()
  const days = certDaysLeft(probe, now)
  if (days === undefined || !probe.tls_not_after) {
    return (
      <span className="text-muted-foreground text-xs">{probe.url.startsWith('http://') ? t('checks.noTLS') : '—'}</span>
    )
  }
  const Icon = probe.tls_valid === false ? ShieldAlert : days <= warnDays ? LockOpen : Lock
  const color =
    probe.tls_valid === false || days < 0 ? 'text-destructive' : days <= warnDays ? 'text-warning' : 'text-success'
  return (
    <div className="text-xs" title={absoluteTime(probe.tls_not_after)}>
      <span className={cn('flex items-center gap-1 font-medium whitespace-nowrap', color)}>
        <Icon className="size-3.5 shrink-0" aria-hidden />
        {days < 0 ? t('checks.certExpired') : t('checks.certDays', { count: days })}
      </span>
      {probe.tls_issuer && <span className="text-muted-foreground break-words">{probe.tls_issuer}</span>}
    </div>
  )
}

function Actions({ probe, onEdit, onDelete }: { probe: Probe; onEdit: () => void; onDelete: () => void }) {
  const { t } = useTranslation()
  const run = useRunProbe()
  const save = useSaveProbe()
  return (
    <div className="flex items-center justify-end gap-1">
      <Switch
        checked={probe.enabled}
        aria-label={t('checks.enabledLabel', { url: probe.url })}
        title={probe.enabled ? t('checks.disable') : t('checks.enable')}
        disabled={save.isPending}
        onCheckedChange={(enabled) => save.mutate({ id: probe.id, input: { enabled } })}
      />
      <Button
        variant="ghost"
        size="icon"
        aria-label={t('checks.runLabel', { url: probe.url })}
        title={t('checks.run')}
        onClick={() => run.mutate(probe.id)}
        disabled={run.isPending || !probe.enabled}
      >
        <RefreshCw className={cn(run.isPending && 'animate-spin')} />
      </Button>
      <Button
        variant="ghost"
        size="icon"
        aria-label={t('checks.editLabel', { url: probe.url })}
        title={t('common.edit')}
        onClick={onEdit}
      >
        <Pencil />
      </Button>
      <Button
        variant="ghost"
        size="icon"
        aria-label={t('checks.removeLabel', { url: probe.url })}
        title={t('common.remove')}
        onClick={onDelete}
      >
        <Trash2 />
      </Button>
    </div>
  )
}

function Checked({ probe, now }: { probe: Probe; now: number }) {
  return probe.last_checked_at ? (
    <span title={absoluteTime(probe.last_checked_at)}>{relativeTime(probe.last_checked_at, now)}</span>
  ) : (
    <>—</>
  )
}

function ProbeRow({ probe, now, warnDays, range, onEdit, onDelete }: RowProps) {
  return (
    <TableRow className={cn(!probe.enabled && 'opacity-60')}>
      <TableCell className="max-w-[18rem] min-w-44">
        <Address probe={probe} />
      </TableCell>
      <TableCell className="max-w-[14rem]">
        <Status probe={probe} />
      </TableCell>
      <TableCell>
        <Response probe={probe} />
      </TableCell>
      <TableCell className="max-w-[12rem]">
        <Certificate probe={probe} now={now} warnDays={warnDays} />
      </TableCell>
      <TableCell className="w-40 min-w-36">
        <History probe={probe} range={range} />
      </TableCell>
      <TableCell className="text-muted-foreground hidden text-xs whitespace-nowrap xl:table-cell">
        <Checked probe={probe} now={now} />
      </TableCell>
      <TableCell>
        <Actions probe={probe} onEdit={onEdit} onDelete={onDelete} />
      </TableCell>
    </TableRow>
  )
}

/** Mobile Darstellung: eine Karte je Prüfung. */
function ProbeCard({ probe, now, warnDays, range, onEdit, onDelete }: RowProps) {
  const { t } = useTranslation()
  return (
    <li className={cn('flex flex-col gap-2 p-4', !probe.enabled && 'opacity-60')} data-probe={probe.url}>
      <div className="flex items-start justify-between gap-2">
        <Address probe={probe} />
        <Actions probe={probe} onEdit={onEdit} onDelete={onDelete} />
      </div>
      <div className="flex flex-wrap items-start gap-x-4 gap-y-2">
        <div>
          <Status probe={probe} />
        </div>
        <Response probe={probe} />
        <Certificate probe={probe} now={now} warnDays={warnDays} />
      </div>
      <History probe={probe} range={range} />
      {probe.last_checked_at && (
        <span className="text-muted-foreground text-xs">
          {t('records.checkedAgo', { time: relativeTime(probe.last_checked_at, now) })}
        </span>
      )}
    </li>
  )
}

function AddForm() {
  const { t } = useTranslation()
  const save = useSaveProbe()
  const [url, setURL] = useState('')
  const [expected, setExpected] = useState('')

  function submit(e: FormEvent) {
    e.preventDefault()
    save.mutate(
      { input: { url, ...(expected.trim() ? { expected_status: expected.trim() } : {}) } },
      { onSuccess: () => (setURL(''), setExpected('')) },
    )
  }

  return (
    <form onSubmit={submit} className="flex flex-col gap-2">
      <div className="flex flex-col gap-2 sm:flex-row">
        <Input
          aria-label={t('checks.url')}
          placeholder="https://nas.example.com"
          value={url}
          onChange={(e) => {
            setURL(e.target.value)
            save.reset()
          }}
          autoComplete="off"
          spellCheck={false}
          inputMode="url"
        />
        <Input
          aria-label={t('checks.expected')}
          title={t('checks.expectedHint')}
          placeholder={t('checks.expectedPlaceholder')}
          value={expected}
          onChange={(e) => {
            setExpected(e.target.value)
            save.reset()
          }}
          autoComplete="off"
          spellCheck={false}
          className="sm:w-56 sm:shrink-0"
        />
        <Button type="submit" disabled={!url.trim() || save.isPending} className="shrink-0">
          {save.isPending ? <Loader2 className="animate-spin" /> : <Plus />}
          {t('checks.add')}
        </Button>
      </div>
      {save.isError && (
        <p className="text-destructive text-sm" role="alert">
          {save.error.message}
        </p>
      )}
    </form>
  )
}

export function Checks() {
  const { t } = useTranslation()
  const probes = useProbes()
  const settings = useSettings()
  const del = useDeleteProbe()
  const now = useNow()
  const [toDelete, setToDelete] = useState<Probe>()
  const [editing, setEditing] = useState<Probe>()
  const [range, setRange] = useState<Range>('24h')
  const warnDays = settings.data?.tls_warn_days ?? 14
  const rowProps = (p: Probe) => ({
    probe: p,
    now,
    warnDays,
    range,
    onEdit: () => setEditing(p),
    onDelete: () => setToDelete(p),
  })

  return (
    <div className="flex flex-col gap-6">
      <div className="flex flex-wrap items-end justify-between gap-3">
        <div>
          <h1 className="text-2xl font-semibold tracking-tight">{t('nav.checks')}</h1>
          <p className="text-muted-foreground text-sm">{t('checks.subtitle')}</p>
        </div>
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
      </div>

      <AddForm />

      <Card className="py-0">
        {probes.isPending ? (
          <div className="flex flex-col gap-2 p-6">
            <Skeleton className="h-10" />
            <Skeleton className="h-10" />
          </div>
        ) : probes.isError ? (
          <p className="text-destructive p-6 text-sm">{t('checks.loadError', { error: probes.error.message })}</p>
        ) : probes.data.length === 0 ? (
          <p className="text-muted-foreground p-10 text-center text-sm">{t('checks.empty')}</p>
        ) : (
          <>
            <div className="hidden md:block">
              <Table>
                <TableHeader>
                  <TableRow className="hover:bg-transparent">
                    <TableHead>{t('checks.col.url')}</TableHead>
                    <TableHead>{t('checks.col.status')}</TableHead>
                    <TableHead>{t('checks.col.response')}</TableHead>
                    <TableHead>{t('checks.col.certificate')}</TableHead>
                    <TableHead>{t('checks.col.history', { range: t(`tunnels.range.${range}`) })}</TableHead>
                    <TableHead className="hidden xl:table-cell">{t('records.col.checked')}</TableHead>
                    <TableHead className="text-right">
                      <span className="sr-only">{t('records.col.actions')}</span>
                    </TableHead>
                  </TableRow>
                </TableHeader>
                <TableBody>
                  {probes.data.map((p) => (
                    <ProbeRow key={p.id} {...rowProps(p)} />
                  ))}
                </TableBody>
              </Table>
            </div>
            <ul className="divide-y md:hidden">
              {probes.data.map((p) => (
                <ProbeCard key={p.id} {...rowProps(p)} />
              ))}
            </ul>
          </>
        )}
      </Card>

      <div className="text-muted-foreground flex gap-3 text-sm">
        <Info className="mt-0.5 size-4 shrink-0" aria-hidden />
        <div className="flex flex-col gap-1">
          <p>{t('checks.howItWorks', { days: warnDays })}</p>
          <p>{t('checks.hairpin')}</p>
        </div>
      </div>

      <AlertDialog open={toDelete !== undefined} onOpenChange={(open) => !open && setToDelete(undefined)}>
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>{t('checks.removeTitle')}</AlertDialogTitle>
            <AlertDialogDescription>
              <Trans
                i18nKey={toDelete?.record_name ? 'checks.removeRecordHint' : 'checks.removeHint'}
                values={{ url: toDelete?.url }}
                components={{ url: <span className="text-foreground font-mono break-all" /> }}
              />
            </AlertDialogDescription>
          </AlertDialogHeader>
          {del.isError && <p className="text-destructive text-sm">{del.error.message}</p>}
          <AlertDialogFooter>
            <AlertDialogCancel>{t('common.cancel')}</AlertDialogCancel>
            <AlertDialogAction
              onClick={(e) => {
                e.preventDefault()
                if (toDelete) del.mutate(toDelete.id, { onSuccess: () => setToDelete(undefined) })
              }}
              disabled={del.isPending}
            >
              {t('common.remove')}
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>

      <ProbeDialog probe={editing} onClose={() => setEditing(undefined)} />
    </div>
  )
}
