import { Loader2, Plus, Trash2 } from 'lucide-react'
import { useEffect, useState, type FormEvent } from 'react'
import { Trans, useTranslation } from 'react-i18next'

import { Button } from '@/components/ui/button'
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from '@/components/ui/dialog'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select'
import { Switch } from '@/components/ui/switch'
import { Textarea } from '@/components/ui/textarea'
import { api, type NotifyEventType, type Webhook, type WebhookInput, type WebhookMethod, type WebhookPreview } from '@/lib/api'
import { useSaveWebhook } from '@/lib/queries'
import { contentTypes, eventTypes, presets, webhookInput, type Preset } from '@/lib/webhooks'

// Template-Syntax nie in Übersetzungstexte schreiben (i18next nutzt ebenfalls
// {{…}}) – sie wird als Wert eingesetzt und nicht ausgewertet.
const SYNTAX = {
  urlVar: '${WEBHOOK_NAME}',
  bodyVar: '{{env "WEBHOOK_NAME"}}',
  fields: '.Type .Title .Message .Priority .Time .Data.<field>',
  funcs: 'printf mul upper lower urlquery',
  headerExample: 'Bearer ${WEBHOOK_TOKEN}',
}

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
    const timer = setTimeout(() => {
      setState((s) => ({ ...s, loading: true }))
      api.previewWebhook(w, ev).then(
        (preview) => !cancelled && setState({ preview, loading: false }),
        (err: Error) => !cancelled && setState({ error: err.message, loading: false }),
      )
    }, 400)
    return () => {
      cancelled = true
      clearTimeout(timer)
    }
  }, [key])
  return state
}

function WebhookForm({ webhook, onDone }: { webhook?: Webhook; onDone: () => void }) {
  const { t } = useTranslation()
  const save = useSaveWebhook()
  const [w, setW] = useState<WebhookInput>(() => toInput(webhook))
  const [presetId, setPresetId] = useState(webhook ? '' : presets[0].id)
  const [previewEvent, setPreviewEvent] = useState<NotifyEventType>(w.events[0] ?? 'tunnel_status')
  const preview = usePreview(w, previewEvent)
  const preset = presets.find((p) => p.id === presetId)
  const presetLabel = (p: Preset) => p.label || t('webhooks.presetGeneric')

  const update = (patch: Partial<WebhookInput>) => setW((x) => ({ ...x, ...patch }))

  function applyPreset(id: string) {
    const p = presets.find((x) => x.id === id)
    if (!p) return
    setPresetId(id)
    // Name mitwechseln, solange er leer ist oder noch einem Vorlagennamen entspricht
    const keepName = w.name && !presets.some((x) => presetLabel(x) === w.name)
    update({ ...p.webhook, name: keepName ? w.name : presetLabel(p) })
  }

  function submit(e: FormEvent) {
    e.preventDefault()
    save.mutate({ id: webhook?.id, input: w }, { onSuccess: onDone })
  }

  const headerRows = w.headers
  const code = <code className="font-mono" />

  return (
    <form onSubmit={submit} className="flex min-w-0 flex-col gap-5">
      <DialogHeader>
        <DialogTitle>{webhook ? t('webhooks.editTitle') : t('webhooks.addTitle')}</DialogTitle>
        <DialogDescription>
          <Trans i18nKey="webhooks.secretsHint" values={SYNTAX} components={{ code }} />
        </DialogDescription>
      </DialogHeader>

      <div className="grid gap-4 sm:grid-cols-2">
        {!webhook && (
          <div className="grid gap-2">
            <Label htmlFor="wh-preset">{t('webhooks.preset')}</Label>
            <Select value={presetId} onValueChange={applyPreset}>
              <SelectTrigger id="wh-preset" aria-label={t('webhooks.preset')}>
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                {presets.map((p) => (
                  <SelectItem key={p.id} value={p.id}>
                    {presetLabel(p)}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
          </div>
        )}
        <div className="grid gap-2">
          <Label htmlFor="wh-name">{t('webhooks.name')}</Label>
          <Input id="wh-name" value={w.name} onChange={(e) => update({ name: e.target.value })} required maxLength={100} />
        </div>
      </div>

      {preset && preset.env.length > 0 && (
        <div className="bg-muted/50 rounded-md p-3 text-xs">
          <p className="mb-1 font-medium">{t('webhooks.envIntro')}</p>
          <ul className="grid gap-0.5">
            {preset.env.map((name) => (
              <li key={name}>
                <code className="font-mono">{name}=…</code>{' '}
                <span className="text-muted-foreground">– {t(`webhooks.env.${name}`)}</span>
              </li>
            ))}
          </ul>
        </div>
      )}

      <div className="grid gap-2 sm:grid-cols-[8rem_1fr]">
        <div className="grid gap-2">
          <Label htmlFor="wh-method">{t('webhooks.method')}</Label>
          <Select value={w.method} onValueChange={(v) => update({ method: v as WebhookMethod })}>
            <SelectTrigger id="wh-method" aria-label={t('webhooks.method')}>
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
          <Input
            id="wh-url"
            className="font-mono"
            value={w.url}
            onChange={(e) => update({ url: e.target.value })}
            spellCheck={false}
            required
          />
        </div>
      </div>

      <div className="grid gap-2">
        <div className="flex items-center justify-between">
          <Label>Header</Label>
          <Button
            type="button"
            variant="ghost"
            size="sm"
            onClick={() => update({ headers: [...headerRows, { name: '', value: '' }] })}
          >
            <Plus /> Header
          </Button>
        </div>
        {headerRows.length === 0 && <p className="text-muted-foreground text-xs">{t('webhooks.noHeaders')}</p>}
        {headerRows.map((h, i) => (
          <div key={i} className="grid grid-cols-[1fr_1.5fr_auto] gap-2">
            <Input
              aria-label={t('webhooks.headerName', { n: i + 1 })}
              placeholder={t('webhooks.name')}
              className="font-mono"
              value={h.name}
              onChange={(e) => update({ headers: headerRows.map((x, j) => (j === i ? { ...x, name: e.target.value } : x)) })}
            />
            <Input
              aria-label={t('webhooks.headerValue', { n: i + 1 })}
              placeholder={t('webhooks.headerValuePlaceholder', { example: SYNTAX.headerExample })}
              className="font-mono"
              value={h.value}
              onChange={(e) => update({ headers: headerRows.map((x, j) => (j === i ? { ...x, value: e.target.value } : x)) })}
            />
            <Button
              type="button"
              variant="ghost"
              size="icon"
              aria-label={t('webhooks.headerRemove', { n: i + 1 })}
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
            <Label htmlFor="wh-body">{t('webhooks.body')}</Label>
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
            placeholder={t('webhooks.bodyPlaceholder')}
            value={w.body_template}
            onChange={(e) => update({ body_template: e.target.value })}
          />
          <p className="text-muted-foreground text-xs">
            <Trans i18nKey="webhooks.bodyHelp" values={SYNTAX} components={{ code }} />
          </p>
        </div>
      )}

      <div className="grid gap-3">
        <Label>{t('webhooks.sendOn')}</Label>
        <div className="grid gap-3 sm:grid-cols-2">
          {eventTypes.map((ev) => (
            <div key={ev} className="flex items-start gap-3">
              <Switch
                id={`wh-ev-${ev}`}
                checked={w.events.includes(ev)}
                onCheckedChange={(on) => update({ events: on ? [...w.events, ev] : w.events.filter((x) => x !== ev) })}
              />
              <div className="grid gap-0.5">
                <Label htmlFor={`wh-ev-${ev}`}>{t(`events.${ev}.label`)}</Label>
                <p className="text-muted-foreground text-xs">{t(`events.${ev}.hint`)}</p>
              </div>
            </div>
          ))}
        </div>
      </div>

      <div className="flex items-center gap-3">
        <Switch id="wh-enabled" checked={w.enabled} onCheckedChange={(on) => update({ enabled: on })} />
        <Label htmlFor="wh-enabled">{t('webhooks.active')}</Label>
      </div>

      <div className="grid gap-2 rounded-md border p-3">
        <div className="flex flex-wrap items-center justify-between gap-2">
          <span className="text-sm font-medium">{t('webhooks.preview')}</span>
          <Select value={previewEvent} onValueChange={(v) => setPreviewEvent(v as NotifyEventType)}>
            <SelectTrigger className="h-8 w-auto text-xs" aria-label={t('webhooks.sampleEvent')}>
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              {eventTypes.map((ev) => (
                <SelectItem key={ev} value={ev}>
                  {t('webhooks.sample', { event: t(`events.${ev}.label`) })}
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
          <pre className="bg-muted max-h-56 overflow-auto rounded p-2 font-mono text-xs break-all whitespace-pre-wrap">
            {`${preview.preview.method} ${preview.preview.url}\n`}
            {Object.entries(preview.preview.headers).map(([k, v]) => `${k}: ${v.join(', ')}\n`)}
            {preview.preview.body && `\n${preview.preview.body}`}
          </pre>
        ) : (
          <Loader2 className="text-muted-foreground size-4 animate-spin" />
        )}
        <p className="text-muted-foreground text-xs">{t('webhooks.previewHint')}</p>
      </div>

      {save.isError && (
        <p className="text-destructive text-sm break-words" role="alert">
          {save.error.message}
        </p>
      )}

      <DialogFooter>
        <Button type="button" variant="outline" onClick={onDone}>
          {t('common.cancel')}
        </Button>
        <Button type="submit" disabled={save.isPending}>
          {save.isPending && <Loader2 className="animate-spin" />}
          {webhook ? t('common.save') : t('common.add')}
        </Button>
      </DialogFooter>
    </form>
  )
}
