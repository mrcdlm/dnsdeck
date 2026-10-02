import type { TFunction } from 'i18next'
import { Loader2 } from 'lucide-react'
import { useState, type FormEvent } from 'react'
import { useTranslation } from 'react-i18next'

import { Button } from '@/components/ui/button'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select'
import { Switch } from '@/components/ui/switch'
import type { DnsRecord, RecordType } from '@/lib/api'
import { probesByRecord } from '@/lib/probes'
import { useProbes, useSaveRecord, useZones } from '@/lib/queries'
import { subdomainOf, ttlOptions } from '@/lib/records'

type TypeChoice = RecordType | 'both'

/** Beschriftung einer TTL-Option, z. B. "5 Minuten" / "5 minutes". */
function ttlLabel(t: TFunction, ttl: number): string {
  if (ttl === 1) return t('ttl.auto')
  if (ttl % 86400 === 0) return t('ttl.days', { count: ttl / 86400 })
  if (ttl % 3600 === 0) return t('ttl.hours', { count: ttl / 3600 })
  if (ttl % 60 === 0) return t('ttl.minutes', { count: ttl / 60 })
  return t('ttl.seconds', { count: ttl })
}

interface Props {
  open: boolean
  onOpenChange: (open: boolean) => void
  /** Zu bearbeitender Record; undefined = neu anlegen. */
  record?: DnsRecord
}

export function RecordDialog({ open, onOpenChange, record }: Props) {
  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent>
        {/* key: Formular bei jedem Öffnen neu initialisieren */}
        {open && <RecordForm key={record?.id ?? 'new'} record={record} onDone={() => onOpenChange(false)} />}
      </DialogContent>
    </Dialog>
  )
}

function RecordForm({ record, onDone }: { record?: DnsRecord; onDone: () => void }) {
  const { t } = useTranslation()
  const zones = useZones()
  const save = useSaveRecord()
  const editing = record !== undefined

  const [zoneId, setZoneId] = useState(record?.zone_id ?? '')
  const [sub, setSub] = useState(record ? subdomainOf(record.name, record.zone_name) : '')
  const [type, setType] = useState<TypeChoice>(record?.type ?? 'A')
  const [proxied, setProxied] = useState(record?.proxied ?? false)
  const [ttl, setTTL] = useState(record?.ttl ?? 1)
  const [enabled, setEnabled] = useState(record?.enabled ?? true)
  // undefined = nicht angefasst: beim Bearbeiten bleibt die Prüfung dann unverändert.
  const [probe, setProbe] = useState<boolean>()
  const [error, setError] = useState<string>()
  const probes = useProbes()
  const hasProbe = record !== undefined && probesByRecord(probes.data).has(record.id)
  const probeOn = probe ?? hasProbe

  // Genau eine Zone → vorauswählen
  const effectiveZoneId = zoneId || (zones.data?.length === 1 ? zones.data[0].id : '')
  const zone = zones.data?.find((z) => z.id === effectiveZoneId)
  const cleanSub = sub.trim().toLowerCase().replace(/\.$/, '')
  const fqdn = zone ? (cleanSub && cleanSub !== '@' ? `${cleanSub}.${zone.name}` : zone.name) : ''

  async function submit(e: FormEvent) {
    e.preventDefault()
    if (!zone) return
    setError(undefined)
    const types: RecordType[] = type === 'both' ? ['A', 'AAAA'] : [type]
    try {
      for (const rt of types) {
        await save.mutateAsync({
          id: record?.id,
          input: {
            zone_id: zone.id,
            name: fqdn,
            type: rt,
            proxied,
            ttl: proxied ? 1 : ttl,
            enabled,
            // Bei A + AAAA nur einmal prüfen – die Adresse ist dieselbe.
            probe: rt === types[0] ? probe : undefined,
          },
        })
        // Scheitert danach AAAA, legt ein erneuter Versuch nur noch AAAA an.
        if (type === 'both' && rt === 'A') setType('AAAA')
      }
      onDone()
    } catch (err) {
      setError(err instanceof Error ? err.message : String(err))
    }
  }

  return (
    <form onSubmit={submit} className="flex flex-col gap-5">
      <DialogHeader>
        <DialogTitle>{editing ? t('recordDialog.editTitle') : t('recordDialog.addTitle')}</DialogTitle>
        <DialogDescription>{editing ? t('recordDialog.editHint') : t('recordDialog.addHint')}</DialogDescription>
      </DialogHeader>

      {zones.isError && (
        <p className="border-destructive/40 bg-destructive/10 text-destructive rounded-md border p-3 text-sm">
          {t('recordDialog.zonesError', { error: zones.error.message })}
        </p>
      )}

      <div className="grid gap-2">
        <Label htmlFor="zone">{t('recordDialog.zone')}</Label>
        <Select value={effectiveZoneId} onValueChange={setZoneId} disabled={!zones.data?.length}>
          <SelectTrigger id="zone" aria-label={t('recordDialog.zone')}>
            <SelectValue placeholder={zones.isPending ? t('recordDialog.zonesLoading') : t('recordDialog.zonePick')} />
          </SelectTrigger>
          <SelectContent>
            {zones.data?.map((z) => (
              <SelectItem key={z.id} value={z.id}>
                {z.name}
              </SelectItem>
            ))}
          </SelectContent>
        </Select>
      </div>

      <div className="grid gap-2">
        <Label htmlFor="sub">{t('recordDialog.name')}</Label>
        <div className="flex items-center gap-2">
          <Input
            id="sub"
            placeholder={t('recordDialog.namePlaceholder')}
            value={sub}
            onChange={(e) => setSub(e.target.value)}
            autoComplete="off"
            spellCheck={false}
          />
          {zone && <span className="text-muted-foreground shrink-0 text-sm">.{zone.name}</span>}
        </div>
        {fqdn && (
          <p className="text-muted-foreground text-xs">
            {t('recordDialog.fqdn')} <span className="text-foreground font-mono">{fqdn}</span>
          </p>
        )}
      </div>

      <div className="grid gap-4 sm:grid-cols-2">
        <div className="grid gap-2">
          <Label htmlFor="type">{t('recordDialog.type')}</Label>
          <Select value={type} onValueChange={(v) => setType(v as TypeChoice)}>
            <SelectTrigger id="type" aria-label={t('recordDialog.type')}>
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              <SelectItem value="A">A (IPv4)</SelectItem>
              <SelectItem value="AAAA">AAAA (IPv6)</SelectItem>
              {!editing && <SelectItem value="both">A + AAAA</SelectItem>}
            </SelectContent>
          </Select>
        </div>
        <div className="grid gap-2">
          <Label htmlFor="ttl">TTL</Label>
          <Select value={String(proxied ? 1 : ttl)} onValueChange={(v) => setTTL(Number(v))} disabled={proxied}>
            <SelectTrigger id="ttl" aria-label="TTL">
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              {ttlOptions.map((v) => (
                <SelectItem key={v} value={String(v)}>
                  {ttlLabel(t, v)}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
        </div>
      </div>

      <div className="flex flex-col gap-4">
        <div className="flex items-start justify-between gap-4">
          <div className="grid gap-1">
            <Label htmlFor="proxied">{t('recordDialog.proxied')}</Label>
            <p className="text-muted-foreground text-xs">{t('recordDialog.proxiedHint')}</p>
          </div>
          <Switch id="proxied" checked={proxied} onCheckedChange={setProxied} />
        </div>
        <div className="flex items-start justify-between gap-4">
          <div className="grid gap-1">
            <Label htmlFor="enabled">{t('recordDialog.enabled')}</Label>
            <p className="text-muted-foreground text-xs">{t('recordDialog.enabledHint')}</p>
          </div>
          <Switch id="enabled" checked={enabled} onCheckedChange={setEnabled} />
        </div>
        <div className="flex items-start justify-between gap-4">
          <div className="grid gap-1">
            <Label htmlFor="probe">{t('recordDialog.probe')}</Label>
            <p className="text-muted-foreground text-xs">
              {t('recordDialog.probeHint', { url: `https://${fqdn || '…'}/` })}
            </p>
          </div>
          <Switch id="probe" checked={probeOn} onCheckedChange={setProbe} />
        </div>
      </div>

      {error && (
        <p className="text-destructive text-sm" role="alert">
          {error}
        </p>
      )}

      <DialogFooter>
        <Button type="button" variant="outline" onClick={onDone}>
          {t('common.cancel')}
        </Button>
        <Button type="submit" disabled={!zone || save.isPending}>
          {save.isPending && <Loader2 className="animate-spin" />}
          {editing ? t('common.save') : t('common.add')}
        </Button>
      </DialogFooter>
    </form>
  )
}
