import { Loader2, Plus, Trash2 } from 'lucide-react'
import { useEffect, useState, type FormEvent } from 'react'

import { Button } from '@/components/ui/button'
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from '@/components/ui/dialog'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select'
import { Switch } from '@/components/ui/switch'
import { Textarea } from '@/components/ui/textarea'
import { api, type NotifyEventType, type Webhook, type WebhookInput, type WebhookMethod, type WebhookPreview } from '@/lib/api'
import { useSaveWebhook } from '@/lib/queries'
import { contentTypes, eventInfo, eventTypes, presets, webhookInput } from '@/lib/webhooks'

interface Props {
  open: boolean
  onOpenChange: (open: boolean) => void
  webhook?: Webhook
}

export function WebhookDialog({ open, onOpenChange, webhook }: Props) {
  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="sm:max-w-3xl">
        {open && <WebhookForm key={webhook?.id ?? 'new'} webhook={webhook} onDone={() => onOpenChange(false)} />}
      </DialogContent>
    </Dialog>
  )
}

function toInput(w?: Webhook): WebhookInput {
  return w ? webhookInput(w) : { name: '', enabled: true, events: [...eventTypes], ...presets[0].webhook }
}

/** Vorschau mit Beispielereignis, verzögert nach der letzten Eingabe. */
function usePreview(input: WebhookInput, eventType: string) {
  const [state, setState] = useState<{ preview?: WebhookPreview; error?: string; loading: boolean }>({ loading: true })
  // Serialisiert, damit der Effekt nur bei echten Änderungen neu läuft.
  const key = JSON.stringify([input, eventType])
  useEffect(() => {
    const [w, ev] = JSON.parse(key) as [WebhookInput, string]
    let cancelled = false
    const t = setTimeout(() => {
      setState((s) => ({ ...s, loading: true }))
      api.previewWebhook(w, ev).then(
        (preview) => !cancelled && setState({ preview, loading: false }),
        (err: Error) => !cancelled && setState({ error: err.message, loading: false }),
      )
    }, 400)
    return () => {
      cancelled = true
      clearTimeout(t)
    }
  }, [key])
  return state
}

function WebhookForm({ webhook, onDone }: { webhook?: Webhook; onDone: () => void }) {
  const save = useSaveWebhook()
  const [w, setW] = useState<WebhookInput>(() => toInput(webhook))
  const [presetId, setPresetId] = useState(webhook ? '' : presets[0].id)
  const [previewEvent, setPreviewEvent] = useState<NotifyEventType>(w.events[0] ?? 'tunnel_status')
  const preview = usePreview(w, previewEvent)
  const preset = presets.find((p) => p.id === presetId)

  const update = (patch: Partial<WebhookInput>) => setW((x) => ({ ...x, ...patch }))

  function applyPreset(id: string) {
    const p = presets.find((x) => x.id === id)
    if (!p) return
    setPresetId(id)
    // Name mitwechseln, solange er leer ist oder noch einem Vorlagennamen entspricht
    const keepName = w.name && !presets.some((x) => x.label === w.name)
    update({ ...p.webhook, name: keepName ? w.name : p.label })
  }

  function submit(e: FormEvent) {
    e.preventDefault()
    save.mutate({ id: webhook?.id, input: w }, { onSuccess: onDone })
  }

  const headerRows = w.headers

  return (
    <form onSubmit={submit} className="flex min-w-0 flex-col gap-5">
      <DialogHeader>
        <DialogTitle>{webhook ? 'Webhook bearbeiten' : 'Webhook hinzufügen'}</DialogTitle>
        <DialogDescription>
          Geheimnisse nie direkt eintragen: in URL und Headern als <code className="font-mono">{'${WEBHOOK_NAME}'}</code>, im
          Body als <code className="font-mono">{'{{env "WEBHOOK_NAME"}}'}</code>. Die Werte gehören in{' '}
          <code className="font-mono">deploy/.env</code>.
        </DialogDescription>
      </DialogHeader>

      <div className="grid gap-4 sm:grid-cols-2">
        {!webhook && (
          <div className="grid gap-2">
            <Label htmlFor="wh-preset">Vorlage</Label>
            <Select value={presetId} onValueChange={applyPreset}>
              <SelectTrigger id="wh-preset" aria-label="Vorlage">
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                {presets.map((p) => (
                  <SelectItem key={p.id} value={p.id}>
                    {p.label}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
          </div>
        )}
        <div className="grid gap-2">
          <Label htmlFor="wh-name">Name</Label>
          <Input id="wh-name" value={w.name} onChange={(e) => update({ name: e.target.value })} required maxLength={100} />
        </div>
      </div>

      {preset && preset.env.length > 0 && (
        <div className="bg-muted/50 rounded-md p-3 text-xs">
          <p className="mb-1 font-medium">In deploy/.env eintragen, dann Container neu starten:</p>
          <ul className="grid gap-0.5">
            {preset.env.map((e) => (
              <li key={e.name}>
                <code className="font-mono">{e.name}=…</code> <span className="text-muted-foreground">– {e.hint}</span>
              </li>
            ))}
          </ul>
        </div>
      )}

      <div className="grid gap-2 sm:grid-cols-[8rem_1fr]">
        <div className="grid gap-2">
          <Label htmlFor="wh-method">Methode</Label>
          <Select value={w.method} onValueChange={(v) => update({ method: v as WebhookMethod })}>
            <SelectTrigger id="wh-method" aria-label="Methode">
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              {(['POST', 'PUT', 'PATCH', 'GET'] as const).map((m) => (
                <SelectItem key={m} value={m}>
                  {m}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
        </div>
        <div className="grid min-w-0 gap-2">
          <Label htmlFor="wh-url">URL</Label>
          <Input id="wh-url" className="font-mono" value={w.url} onChange={(e) => update({ url: e.target.value })} spellCheck={false} required />
        </div>
      </div>

      <div className="grid gap-2">
        <div className="flex items-center justify-between">
          <Label>Header</Label>
          <Button type="button" variant="ghost" size="sm" onClick={() => update({ headers: [...headerRows, { name: '', value: '' }] })}>
            <Plus /> Header
          </Button>
        </div>
        {headerRows.length === 0 && <p className="text-muted-foreground text-xs">Keine zusätzlichen Header.</p>}
        {headerRows.map((h, i) => (
          <div key={i} className="grid grid-cols-[1fr_1.5fr_auto] gap-2">
            <Input
              aria-label={`Header ${i + 1} Name`}
              placeholder="Name"
              className="font-mono"
              value={h.name}
              onChange={(e) => update({ headers: headerRows.map((x, j) => (j === i ? { ...x, name: e.target.value } : x)) })}
            />
            <Input
              aria-label={`Header ${i + 1} Wert`}
              placeholder="Wert, z. B. Bearer ${WEBHOOK_TOKEN}"
              className="font-mono"
              value={h.value}
              onChange={(e) => update({ headers: headerRows.map((x, j) => (j === i ? { ...x, value: e.target.value } : x)) })}
            />
            <Button
              type="button"
              variant="ghost"
              size="icon"
              aria-label={`Header ${i + 1} entfernen`}
              onClick={() => update({ headers: headerRows.filter((_, j) => j !== i) })}
            >
              <Trash2 />
            </Button>
          </div>
        ))}
      </div>

      {w.method !== 'GET' && (
        <div className="grid gap-2">
          <div className="flex flex-wrap items-center justify-between gap-2">
            <Label htmlFor="wh-body">Body-Template</Label>
            <Select value={w.content_type} onValueChange={(v) => update({ content_type: v })}>
              <SelectTrigger className="h-8 w-auto text-xs" aria-label="Content-Type">
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                {contentTypes.map((c) => (
                  <SelectItem key={c} value={c}>
                    {c}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
          </div>
          <Textarea
            id="wh-body"
            className="min-h-32 font-mono text-xs"
            spellCheck={false}
            placeholder="Leer = Standard-JSON mit type, title, message, priority, time, data"
            value={w.body_template}
            onChange={(e) => update({ body_template: e.target.value })}
          />
          <p className="text-muted-foreground text-xs">
            Verfügbar: <code className="font-mono">.Type .Title .Message .Priority .Time .Data.&lt;feld&gt;</code> ·
            Funktionen: <code className="font-mono">json</code> (für JSON-Werte), <code className="font-mono">env</code>,{' '}
            <code className="font-mono">printf mul upper lower urlquery</code>
          </p>
        </div>
      )}

      <div className="grid gap-3">
        <Label>Senden bei</Label>
        <div className="grid gap-3 sm:grid-cols-2">
          {eventTypes.map((t) => (
            <div key={t} className="flex items-start gap-3">
              <Switch
                id={`wh-ev-${t}`}
                checked={w.events.includes(t)}
                onCheckedChange={(on) => update({ events: on ? [...w.events, t] : w.events.filter((x) => x !== t) })}
              />
              <div className="grid gap-0.5">
                <Label htmlFor={`wh-ev-${t}`}>{eventInfo[t].label}</Label>
                <p className="text-muted-foreground text-xs">{eventInfo[t].hint}</p>
              </div>
            </div>
          ))}
        </div>
      </div>

      <div className="flex items-center gap-3">
        <Switch id="wh-enabled" checked={w.enabled} onCheckedChange={(on) => update({ enabled: on })} />
        <Label htmlFor="wh-enabled">Aktiv</Label>
      </div>

      <div className="grid gap-2 rounded-md border p-3">
        <div className="flex flex-wrap items-center justify-between gap-2">
          <span className="text-sm font-medium">Vorschau</span>
          <Select value={previewEvent} onValueChange={(v) => setPreviewEvent(v as NotifyEventType)}>
            <SelectTrigger className="h-8 w-auto text-xs" aria-label="Beispielereignis">
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              {eventTypes.map((t) => (
                <SelectItem key={t} value={t}>
                  Beispiel: {eventInfo[t].label}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
        </div>
        {preview.error ? (
          <p className="text-destructive text-xs break-words" role="alert">
            {preview.error}
          </p>
        ) : preview.preview ? (
          <pre className="bg-muted max-h-56 overflow-auto rounded p-2 font-mono text-xs whitespace-pre-wrap break-all">
            {`${preview.preview.method} ${preview.preview.url}\n`}
            {Object.entries(preview.preview.headers).map(([k, v]) => `${k}: ${v.join(', ')}\n`)}
            {preview.preview.body && `\n${preview.preview.body}`}
          </pre>
        ) : (
          <Loader2 className="text-muted-foreground size-4 animate-spin" />
        )}
        <p className="text-muted-foreground text-xs">Platzhalter bleiben in der Vorschau sichtbar – Werte werden nie angezeigt.</p>
      </div>

      {save.isError && (
        <p className="text-destructive text-sm break-words" role="alert">
          {save.error.message}
        </p>
      )}

      <DialogFooter>
        <Button type="button" variant="outline" onClick={onDone}>
          Abbrechen
        </Button>
        <Button type="submit" disabled={save.isPending}>
          {save.isPending && <Loader2 className="animate-spin" />}
          {webhook ? 'Speichern' : 'Hinzufügen'}
        </Button>
      </DialogFooter>
    </form>
  )
}
