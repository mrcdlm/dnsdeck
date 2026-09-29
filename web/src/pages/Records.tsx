import { AlertTriangle, Cloud, CloudOff, Pencil, Plus, RefreshCw, Trash2 } from 'lucide-react'
import { useState } from 'react'

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
import { ApiError, type DnsRecord } from '@/lib/api'
import { absoluteTime, relativeTime, useNow } from '@/lib/format'
import { useDeleteRecord, useRecords, useRefreshIP, useSyncRecord, useZones } from '@/lib/queries'
import { formatTTL, recordStatus } from '@/lib/records'
import { cn } from '@/lib/utils'

interface RowProps {
  record: DnsRecord
  now: number
  onEdit: () => void
  onDelete: () => void
}

function RecordActions({ record, onEdit, onDelete }: Omit<RowProps, 'now'>) {
  const sync = useSyncRecord()
  const label = `${record.name} ${record.type}`
  return (
    <div className="flex justify-end gap-1">
      <Button
        variant="ghost"
        size="icon"
        aria-label={`${label} jetzt abgleichen`}
        title="Jetzt abgleichen"
        onClick={() => sync.mutate(record.id)}
        disabled={sync.isPending}
      >
        <RefreshCw className={cn(sync.isPending && 'animate-spin')} />
      </Button>
      <Button variant="ghost" size="icon" aria-label={`${label} bearbeiten`} title="Bearbeiten" onClick={onEdit}>
        <Pencil />
      </Button>
      <Button variant="ghost" size="icon" aria-label={`${label} entfernen`} title="Entfernen" onClick={onDelete}>
        <Trash2 />
      </Button>
    </div>
  )
}

function RecordStatus({ record }: { record: DnsRecord }) {
  const st = recordStatus[record.status]
  return (
    <>
      <Badge variant={st.variant}>{st.label}</Badge>
      {record.settings_pending && (
        <p className="text-warning mt-1 text-xs">Proxy/TTL-Änderung noch nicht übertragen</p>
      )}
      {record.message && record.status !== 'ok' && (
        <p
          className={cn(
            'mt-1 text-xs break-words',
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
  return proxied ? (
    <Cloud className="size-4 text-orange-400" aria-label="Proxied" />
  ) : (
    <CloudOff className="text-muted-foreground size-4" aria-label="Nur DNS" />
  )
}

function RecordRow({ record, now, onEdit, onDelete }: RowProps) {
  return (
    <TableRow className={cn(!record.enabled && 'opacity-60')}>
      <TableCell className="max-w-[16rem]">
        <div className="font-medium break-all">{record.name}</div>
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
function RecordCard({ record, now, onEdit, onDelete }: RowProps) {
  return (
    <li className={cn('flex flex-col gap-2 p-4', !record.enabled && 'opacity-60')} data-record={`${record.name} ${record.type}`}>
      <div className="flex items-start justify-between gap-2">
        <div className="min-w-0">
          <div className="flex items-center gap-2">
            <span className="font-medium [overflow-wrap:anywhere]">{record.name}</span>
            <Badge variant="outline">{record.type}</Badge>
          </div>
          <div className="text-muted-foreground mt-0.5 flex items-center gap-2 text-xs">
            <ProxyIcon proxied={record.proxied} />
            TTL {formatTTL(record.ttl)}
            {record.last_checked_at && <span>· geprüft {relativeTime(record.last_checked_at, now)}</span>}
          </div>
        </div>
        <RecordActions record={record} onEdit={onEdit} onDelete={onDelete} />
      </div>
      <div className="flex flex-wrap items-center gap-x-3 gap-y-1">
        <span className="font-mono text-sm break-all">{record.current_ip ?? '—'}</span>
        <div>
          <RecordStatus record={record} />
        </div>
      </div>
    </li>
  )
}

export function Records() {
  const records = useRecords()
  const zones = useZones()
  const refresh = useRefreshIP()
  const del = useDeleteRecord()
  const now = useNow()

  const [dialog, setDialog] = useState<{ open: boolean; record?: DnsRecord }>({ open: false })
  const [toDelete, setToDelete] = useState<DnsRecord>()

  const notConfigured = zones.error instanceof ApiError && zones.error.status === 503

  return (
    <div className="flex flex-col gap-6">
      <div className="flex flex-wrap items-end justify-between gap-3">
        <div>
          <h1 className="text-2xl font-semibold tracking-tight">Records</h1>
          <p className="text-muted-foreground text-sm">DNS-Einträge, die auf die öffentliche IP zeigen sollen</p>
        </div>
        <div className="flex gap-2">
          <Button variant="outline" onClick={() => refresh.mutate()} disabled={refresh.isPending}>
            <RefreshCw className={cn(refresh.isPending && 'animate-spin')} />
            Jetzt aktualisieren
          </Button>
          <Button onClick={() => setDialog({ open: true })}>
            <Plus />
            Hinzufügen
          </Button>
        </div>
      </div>

      {notConfigured && (
        <div className="border-warning/40 bg-warning/10 flex gap-3 rounded-lg border p-4 text-sm">
          <AlertTriangle className="text-warning mt-0.5 size-4 shrink-0" />
          <div>
            <p className="font-medium">Cloudflare ist nicht konfiguriert</p>
            <p className="text-muted-foreground">
              Setze <code className="font-mono">CF_API_TOKEN</code> in <code className="font-mono">deploy/.env</code> und
              starte den Container neu. Benötigte Rechte: Zone → DNS → Edit, Zone → Zone → Read.
            </p>
          </div>
        </div>
      )}
      {zones.isError && !notConfigured && (
        <div className="border-destructive/40 bg-destructive/10 text-destructive rounded-lg border p-4 text-sm">
          Cloudflare nicht erreichbar: {zones.error.message}
        </div>
      )}
      {refresh.isError && (
        <p className="text-destructive text-sm" role="alert">
          Aktualisierung fehlgeschlagen: {refresh.error.message}
        </p>
      )}

      <Card className="py-0">
        {records.isPending ? (
          <div className="flex flex-col gap-2 p-6">
            <Skeleton className="h-10" />
            <Skeleton className="h-10" />
          </div>
        ) : records.isError ? (
          <p className="text-destructive p-6 text-sm">Records konnten nicht geladen werden: {records.error.message}</p>
        ) : records.data.length === 0 ? (
          <div className="flex flex-col items-center gap-3 p-10 text-center">
            <p className="text-muted-foreground text-sm">Noch keine Records verwaltet.</p>
            <Button variant="outline" onClick={() => setDialog({ open: true })}>
              <Plus />
              Ersten Record hinzufügen
            </Button>
          </div>
        ) : (
          <>
            <div className="hidden md:block">
              <Table>
                <TableHeader>
                  <TableRow className="hover:bg-transparent">
                    <TableHead>Name</TableHead>
                    <TableHead>Typ</TableHead>
                    <TableHead>IP bei Cloudflare</TableHead>
                    <TableHead>Proxy</TableHead>
                    <TableHead>TTL</TableHead>
                    <TableHead>Status</TableHead>
                    <TableHead className="hidden lg:table-cell">Geprüft</TableHead>
                    <TableHead className="text-right">
                      <span className="sr-only">Aktionen</span>
                    </TableHead>
                  </TableRow>
                </TableHeader>
                <TableBody>
                  {records.data.map((r) => (
                    <RecordRow
                      key={r.id}
                      record={r}
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
          <CardTitle className="text-lg">Update-Protokoll</CardTitle>
          <CardDescription>Angelegte und geänderte Einträge sowie Fehler</CardDescription>
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
            <AlertDialogTitle>Record nicht mehr verwalten?</AlertDialogTitle>
            <AlertDialogDescription>
              <span className="text-foreground font-mono">
                {toDelete?.name} ({toDelete?.type})
              </span>{' '}
              wird aus dnsdeck entfernt und nicht mehr aktualisiert. Der Eintrag bei Cloudflare bleibt unverändert
              bestehen.
            </AlertDialogDescription>
          </AlertDialogHeader>
          {del.isError && <p className="text-destructive text-sm">{del.error.message}</p>}
          <AlertDialogFooter>
            <AlertDialogCancel>Abbrechen</AlertDialogCancel>
            <AlertDialogAction
              onClick={(e) => {
                e.preventDefault()
                if (toDelete) del.mutate(toDelete.id, { onSuccess: () => setToDelete(undefined) })
              }}
              disabled={del.isPending}
            >
              Entfernen
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </div>
  )
}
