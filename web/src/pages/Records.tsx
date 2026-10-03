import { Activity, AlertTriangle, Cloud, CloudOff, Pencil, Plus, RefreshCw, Trash2 } from 'lucide-react'
import { useState } from 'react'
import { Trans, useTranslation } from 'react-i18next'
import { Link } from 'react-router'

import { PropagationBadge } from '@/components/Propagation'
import { RecordDialog } from '@/components/RecordDialog'
import { UpdateLogList } from '@/components/UpdateLogList'
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
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card'
import { Skeleton } from '@/components/ui/skeleton'
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from '@/components/ui/table'
import { ApiError, type DnsRecord, type Probe } from '@/lib/api'
import { absoluteTime, relativeTime, useNow } from '@/lib/format'
import { probesByRecord } from '@/lib/probes'
import { useDeleteRecord, useProbes, useRecords, useRefreshIP, useSyncRecord, useZones } from '@/lib/queries'
import { formatTTL, recordStatusVariant } from '@/lib/records'
import { cn } from '@/lib/utils'

interface RowProps {
  record: DnsRecord
  probe?: Probe
  now: number
  onEdit: () => void
  onDelete: () => void
}

function RecordActions({ record, onEdit, onDelete }: Omit<RowProps, 'now' | 'probe'>) {
  const { t } = useTranslation()
  const sync = useSyncRecord()
  const name = `${record.name} ${record.type}`
  return (
    <div className="flex justify-end gap-1">
      <Button
        variant="ghost"
        size="icon"
        aria-label={t('records.syncLabel', { name })}
        title={t('records.sync')}
        onClick={() => sync.mutate(record.id)}
        disabled={sync.isPending}
      >
        <RefreshCw className={cn(sync.isPending && 'animate-spin')} />
      </Button>
      <Button variant="ghost" size="icon" aria-label={t('records.editLabel', { name })} title={t('common.edit')} onClick={onEdit}>
        <Pencil />
      </Button>
      <Button
        variant="ghost"
        size="icon"
        aria-label={t('records.removeLabel', { name })}
        title={t('common.remove')}
        onClick={onDelete}
      >
        <Trash2 />
      </Button>
    </div>
  )
}

function RecordStatus({ record }: { record: DnsRecord }) {
  const { t } = useTranslation()
  return (
    <>
      <Badge variant={recordStatusVariant[record.status]}>{t(`record.status.${record.status}`)}</Badge>
      {record.settings_pending && <p className="text-warning mt-1 text-xs">{t('records.settingsPending')}</p>}
      {record.message && record.status !== 'ok' && (
        <p
          className={cn(
            'mt-1 text-xs [overflow-wrap:anywhere]',
            record.status === 'error' ? 'text-destructive' : 'text-muted-foreground',
          )}
        >
          {record.message}
        </p>
      )}
    </>
  )
}

function ProxyIcon({ proxied }: { proxied: boolean }) {
  const { t } = useTranslation()
  return proxied ? (
    <Cloud className="size-4 text-orange-500 dark:text-orange-400" aria-label={t('records.proxied')} />
  ) : (
    <CloudOff className="text-muted-foreground size-4" aria-label={t('records.dnsOnly')} />
  )
}

const probeColor: Record<Probe['status'], string> = {
  up: 'text-success',
  expiring: 'text-warning',
  tls_error: 'text-destructive',
  down: 'text-destructive',
  pending: 'text-muted-foreground',
  paused: 'text-muted-foreground',
}

/** Symbol für eine aktive Erreichbarkeitsprüfung, verlinkt auf die Übersicht. */
function ProbeIndicator({ probe }: { probe?: Probe }) {
  const { t } = useTranslation()
  if (!probe) return null
  const label = t('records.probeLabel', { status: t(`checks.status.${probe.status}`) })
  return (
    <Link to="/erreichbarkeit" title={label} aria-label={label} className="shrink-0">
      <Activity className={cn('size-3.5', probeColor[probe.status])} />
    </Link>
  )
}

function RecordRow({ record, probe, now, onEdit, onDelete }: RowProps) {
  return (
    <TableRow className={cn(!record.enabled && 'opacity-60')}>
      <TableCell className="max-w-[16rem]">
        <div className="flex items-center gap-1.5">
          <span className="font-medium break-all">{record.name}</span>
          <ProbeIndicator probe={probe} />
        </div>
        <div className="text-muted-foreground text-xs">{record.zone_name}</div>
      </TableCell>
      <TableCell>
        <Badge variant="outline">{record.type}</Badge>
      </TableCell>
      <TableCell className="font-mono text-xs break-all">{record.current_ip ?? '—'}</TableCell>
      <TableCell>
        <ProxyIcon proxied={record.proxied} />
      </TableCell>
      <TableCell>{formatTTL(record.ttl)}</TableCell>
      <TableCell className="max-w-[18rem]">
        <RecordStatus record={record} />
      </TableCell>
      <TableCell>
        <PropagationBadge record={record} />
      </TableCell>
      <TableCell className="text-muted-foreground hidden text-xs whitespace-nowrap lg:table-cell">
        {record.last_checked_at ? (
          <span title={absoluteTime(record.last_checked_at)}>{relativeTime(record.last_checked_at, now)}</span>
        ) : (
          '—'
        )}
      </TableCell>
      <TableCell>
        <RecordActions record={record} onEdit={onEdit} onDelete={onDelete} />
      </TableCell>
    </TableRow>
  )
}

/** Mobile Darstellung: eine Karte je Record statt Tabellenzeile. */
function RecordCard({ record, probe, now, onEdit, onDelete }: RowProps) {
  const { t } = useTranslation()
  return (
    <li
      className={cn('flex flex-col gap-2 p-4', !record.enabled && 'opacity-60')}
      data-record={`${record.name} ${record.type}`}
    >
      <div className="flex items-start justify-between gap-2">
        <div className="min-w-0">
          <div className="flex items-center gap-2">
            <span className="font-medium [overflow-wrap:anywhere]">{record.name}</span>
            <Badge variant="outline">{record.type}</Badge>
            <ProbeIndicator probe={probe} />
          </div>
          <div className="text-muted-foreground mt-0.5 flex items-center gap-2 text-xs">
            <ProxyIcon proxied={record.proxied} />
            TTL {formatTTL(record.ttl)}
            {record.last_checked_at && (
              <span>· {t('records.checkedAgo', { time: relativeTime(record.last_checked_at, now) })}</span>
            )}
          </div>
        </div>
        <RecordActions record={record} onEdit={onEdit} onDelete={onDelete} />
      </div>
      <div className="flex flex-wrap items-center gap-x-3 gap-y-1">
        <span className="font-mono text-sm break-all">{record.current_ip ?? '—'}</span>
        <div>
          <RecordStatus record={record} />
        </div>
        <PropagationBadge record={record} />
      </div>
    </li>
  )
}

export function Records() {
  const { t } = useTranslation()
  const records = useRecords()
  const zones = useZones()
  const refresh = useRefreshIP()
  const del = useDeleteRecord()
  const probes = probesByRecord(useProbes().data)
  const now = useNow()

  const [dialog, setDialog] = useState<{ open: boolean; record?: DnsRecord }>({ open: false })
  const [toDelete, setToDelete] = useState<DnsRecord>()

  const notConfigured = zones.error instanceof ApiError && zones.error.status === 503
  const code = <code className="font-mono" />

  return (
    <div className="flex flex-col gap-6">
      <div className="flex flex-wrap items-end justify-between gap-3">
        <div>
          <h1 className="text-2xl font-semibold tracking-tight">{t('nav.records')}</h1>
          <p className="text-muted-foreground text-sm">{t('records.subtitle')}</p>
        </div>
        <div className="flex gap-2">
          <Button variant="outline" onClick={() => refresh.mutate()} disabled={refresh.isPending}>
            <RefreshCw className={cn(refresh.isPending && 'animate-spin')} />
            {t('common.refreshNow')}
          </Button>
          <Button onClick={() => setDialog({ open: true })}>
            <Plus />
            {t('common.add')}
          </Button>
        </div>
      </div>

      {notConfigured && (
        <div className="border-warning/40 bg-warning/10 flex gap-3 rounded-lg border p-4 text-sm">
          <AlertTriangle className="text-warning mt-0.5 size-4 shrink-0" />
          <div>
            <p className="font-medium">{t('records.notConfigured')}</p>
            <p className="text-muted-foreground">
              <Trans i18nKey="records.notConfiguredHint" components={{ code }} />
            </p>
          </div>
        </div>
      )}
      {zones.isError && !notConfigured && (
        <div className="border-destructive/40 bg-destructive/10 text-destructive rounded-lg border p-4 text-sm">
          {t('records.cfUnreachable', { error: zones.error.message })}
        </div>
      )}
      {refresh.isError && (
        <p className="text-destructive text-sm" role="alert">
          {t('common.refreshFailed', { error: refresh.error.message })}
        </p>
      )}

      <Card className="py-0">
        {records.isPending ? (
          <div className="flex flex-col gap-2 p-6">
            <Skeleton className="h-10" />
            <Skeleton className="h-10" />
          </div>
        ) : records.isError ? (
          <p className="text-destructive p-6 text-sm">{t('records.loadError', { error: records.error.message })}</p>
        ) : records.data.length === 0 ? (
          <div className="flex flex-col items-center gap-3 p-10 text-center">
            <p className="text-muted-foreground text-sm">{t('records.empty')}</p>
            <Button variant="outline" onClick={() => setDialog({ open: true })}>
              <Plus />
              {t('records.addFirst')}
            </Button>
          </div>
        ) : (
          <>
            <div className="hidden md:block">
              <Table>
                <TableHeader>
                  <TableRow className="hover:bg-transparent">
                    <TableHead>{t('records.col.name')}</TableHead>
                    <TableHead>{t('records.col.type')}</TableHead>
                    <TableHead>{t('records.col.ip')}</TableHead>
                    <TableHead>{t('records.col.proxy')}</TableHead>
                    <TableHead>TTL</TableHead>
                    <TableHead>{t('records.col.status')}</TableHead>
                    <TableHead>{t('records.col.propagation')}</TableHead>
                    <TableHead className="hidden lg:table-cell">{t('records.col.checked')}</TableHead>
                    <TableHead className="text-right">
                      <span className="sr-only">{t('records.col.actions')}</span>
                    </TableHead>
                  </TableRow>
                </TableHeader>
                <TableBody>
                  {records.data.map((r) => (
                    <RecordRow
                      key={r.id}
                      record={r}
                      probe={probes.get(r.id)}
                      now={now}
                      onEdit={() => setDialog({ open: true, record: r })}
                      onDelete={() => setToDelete(r)}
                    />
                  ))}
                </TableBody>
              </Table>
            </div>
            <ul className="divide-y md:hidden">
              {records.data.map((r) => (
                <RecordCard
                  key={r.id}
                  record={r}
                  probe={probes.get(r.id)}
                  now={now}
                  onEdit={() => setDialog({ open: true, record: r })}
                  onDelete={() => setToDelete(r)}
                />
              ))}
            </ul>
          </>
        )}
      </Card>

      <Card>
        <CardHeader>
          <CardTitle className="text-lg">{t('records.logTitle')}</CardTitle>
          <CardDescription>{t('records.logHint')}</CardDescription>
        </CardHeader>
        <CardContent>
          <UpdateLogList limit={20} />
        </CardContent>
      </Card>

      <RecordDialog
        open={dialog.open}
        record={dialog.record}
        onOpenChange={(open) => setDialog((d) => ({ ...d, open }))}
      />

      <AlertDialog open={toDelete !== undefined} onOpenChange={(open) => !open && setToDelete(undefined)}>
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>{t('records.removeTitle')}</AlertDialogTitle>
            <AlertDialogDescription>
              <Trans
                i18nKey="records.removeHint"
                values={{ name: `${toDelete?.name} (${toDelete?.type})` }}
                components={{ name: <span className="text-foreground font-mono" /> }}
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
    </div>
  )
}
