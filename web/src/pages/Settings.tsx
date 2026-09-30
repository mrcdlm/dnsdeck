import { AlertTriangle, ArrowDown, ArrowUp, Check, Info as InfoIcon, Loader2, Pencil, Plus, Send, Timer, Trash2, Wifi, X } from 'lucide-react'
import { useEffect, useState, type ReactNode } from 'react'

import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card'
import { Label } from '@/components/ui/label'
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select'
import { Skeleton } from '@/components/ui/skeleton'
import { Switch } from '@/components/ui/switch'
import { WebhookDialog } from '@/components/WebhookDialog'
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
import type { Settings as SettingsData, Webhook } from '@/lib/api'
import { absoluteTime, relativeTime, useNow } from '@/lib/format'
import {
  useDeleteWebhook,
  useInfo,
  useSaveSettings,
  useSaveWebhook,
  useSettings,
  useTestWebhook,
  useWebhooks,
} from '@/lib/queries'
import { eventInfo, webhookInput } from '@/lib/webhooks'

const ipIntervals = [60, 120, 300, 600, 900, 1800, 3600]
const tunnelIntervals = [30, 60, 120, 300, 600]

const sourceInfo: Record<string, string> = {
  cloudflare: 'Cloudflare (1.1.1.1/cdn-cgi/trace)',
  ipify: 'ipify (api.ipify.org / api6.ipify.org)',
  icanhazip: 'icanhazip.com',
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

// saved liegt beim Aufrufer: das Formular wird nach dem Speichern mit den neuen
// Werten neu aufgebaut (key) und verlöre sonst die Bestätigung.
function SettingsForm({
  initial,
  saved,
  setSaved,
}: {
  initial: SettingsData
  saved: boolean
  setSaved: (v: boolean) => void
}) {
  const save = useSaveSettings()
  const [draft, setDraft] = useState(initial)
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

function WebhookCard({ webhook, onEdit, onDelete }: { webhook: Webhook; onEdit: () => void; onDelete: () => void }) {
  const test = useTestWebhook()
  const save = useSaveWebhook()
  const now = useNow()
  const { id, missing_env, last_error, last_sent_at } = webhook

  // Testergebnis nur kurz zeigen – danach gilt wieder der gespeicherte Status.
  const { reset: resetTest, data: testData } = test
  useEffect(() => {
    if (!testData) return
    const t = setTimeout(resetTest, 8000)
    return () => clearTimeout(t)
  }, [testData, resetTest])

  return (
    <li className="flex flex-col gap-2 px-3 py-3" data-webhook={webhook.name}>
      <div className="flex flex-wrap items-center gap-2">
        <Switch
          checked={webhook.enabled}
          aria-label={`${webhook.name} aktiv`}
          disabled={save.isPending}
          onCheckedChange={(on) => save.mutate({ id, input: { ...webhookInput(webhook), enabled: on } })}
        />
        <span className="font-medium">{webhook.name}</span>
        {!webhook.enabled && <Badge variant="outline">inaktiv</Badge>}
        <div className="ml-auto flex gap-1">
          <Button type="button" variant="ghost" size="sm" onClick={() => test.mutate(id)} disabled={test.isPending}>
            {test.isPending ? <Loader2 className="animate-spin" /> : <Send />}
            Testen
          </Button>
          <Button type="button" variant="ghost" size="icon" aria-label={`${webhook.name} bearbeiten`} onClick={onEdit}>
            <Pencil />
          </Button>
          <Button type="button" variant="ghost" size="icon" aria-label={`${webhook.name} löschen`} onClick={onDelete}>
            <Trash2 />
          </Button>
        </div>
      </div>
      <p className="text-muted-foreground font-mono text-xs break-all">
        {webhook.method} {webhook.url}
      </p>
      <div className="flex flex-wrap gap-1">
        {webhook.events.length === 0 ? (
          <span className="text-warning text-xs">Kein Ereignis ausgewählt – sendet nur Testnachrichten</span>
        ) : (
          webhook.events.map((e) => (
            <Badge key={e} variant="secondary">
              {eventInfo[e].label}
            </Badge>
          ))
        )}
      </div>
      {missing_env.length > 0 && (
        <p className="text-warning flex items-start gap-1.5 text-xs">
          <AlertTriangle className="mt-0.5 size-3.5 shrink-0" />
          <span>
            Nicht gesetzt: <code className="font-mono">{missing_env.join(', ')}</code> – in deploy/.env eintragen und Container
            neu starten.
          </span>
        </p>
      )}
      {save.isError && (
        <p className="text-destructive text-xs break-words" role="alert">
          Speichern fehlgeschlagen: {save.error.message}
        </p>
      )}
      <p className="text-xs">
        {test.data ? (
          test.data.ok ? (
            <span className="text-success flex items-center gap-1">
              <Check className="size-3.5" /> Testnachricht zugestellt
            </span>
          ) : (
            <span className="text-destructive flex items-center gap-1 break-words">
              <X className="size-3.5 shrink-0" /> {test.data.error}
            </span>
          )
        ) : last_error ? (
          <span className="text-destructive break-words">Letzte Zustellung fehlgeschlagen: {last_error}</span>
        ) : last_sent_at ? (
          <span className="text-muted-foreground" title={absoluteTime(last_sent_at)}>
            Zuletzt zugestellt {relativeTime(last_sent_at, now)}
          </span>
        ) : (
          <span className="text-muted-foreground">Noch nichts gesendet</span>
        )}
      </p>
    </li>
  )
}

function Webhooks() {
  const webhooks = useWebhooks()
  const del = useDeleteWebhook()
  const [dialog, setDialog] = useState<{ open: boolean; webhook?: Webhook }>({ open: false })
  const [toDelete, setToDelete] = useState<Webhook>()
  const list = webhooks.data ?? []

  return (
    <Section
      icon={<Send className="size-5" />}
      title="Benachrichtigungen (Webhooks)"
      description="Beliebige Dienste per HTTP-Anfrage benachrichtigen – Vorlagen für ntfy, Gotify, Discord, Slack, Telegram und Home Assistant."
    >
      {webhooks.isPending ? (
        <Skeleton className="h-16" />
      ) : webhooks.isError ? (
        <p className="text-destructive text-sm">Webhooks konnten nicht geladen werden: {webhooks.error.message}</p>
      ) : list.length === 0 ? (
        <p className="text-muted-foreground text-sm">Noch keine Webhooks.</p>
      ) : (
        <ul className="divide-y rounded-md border">
          {list.map((w) => (
            <WebhookCard key={w.id} webhook={w} onEdit={() => setDialog({ open: true, webhook: w })} onDelete={() => setToDelete(w)} />
          ))}
        </ul>
      )}
      <div>
        <Button type="button" variant="outline" onClick={() => setDialog({ open: true })}>
          <Plus /> Webhook hinzufügen
        </Button>
      </div>

      <WebhookDialog open={dialog.open} webhook={dialog.webhook} onOpenChange={(open) => setDialog((d) => ({ ...d, open }))} />

      <AlertDialog open={toDelete !== undefined} onOpenChange={(open) => !open && setToDelete(undefined)}>
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>Webhook löschen?</AlertDialogTitle>
            <AlertDialogDescription>
              „{toDelete?.name}“ wird gelöscht und erhält keine Benachrichtigungen mehr. Die Env-Variablen in deploy/.env
              bleiben unverändert.
            </AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel>Abbrechen</AlertDialogCancel>
            <AlertDialogAction
              onClick={(e) => {
                e.preventDefault()
                if (toDelete) del.mutate(toDelete.id, { onSuccess: () => setToDelete(undefined) })
              }}
              disabled={del.isPending}
            >
              Löschen
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
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
  const [saved, setSaved] = useState(false)

  return (
    <div className="flex flex-col gap-6">
      <div>
        <h1 className="text-2xl font-semibold tracking-tight">Einstellungen</h1>
        <p className="text-muted-foreground text-sm">Intervalle, IP-Quellen und Webhooks</p>
      </div>
      {settings.isPending ? (
        <Skeleton className="h-96" />
      ) : settings.isError ? (
        <p className="text-destructive text-sm">Einstellungen konnten nicht geladen werden: {settings.error.message}</p>
      ) : (
        // key: nach dem Speichern (oder Änderung in anderem Tab) neu initialisieren
        <SettingsForm key={JSON.stringify(settings.data)} initial={settings.data} saved={saved} setSaved={setSaved} />
      )}
      <Webhooks />
      <SystemInfo />
    </div>
  )
}
