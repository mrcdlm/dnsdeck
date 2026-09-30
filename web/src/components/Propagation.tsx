import { AlertTriangle, Check, Loader2, RefreshCw, X } from 'lucide-react'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'

import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from '@/components/ui/dialog'
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from '@/components/ui/table'
import type { DnsRecord, Propagation, PropagationAnswer } from '@/lib/api'
import { absoluteTime, relativeTime, useNow } from '@/lib/format'
import { useCheckPropagation, useInfo } from '@/lib/queries'
import { cn } from '@/lib/utils'

/** Ergebnis passt nicht mehr zum Record (z. B. neue IP seit der Prüfung). */
function isStale(p: Propagation, r: DnsRecord) {
  return p.type !== r.type || p.proxied !== r.proxied || (!p.proxied && p.expected !== r.current_ip)
}

/** Sekunden, bis spätestens alle Resolver aktuell sein sollten (0 = unbekannt/vorbei). */
function remaining(p: Propagation, now: number) {
  if (!p.max_wait) return 0
  return Math.max(0, Math.round((Date.parse(p.checked_at) + p.max_wait * 1000 - now) / 1000))
}

function Summary({ record }: { record: DnsRecord }) {
  const { t } = useTranslation()
  const p = record.propagation
  const watching = record.propagation_watching

  if (!p || isStale(p, record)) {
    return watching ? (
      <Badge variant="secondary">
        <Loader2 className="animate-spin" />
        {t('propagation.checking')}
      </Badge>
    ) : (
      <span className="text-muted-foreground">{t('propagation.unknown')}</span>
    )
  }
  const spinner = watching && p.status !== 'propagated' && <Loader2 className="animate-spin" />
  const warn = p.errors > 0 && <AlertTriangle className="text-warning" aria-label={t('propagation.someFailed')} />
  switch (p.status) {
    case 'propagated':
      return (
        <Badge variant="success">
          <Check />
          {p.proxied ? t('propagation.resolved') : `${p.matching}/${p.total}`}
          {warn}
        </Badge>
      )
    case 'partial':
      return (
        <Badge variant="warning">
          {spinner}
          {`${p.matching}/${p.total}`}
          {warn}
        </Badge>
      )
    case 'pending':
      return (
        <Badge variant="secondary">
          {spinner}
          {`0/${p.total}`}
          {warn}
        </Badge>
      )
    default:
      return <Badge variant="destructive">{t('propagation.status.error')}</Badge>
  }
}

function AnswerValue({ a }: { a: PropagationAnswer }) {
  const { t } = useTranslation()
  if (a.error) return <span className="text-destructive">{t(`propagation.error.${a.error}`)}</span>
  if (!a.values?.length) {
    return <span className="text-muted-foreground">{t('propagation.notFound')}</span>
  }
  return <span className="font-mono break-all">{a.values.join(', ')}</span>
}

function Details({ record }: { record: DnsRecord }) {
  const { t } = useTranslation()
  const check = useCheckPropagation()
  const now = useNow(5_000)
  const p = record.propagation
  const stale = p && isStale(p, record)
  const wait = p && !stale ? remaining(p, now) : 0

  return (
    <>
      <DialogHeader>
        <DialogTitle>{t('propagation.title')}</DialogTitle>
        <DialogDescription>
          <span className="text-foreground font-mono">{record.name}</span> ({record.type}) ·{' '}
          {record.proxied
            ? t('propagation.expectProxied')
            : t('propagation.expect', { ip: record.current_ip ?? '—' })}
        </DialogDescription>
      </DialogHeader>

      {!p ? (
        <p className="text-muted-foreground text-sm">{t('propagation.never')}</p>
      ) : (
        <>
          <div className="flex flex-wrap items-center gap-2 text-sm">
            <Summary record={record} />
            {!stale && <span>{t(`propagation.status.${p.status}`)}</span>}
            {stale && <span className="text-muted-foreground">{t('propagation.stale')}</span>}
            {wait > 0 && (
              <span className="text-muted-foreground">
                · {wait < 60 ? t('propagation.etaSeconds', { count: wait }) : t('propagation.eta', { count: Math.ceil(wait / 60) })}
              </span>
            )}
          </div>
          <div className={cn('overflow-x-auto rounded-md border', stale && 'opacity-60')}>
            <Table>
              <TableHeader>
                <TableRow className="hover:bg-transparent">
                  <TableHead>{t('propagation.col.server')}</TableHead>
                  <TableHead>{t('propagation.col.answer')}</TableHead>
                  <TableHead className="text-right">TTL</TableHead>
                  <TableHead>
                    <span className="sr-only">{t('propagation.col.match')}</span>
                  </TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {p.answers.map((a, i) => (
                  <TableRow key={`${a.resolver}-${i}`}>
                    <TableCell>
                      <div className="flex items-center gap-1.5 font-medium">
                        {a.resolver}
                        {a.authoritative && <Badge variant="outline">{t('propagation.authoritative')}</Badge>}
                      </div>
                      {a.addr && <div className="text-muted-foreground font-mono text-xs">{a.addr}</div>}
                    </TableCell>
                    <TableCell className="text-xs">
                      <AnswerValue a={a} />
                    </TableCell>
                    <TableCell className="text-right text-xs tabular-nums">{a.error ? '—' : `${a.ttl} s`}</TableCell>
                    <TableCell>
                      {a.error ? null : a.match ? (
                        <Check className="text-success size-4" aria-label={t('propagation.match')} />
                      ) : (
                        <X className="text-destructive size-4" aria-label={t('propagation.noMatch')} />
                      )}
                    </TableCell>
                  </TableRow>
                ))}
              </TableBody>
            </Table>
          </div>
          <p className="text-muted-foreground text-xs" title={absoluteTime(p.checked_at)}>
            {t('propagation.checkedAt', { time: relativeTime(p.checked_at, now) })}
            {record.propagation_watching && ` · ${t('propagation.watching')}`}
          </p>
        </>
      )}
      {check.isError && (
        <p className="text-destructive text-sm" role="alert">
          {check.error.message}
        </p>
      )}
      <DialogFooter>
        <Button onClick={() => check.mutate(record.id)} disabled={check.isPending}>
          {check.isPending ? <Loader2 className="animate-spin" /> : <RefreshCw />}
          {t('propagation.checkNow')}
        </Button>
      </DialogFooter>
    </>
  )
}

/** DNS-Verbreitung eines Records; Klick öffnet die Details je Resolver. */
export function PropagationBadge({ record }: { record: DnsRecord }) {
  const { t } = useTranslation()
  const info = useInfo()
  const [open, setOpen] = useState(false)

  if (info.data && !info.data.dnscheck) {
    return (
      <span className="text-muted-foreground" title={t('propagation.disabled')}>
        —
      </span>
    )
  }
  if (!record.enabled) return <span className="text-muted-foreground">—</span>

  return (
    <>
      <button
        type="button"
        className="focus-visible:ring-ring/50 rounded-md outline-none hover:opacity-80 focus-visible:ring-[3px]"
        aria-label={t('propagation.detailsLabel', { name: `${record.name} ${record.type}` })}
        onClick={() => setOpen(true)}
      >
        <Summary record={record} />
      </button>
      <Dialog open={open} onOpenChange={setOpen}>
        <DialogContent className="sm:max-w-2xl">{open && <Details record={record} />}</DialogContent>
      </Dialog>
    </>
  )
}
