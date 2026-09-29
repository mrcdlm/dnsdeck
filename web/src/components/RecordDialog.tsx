import { Loader2 } from 'lucide-react'
import { useState, type FormEvent } from 'react'

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
import { useSaveRecord, useZones } from '@/lib/queries'
import { subdomainOf, ttlOptions } from '@/lib/records'

type TypeChoice = RecordType | 'both'

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
  const zones = useZones()
  const save = useSaveRecord()
  const editing = record !== undefined

  const [zoneId, setZoneId] = useState(record?.zone_id ?? '')
  const [sub, setSub] = useState(record ? subdomainOf(record.name, record.zone_name) : '')
  const [type, setType] = useState<TypeChoice>(record?.type ?? 'A')
  const [proxied, setProxied] = useState(record?.proxied ?? false)
  const [ttl, setTTL] = useState(record?.ttl ?? 1)
  const [enabled, setEnabled] = useState(record?.enabled ?? true)
  const [error, setError] = useState<string>()

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
      for (const t of types) {
        await save.mutateAsync({
          id: record?.id,
          input: { zone_id: zone.id, name: fqdn, type: t, proxied, ttl: proxied ? 1 : ttl, enabled },
        })
        // Scheitert danach AAAA, legt ein erneuter Versuch nur noch AAAA an.
        if (type === 'both' && t === 'A') setType('AAAA')
      }
      onDone()
    } catch (err) {
      setError(err instanceof Error ? err.message : String(err))
    }
  }

  return (
    <form onSubmit={submit} className="flex flex-col gap-5">
      <DialogHeader>
        <DialogTitle>{editing ? 'Record bearbeiten' : 'Record hinzufügen'}</DialogTitle>
        <DialogDescription>
          {editing
            ? 'Geänderte Proxy-/TTL-Werte werden sofort zu Cloudflare übertragen.'
            : 'Fehlt der Eintrag bei Cloudflare, wird er mit diesen Werten angelegt. Existiert er bereits, übernimmt dnsdeck Proxy und TTL von Cloudflare und ändert nur die IP.'}
        </DialogDescription>
      </DialogHeader>

      {zones.isError && (
        <p className="border-destructive/40 bg-destructive/10 text-destructive rounded-md border p-3 text-sm">
          Zonen konnten nicht geladen werden: {zones.error.message}
        </p>
      )}

      <div className="grid gap-2">
        <Label htmlFor="zone">Zone</Label>
        <Select value={effectiveZoneId} onValueChange={setZoneId} disabled={!zones.data?.length}>
          <SelectTrigger id="zone" aria-label="Zone">
            <SelectValue placeholder={zones.isPending ? 'Lade Zonen …' : 'Zone wählen'} />
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
        <Label htmlFor="sub">Name</Label>
        <div className="flex items-center gap-2">
          <Input
            id="sub"
            placeholder="z. B. home (leer = Zone selbst)"
            value={sub}
            onChange={(e) => setSub(e.target.value)}
            autoComplete="off"
            spellCheck={false}
          />
          {zone && <span className="text-muted-foreground shrink-0 text-sm">.{zone.name}</span>}
        </div>
        {fqdn && (
          <p className="text-muted-foreground text-xs">
            Vollständiger Name: <span className="text-foreground font-mono">{fqdn}</span>
          </p>
        )}
      </div>

      <div className="grid gap-4 sm:grid-cols-2">
        <div className="grid gap-2">
          <Label htmlFor="type">Typ</Label>
          <Select value={type} onValueChange={(v) => setType(v as TypeChoice)}>
            <SelectTrigger id="type" aria-label="Typ">
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
              {ttlOptions.map((o) => (
                <SelectItem key={o.value} value={String(o.value)}>
                  {o.label}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
        </div>
      </div>

      <div className="flex flex-col gap-4">
        <div className="flex items-start justify-between gap-4">
          <div className="grid gap-1">
            <Label htmlFor="proxied">Über Cloudflare proxien</Label>
            <p className="text-muted-foreground text-xs">
              Orange Wolke: Verkehr läuft über Cloudflare, die TTL ist dann automatisch.
            </p>
          </div>
          <Switch id="proxied" checked={proxied} onCheckedChange={setProxied} />
        </div>
        <div className="flex items-start justify-between gap-4">
          <div className="grid gap-1">
            <Label htmlFor="enabled">Automatisch aktualisieren</Label>
            <p className="text-muted-foreground text-xs">Deaktiviert: Eintrag bleibt verwaltet, wird aber nicht geändert.</p>
          </div>
          <Switch id="enabled" checked={enabled} onCheckedChange={setEnabled} />
        </div>
      </div>

      {error && (
        <p className="text-destructive text-sm" role="alert">
          {error}
        </p>
      )}

      <DialogFooter>
        <Button type="button" variant="outline" onClick={onDone}>
          Abbrechen
        </Button>
        <Button type="submit" disabled={!zone || save.isPending}>
          {save.isPending && <Loader2 className="animate-spin" />}
          {editing ? 'Speichern' : 'Hinzufügen'}
        </Button>
      </DialogFooter>
    </form>
  )
}
