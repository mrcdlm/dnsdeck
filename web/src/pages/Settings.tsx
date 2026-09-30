import { AlertTriangle, ArrowDown, ArrowUp, Bell, Check, Info as InfoIcon, Loader2, Send, Timer, Wifi, X } from 'lucide-react'
import { useState, type ReactNode } from 'react'

import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card'
import { Label } from '@/components/ui/label'
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select'
import { Skeleton } from '@/components/ui/skeleton'
import { Switch } from '@/components/ui/switch'
import type { NotifyEventType, Settings as SettingsData } from '@/lib/api'
import { absoluteTime, relativeTime, useNow } from '@/lib/format'
import { useInfo, useNotifications, useSaveSettings, useSettings, useTestNotifications } from '@/lib/queries'

const ipIntervals = [60, 120, 300, 600, 900, 1800, 3600]
const tunnelIntervals = [30, 60, 120, 300, 600]

const sourceInfo: Record<string, string> = {
  cloudflare: 'Cloudflare (1.1.1.1/cdn-cgi/trace)',
  ipify: 'ipify (api.ipify.org / api6.ipify.org)',
  icanhazip: 'icanhazip.com',
}

const eventInfo: Record<NotifyEventType, { label: string; hint: string }> = {
  ip_change: { label: 'IP-Wechsel', hint: 'Die öffentliche IPv4 oder IPv6 hat sich geändert.' },
  update_failed: { label: 'DNS-Update fehlgeschlagen', hint: 'Einmal je neuem Fehler, nicht bei jeder Wiederholung.' },
  update_recovered: { label: 'DNS-Update wieder OK', hint: 'Ein zuvor fehlerhafter Record ist wieder aktuell.' },
  tunnel_status: { label: 'Tunnel-Statuswechsel', hint: 'z. B. verbunden → getrennt.' },
}

function formatSeconds(s: number): string {
  if (s % 3600 === 0) return s === 3600 ? '1 Stunde' : `${s / 3600} Stunden`
  if (s % 60 === 0) return s === 60 ? '1 Minute' : `${s / 60} Minuten`
  return `${s} Sekunden`
}

function IntervalSelect({
  id,
  value,
  presets,
  onChange,
}: {
  id: string
  value: number
  presets: number[]
  onChange: (v: number) => void
}) {
  // Ein gespeicherter Wert außerhalb der Vorgaben bleibt wählbar.
  const options = presets.includes(value) ? presets : [...presets, value].sort((a, b) => a - b)
  return (
    <Select value={String(value)} onValueChange={(v) => onChange(Number(v))}>
      <SelectTrigger id={id} className="w-full sm:w-48">
        <SelectValue />
      </SelectTrigger>
      <SelectContent>
        {options.map((s) => (
          <SelectItem key={s} value={String(s)}>
            {formatSeconds(s)}
          </SelectItem>
        ))}
      </SelectContent>
    </Select>
  )
}

function Section({ icon, title, description, children }: { icon: ReactNode; title: string; description: string; children: ReactNode }) {
  return (
    <Card>
      <CardHeader>
        <CardTitle className="flex items-center gap-2 text-lg">
          {icon}
          {title}
        </CardTitle>
        <CardDescription>{description}</CardDescription>
      </CardHeader>
      <CardContent className="flex flex-col gap-5">{children}</CardContent>
    </Card>
  )
}

function SettingsForm({ initial }: { initial: SettingsData }) {
  const save = useSaveSettings()
  const [draft, setDraft] = useState(initial)
  const [saved, setSaved] = useState(false)
  const dirty = JSON.stringify(draft) !== JSON.stringify(initial)
  const enabledSources = draft.ip_sources.filter((s) => s.enabled).length
  const minSources = draft.limits.ip_sources_min ?? 2

  function update(patch: Partial<SettingsData>) {
    setSaved(false)
    setDraft((d) => ({ ...d, ...patch }))
  }

  function moveSource(i: number, dir: -1 | 1) {
    const list = [...draft.ip_sources]
    const j = i + dir
    if (j < 0 || j >= list.length) return
    ;[list[i], list[j]] = [list[j], list[i]]
    update({ ip_sources: list })
  }

  return (
    <form
      className="flex flex-col gap-6"
      onSubmit={(e) => {
        e.preventDefault()
        save.mutate(draft, { onSuccess: (s) => (setDraft(s), setSaved(true)) })
      }}
    >
      <Section icon={<Timer className="size-5" />} title="Intervalle" description="Änderungen gelten sofort, ohne Neustart.">
        <div className="flex flex-col gap-2 sm:flex-row sm:items-center sm:justify-between">
          <div className="grid gap-1">
            <Label htmlFor="ip-interval">IP prüfen und DNS abgleichen</Label>
            <p className="text-muted-foreground text-xs">So lange zeigt ein Record nach einem IP-Wechsel höchstens auf die alte IP.</p>
          </div>
          <IntervalSelect
            id="ip-interval"
            value={draft.ip_check_interval_seconds}
            presets={ipIntervals}
            onChange={(v) => update({ ip_check_interval_seconds: v })}
          />
        </div>
        <div className="flex flex-col gap-2 sm:flex-row sm:items-center sm:justify-between">
          <div className="grid gap-1">
            <Label htmlFor="tunnel-interval">Tunnels abfragen</Label>
            <p className="text-muted-foreground text-xs">Bestimmt, wie schnell ein Tunnel-Ausfall erkannt wird.</p>
          </div>
          <IntervalSelect
            id="tunnel-interval"
            value={draft.tunnel_interval_seconds}
            presets={tunnelIntervals}
            onChange={(v) => update({ tunnel_interval_seconds: v })}
          />
        </div>
      </Section>

      <Section
        icon={<Wifi className="size-5" />}
        title="IP-Quellen"
        description={`Die öffentliche IP gilt erst, wenn die Mehrheit der Quellen übereinstimmt. Mindestens ${minSources} aktivieren, empfohlen: alle.`}
      >
        <ul className="divide-y rounded-md border">
          {draft.ip_sources.map((s, i) => (
            <li key={s.name} className="flex items-center gap-3 px-3 py-2">
              <Switch
                id={`src-${s.name}`}
                checked={s.enabled}
                onCheckedChange={(on) =>
                  update({ ip_sources: draft.ip_sources.map((x) => (x.name === s.name ? { ...x, enabled: on } : x)) })
                }
              />
              <Label htmlFor={`src-${s.name}`} className="min-w-0 flex-1 font-normal">
                {sourceInfo[s.name] ?? s.name}
              </Label>
              <Button type="button" variant="ghost" size="icon" aria-label={`${s.name} nach oben`} disabled={i === 0} onClick={() => moveSource(i, -1)}>
                <ArrowUp />
              </Button>
              <Button
                type="button"
                variant="ghost"
                size="icon"
                aria-label={`${s.name} nach unten`}
                disabled={i === draft.ip_sources.length - 1}
                onClick={() => moveSource(i, 1)}
              >
                <ArrowDown />
              </Button>
            </li>
          ))}
        </ul>
        {enabledSources < minSources && (
          <p className="text-destructive text-sm">Mindestens {minSources} Quellen aktivieren.</p>
        )}
      </Section>

      <Section icon={<Bell className="size-5" />} title="Benachrichtigen bei" description="Gilt für alle konfigurierten Kanäle.">
        {(Object.keys(eventInfo) as NotifyEventType[]).map((t) => (
          <div key={t} className="flex items-start justify-between gap-4">
            <div className="grid gap-1">
              <Label htmlFor={`ev-${t}`}>{eventInfo[t].label}</Label>
              <p className="text-muted-foreground text-xs">{eventInfo[t].hint}</p>
            </div>
            <Switch
              id={`ev-${t}`}
              checked={draft.notify_events[t]}
              onCheckedChange={(on) => update({ notify_events: { ...draft.notify_events, [t]: on } })}
            />
          </div>
        ))}
      </Section>

      <div className="bg-background/90 sticky bottom-0 -mx-4 flex flex-wrap items-center justify-end gap-3 border-t px-4 py-3 backdrop-blur md:-mx-8 md:px-8">
        {save.isError && <p className="text-destructive mr-auto text-sm" role="alert">{save.error.message}</p>}
        {saved && !dirty && (
          <span className="text-success mr-auto flex items-center gap-1 text-sm">
            <Check className="size-4" /> Gespeichert
          </span>
        )}
        <Button type="button" variant="outline" disabled={!dirty || save.isPending} onClick={() => (setDraft(initial), setSaved(false))}>
          Verwerfen
        </Button>
        <Button type="submit" disabled={!dirty || save.isPending || enabledSources < minSources}>
          {save.isPending && <Loader2 className="animate-spin" />}
          Speichern
        </Button>
      </div>
    </form>
  )
}

function NotificationChannels() {
  const n = useNotifications()
  const test = useTestNotifications()
  const now = useNow()
  const channels = n.data?.channels ?? []

  return (
    <Section
      icon={<Send className="size-5" />}
      title="Benachrichtigungskanäle"
      description="Aus Sicherheitsgründen nur per Env-Variable konfigurierbar (URLs und Tokens sind Geheimnisse)."
    >
      {n.data?.config_error && (
        <div className="border-warning/40 bg-warning/10 flex gap-3 rounded-md border p-3 text-sm">
          <AlertTriangle className="text-warning mt-0.5 size-4 shrink-0" />
          <span>{n.data.config_error}</span>
        </div>
      )}
      {n.isPending ? (
        <Skeleton className="h-12" />
      ) : channels.length === 0 ? (
        <div className="text-muted-foreground text-sm">
          <p>Keine Kanäle konfiguriert. In <code className="font-mono">deploy/.env</code> eintragen und neu starten:</p>
          <pre className="bg-muted mt-2 overflow-x-auto rounded-md p-3 text-xs">
{`NOTIFY_NTFY_URL=https://ntfy.sh/dein-topic
NOTIFY_NTFY_TOKEN=            # optional
NOTIFY_GOTIFY_URL=https://gotify.example.com
NOTIFY_GOTIFY_TOKEN=app-token
NOTIFY_WEBHOOK_URL=https://example.com/hook`}
          </pre>
        </div>
      ) : (
        <ul className="divide-y rounded-md border">
          {channels.map((c, i) => {
            const result = test.data?.[i]
            return (
              <li key={`${c.type}-${i}`} className="flex flex-col gap-1 px-3 py-2">
                <div className="flex flex-wrap items-center gap-2">
                  <Badge variant="outline">{c.type}</Badge>
                  <span className="text-muted-foreground font-mono text-xs">{c.target}</span>
                  {result &&
                    (result.ok ? (
                      <span className="text-success ml-auto flex items-center gap-1 text-xs">
                        <Check className="size-3.5" /> Test zugestellt
                      </span>
                    ) : (
                      <span className="text-destructive ml-auto flex items-center gap-1 text-xs">
                        <X className="size-3.5" /> {result.error}
                      </span>
                    ))}
                </div>
                <p className="text-muted-foreground text-xs">
                  {c.last_error ? (
                    <span className="text-destructive">Letzter Versuch fehlgeschlagen: {c.last_error}</span>
                  ) : c.last_sent ? (
                    <span title={absoluteTime(c.last_sent)}>Zuletzt zugestellt {relativeTime(c.last_sent, now)}</span>
                  ) : (
                    'Noch nichts gesendet'
                  )}
                </p>
              </li>
            )
          })}
        </ul>
      )}
      {channels.length > 0 && (
        <div>
          <Button type="button" variant="outline" onClick={() => test.mutate()} disabled={test.isPending}>
            {test.isPending ? <Loader2 className="animate-spin" /> : <Send />}
            Testnachricht senden
          </Button>
          {test.isError && <p className="text-destructive mt-2 text-sm">{test.error.message}</p>}
        </div>
      )}
    </Section>
  )
}

function InfoRow({ label, children }: { label: string; children: ReactNode }) {
  return (
    <div className="flex justify-between gap-4">
      <dt className="text-muted-foreground shrink-0">{label}</dt>
      <dd className="min-w-0 text-right break-all">{children}</dd>
    </div>
  )
}

function SystemInfo() {
  const info = useInfo()
  const d = info.data
  const missing = <span className="text-warning">fehlt</span>
  return (
    <Section icon={<InfoIcon className="size-5" />} title="System" description="Zugangsdaten werden nie angezeigt, nur ob sie gesetzt sind.">
      {!d ? (
        <Skeleton className="h-20" />
      ) : (
        <dl className="grid gap-2 text-sm">
          <InfoRow label="Version">
            <span className="font-mono">{d.version}</span>
          </InfoRow>
          <InfoRow label="Cloudflare-Token">{d.cf_token_set ? 'gesetzt' : missing}</InfoRow>
          <InfoRow label="Cloudflare-Account-ID">{d.cf_account_set ? 'gesetzt' : missing}</InfoRow>
          <InfoRow label="Datenverzeichnis">
            <span className="font-mono">{d.data_dir}</span>
          </InfoRow>
        </dl>
      )}
    </Section>
  )
}

export function Settings() {
  const settings = useSettings()

  return (
    <div className="flex flex-col gap-6">
      <div>
        <h1 className="text-2xl font-semibold tracking-tight">Einstellungen</h1>
        <p className="text-muted-foreground text-sm">Intervalle, IP-Quellen und Benachrichtigungen</p>
      </div>
      {settings.isPending ? (
        <Skeleton className="h-96" />
      ) : settings.isError ? (
        <p className="text-destructive text-sm">Einstellungen konnten nicht geladen werden: {settings.error.message}</p>
      ) : (
        // key: nach dem Speichern (oder Änderung in anderem Tab) neu initialisieren
        <SettingsForm key={JSON.stringify(settings.data)} initial={settings.data} />
      )}
      <NotificationChannels />
      <SystemInfo />
    </div>
  )
}
