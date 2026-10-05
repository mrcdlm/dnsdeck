import type { TFunction } from 'i18next'
import {
  AlertTriangle,
  ArrowDown,
  ArrowUp,
  Languages,
  Check,
  Info as InfoIcon,
  Loader2,
  Pencil,
  Plus,
  Send,
  Timer,
  Trash2,
  Wifi,
  X,
} from 'lucide-react'
import { useEffect, useState, type ReactNode } from 'react'
import { Trans, useTranslation } from 'react-i18next'

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
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card'
import { Label } from '@/components/ui/label'
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select'
import { Skeleton } from '@/components/ui/skeleton'
import { Switch } from '@/components/ui/switch'
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
import { formatInterval } from '@/lib/speedtest'
import { webhookInput } from '@/lib/webhooks'

const ipIntervals = [60, 120, 300, 600, 900, 1800, 3600]
const tunnelIntervals = [30, 60, 120, 300, 600]
const probeIntervals = [60, 120, 300, 600, 900, 1800, 3600]
const warnDayOptions = [7, 14, 21, 30, 60]
// 0 = nur manuell
const speedtestIntervals = [0, 3600, 3 * 3600, 6 * 3600, 12 * 3600, 86400, 7 * 86400]

const sourceInfo: Record<string, string> = {
  cloudflare: 'Cloudflare (1.1.1.1/cdn-cgi/trace)',
  ipify: 'ipify (api.ipify.org / api6.ipify.org)',
  icanhazip: 'icanhazip.com',
}

function formatSeconds(t: TFunction, s: number): string {
  if (s % 3600 === 0) return t('interval.hours', { count: s / 3600 })
  if (s % 60 === 0) return t('interval.minutes', { count: s / 60 })
  return t('interval.seconds', { count: s })
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
  const { t } = useTranslation()
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
            {formatSeconds(t, s)}
          </SelectItem>
        ))}
      </SelectContent>
    </Select>
  )
}

function Section({
  icon,
  title,
  description,
  children,
}: {
  icon: ReactNode
  title: string
  description: string
  children: ReactNode
}) {
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

function SettingRow({ id, label, hint, children }: { id: string; label: string; hint: string; children: ReactNode }) {
  return (
    <div className="flex flex-col gap-2 sm:flex-row sm:items-center sm:justify-between">
      <div className="grid gap-1">
        <Label htmlFor={id}>{label}</Label>
        <p className="text-muted-foreground text-xs">{hint}</p>
      </div>
      {children}
    </div>
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
  const { t } = useTranslation()
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
      <Section icon={<Timer className="size-5" />} title={t('settings.intervals')} description={t('settings.intervalsHint')}>
        <SettingRow id="ip-interval" label={t('settings.ipInterval')} hint={t('settings.ipIntervalHint')}>
          <IntervalSelect
            id="ip-interval"
            value={draft.ip_check_interval_seconds}
            presets={ipIntervals}
            onChange={(v) => update({ ip_check_interval_seconds: v })}
          />
        </SettingRow>
        <SettingRow id="tunnel-interval" label={t('settings.tunnelInterval')} hint={t('settings.tunnelIntervalHint')}>
          <IntervalSelect
            id="tunnel-interval"
            value={draft.tunnel_interval_seconds}
            presets={tunnelIntervals}
            onChange={(v) => update({ tunnel_interval_seconds: v })}
          />
        </SettingRow>
        <SettingRow id="probe-interval" label={t('settings.probeInterval')} hint={t('settings.probeIntervalHint')}>
          <IntervalSelect
            id="probe-interval"
            value={draft.probe_interval_seconds}
            presets={probeIntervals}
            onChange={(v) => update({ probe_interval_seconds: v })}
          />
        </SettingRow>
        <SettingRow
          id="speedtest-interval"
          label={t('settings.speedtestInterval')}
          hint={t('settings.speedtestIntervalHint')}
        >
          <Select
            value={String(draft.speedtest_interval_seconds)}
            onValueChange={(v) => update({ speedtest_interval_seconds: Number(v) })}
          >
            <SelectTrigger id="speedtest-interval" className="w-full sm:w-48">
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              {(speedtestIntervals.includes(draft.speedtest_interval_seconds)
                ? speedtestIntervals
                : [...speedtestIntervals, draft.speedtest_interval_seconds].sort((a, b) => a - b)
              ).map((s) => (
                <SelectItem key={s} value={String(s)}>
                  {s === 0 ? t('settings.speedtestOff') : formatInterval(t, s)}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
        </SettingRow>
        <SettingRow id="tls-warn-days" label={t('settings.tlsWarnDays')} hint={t('settings.tlsWarnDaysHint')}>
          <Select
            value={String(draft.tls_warn_days)}
            onValueChange={(v) => update({ tls_warn_days: Number(v) })}
          >
            <SelectTrigger id="tls-warn-days" className="w-full sm:w-48">
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              {(warnDayOptions.includes(draft.tls_warn_days)
                ? warnDayOptions
                : [...warnDayOptions, draft.tls_warn_days].sort((a, b) => a - b)
              ).map((d) => (
                <SelectItem key={d} value={String(d)}>
                  {t('settings.days', { count: d })}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
        </SettingRow>
      </Section>

      <Section
        icon={<Wifi className="size-5" />}
        title={t('settings.sources')}
        description={t('settings.sourcesHint', { min: minSources })}
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
              <Button
                type="button"
                variant="ghost"
                size="icon"
                aria-label={t('settings.moveUp', { name: s.name })}
                disabled={i === 0}
                onClick={() => moveSource(i, -1)}
              >
                <ArrowUp />
              </Button>
              <Button
                type="button"
                variant="ghost"
                size="icon"
                aria-label={t('settings.moveDown', { name: s.name })}
                disabled={i === draft.ip_sources.length - 1}
                onClick={() => moveSource(i, 1)}
              >
                <ArrowDown />
              </Button>
            </li>
          ))}
        </ul>
        {enabledSources < minSources && (
          <p className="text-destructive text-sm">{t('settings.sourcesMin', { min: minSources })}</p>
        )}
      </Section>

      <Section icon={<Languages className="size-5" />} title={t('settings.notifications')} description={t('settings.notificationsHint')}>
        <SettingRow id="notify-language" label={t('settings.notifyLanguage')} hint={t('settings.notifyLanguageHint')}>
          <Select
            value={draft.notify_language}
            onValueChange={(v) => update({ notify_language: v as SettingsData['notify_language'] })}
          >
            <SelectTrigger id="notify-language" className="w-full sm:w-48">
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              <SelectItem value="de">{t('prefs.languageName.de')}</SelectItem>
              <SelectItem value="en">{t('prefs.languageName.en')}</SelectItem>
            </SelectContent>
          </Select>
        </SettingRow>
      </Section>

      <div className="bg-background/90 sticky bottom-0 -mx-4 flex flex-wrap items-center justify-end gap-3 border-t px-4 py-3 backdrop-blur md:-mx-8 md:px-8">
        {save.isError && (
          <p className="text-destructive mr-auto text-sm" role="alert">
            {save.error.message}
          </p>
        )}
        {saved && !dirty && (
          <span className="text-success mr-auto flex items-center gap-1 text-sm">
            <Check className="size-4" /> {t('settings.saved')}
          </span>
        )}
        <Button
          type="button"
          variant="outline"
          disabled={!dirty || save.isPending}
          onClick={() => (setDraft(initial), setSaved(false))}
        >
          {t('settings.discard')}
        </Button>
        <Button type="submit" disabled={!dirty || save.isPending || enabledSources < minSources}>
          {save.isPending && <Loader2 className="animate-spin" />}
          {t('common.save')}
        </Button>
      </div>
    </form>
  )
}

function WebhookCard({ webhook, onEdit, onDelete }: { webhook: Webhook; onEdit: () => void; onDelete: () => void }) {
  const { t } = useTranslation()
  const test = useTestWebhook()
  const save = useSaveWebhook()
  const now = useNow()
  const { id, missing_env, last_error, last_sent_at } = webhook

  // Testergebnis nur kurz zeigen – danach gilt wieder der gespeicherte Status.
  const { reset: resetTest, data: testData } = test
  useEffect(() => {
    if (!testData) return
    const timer = setTimeout(resetTest, 8000)
    return () => clearTimeout(timer)
  }, [testData, resetTest])

  return (
    <li className="flex flex-col gap-2 px-3 py-3" data-webhook={webhook.name}>
      <div className="flex flex-wrap items-center gap-2">
        <Switch
          checked={webhook.enabled}
          aria-label={t('webhooks.activeLabel', { name: webhook.name })}
          disabled={save.isPending}
          onCheckedChange={(on) => save.mutate({ id, input: { ...webhookInput(webhook), enabled: on } })}
        />
        <span className="font-medium">{webhook.name}</span>
        {!webhook.enabled && <Badge variant="outline">{t('webhooks.inactive')}</Badge>}
        <div className="ml-auto flex gap-1">
          <Button type="button" variant="ghost" size="sm" onClick={() => test.mutate(id)} disabled={test.isPending}>
            {test.isPending ? <Loader2 className="animate-spin" /> : <Send />}
            {t('webhooks.test')}
          </Button>
          <Button
            type="button"
            variant="ghost"
            size="icon"
            aria-label={t('webhooks.editLabel', { name: webhook.name })}
            onClick={onEdit}
          >
            <Pencil />
          </Button>
          <Button
            type="button"
            variant="ghost"
            size="icon"
            aria-label={t('webhooks.deleteLabel', { name: webhook.name })}
            onClick={onDelete}
          >
            <Trash2 />
          </Button>
        </div>
      </div>
      <p className="text-muted-foreground font-mono text-xs break-all">
        {webhook.method} {webhook.url}
      </p>
      <div className="flex flex-wrap gap-1">
        {webhook.events.length === 0 ? (
          <span className="text-warning text-xs">{t('webhooks.noEvents')}</span>
        ) : (
          webhook.events.map((e) => (
            <Badge key={e} variant="secondary">
              {t(`events.${e}.label`)}
            </Badge>
          ))
        )}
      </div>
      {missing_env.length > 0 && (
        <p className="text-warning flex items-start gap-1.5 text-xs">
          <AlertTriangle className="mt-0.5 size-3.5 shrink-0" />
          <span>
            <Trans
              i18nKey="webhooks.missingEnv"
              values={{ names: missing_env.join(', ') }}
              components={{ code: <code className="font-mono" /> }}
            />
          </span>
        </p>
      )}
      {save.isError && (
        <p className="text-destructive text-xs break-words" role="alert">
          {t('webhooks.saveFailed', { error: save.error.message })}
        </p>
      )}
      <p className="text-xs">
        {test.data ? (
          test.data.ok ? (
            <span className="text-success flex items-center gap-1">
              <Check className="size-3.5" /> {t('webhooks.testDelivered')}
            </span>
          ) : (
            <span className="text-destructive flex items-center gap-1 break-words">
              <X className="size-3.5 shrink-0" /> {test.data.error}
            </span>
          )
        ) : last_error ? (
          <span className="text-destructive break-words">{t('webhooks.lastFailed', { error: last_error })}</span>
        ) : last_sent_at ? (
          <span className="text-muted-foreground" title={absoluteTime(last_sent_at)}>
            {t('webhooks.lastSent', { time: relativeTime(last_sent_at, now) })}
          </span>
        ) : (
          <span className="text-muted-foreground">{t('webhooks.nothingSent')}</span>
        )}
      </p>
    </li>
  )
}

function Webhooks() {
  const { t } = useTranslation()
  const webhooks = useWebhooks()
  const del = useDeleteWebhook()
  const [dialog, setDialog] = useState<{ open: boolean; webhook?: Webhook }>({ open: false })
  const [toDelete, setToDelete] = useState<Webhook>()
  const list = webhooks.data ?? []

  return (
    <Section icon={<Send className="size-5" />} title={t('webhooks.title')} description={t('webhooks.subtitle')}>
      {webhooks.isPending ? (
        <Skeleton className="h-16" />
      ) : webhooks.isError ? (
        <p className="text-destructive text-sm">{t('webhooks.loadError', { error: webhooks.error.message })}</p>
      ) : list.length === 0 ? (
        <p className="text-muted-foreground text-sm">{t('webhooks.empty')}</p>
      ) : (
        <ul className="divide-y rounded-md border">
          {list.map((w) => (
            <WebhookCard
              key={w.id}
              webhook={w}
              onEdit={() => setDialog({ open: true, webhook: w })}
              onDelete={() => setToDelete(w)}
            />
          ))}
        </ul>
      )}
      <div>
        <Button type="button" variant="outline" onClick={() => setDialog({ open: true })}>
          <Plus /> {t('webhooks.add')}
        </Button>
      </div>

      <WebhookDialog
        open={dialog.open}
        webhook={dialog.webhook}
        onOpenChange={(open) => setDialog((d) => ({ ...d, open }))}
      />

      <AlertDialog open={toDelete !== undefined} onOpenChange={(open) => !open && setToDelete(undefined)}>
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>{t('webhooks.deleteTitle')}</AlertDialogTitle>
            <AlertDialogDescription>{t('webhooks.deleteHint', { name: toDelete?.name })}</AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel>{t('common.cancel')}</AlertDialogCancel>
            <AlertDialogAction
              onClick={(e) => {
                e.preventDefault()
                if (toDelete) del.mutate(toDelete.id, { onSuccess: () => setToDelete(undefined) })
              }}
              disabled={del.isPending}
            >
              {t('common.delete')}
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
  const { t } = useTranslation()
  const info = useInfo()
  const d = info.data
  const set = t('system.set')
  const missing = <span className="text-warning">{t('system.missing')}</span>
  return (
    <Section icon={<InfoIcon className="size-5" />} title={t('system.title')} description={t('system.hint')}>
      {!d ? (
        <Skeleton className="h-20" />
      ) : (
        <dl className="grid gap-2 text-sm">
          <InfoRow label={t('system.version')}>
            <span className="font-mono">{d.version}</span>
          </InfoRow>
          <InfoRow label={t('system.cfToken')}>{d.cf_token_set ? set : missing}</InfoRow>
          <InfoRow label={t('system.cfAccount')}>{d.cf_account_set ? set : missing}</InfoRow>
          <InfoRow label={t('system.dataDir')}>
            <span className="font-mono">{d.data_dir}</span>
          </InfoRow>
        </dl>
      )}
    </Section>
  )
}

export function Settings() {
  const { t } = useTranslation()
  const settings = useSettings()
  const [saved, setSaved] = useState(false)

  return (
    <div className="flex flex-col gap-6">
      <div>
        <h1 className="text-2xl font-semibold tracking-tight">{t('nav.settings')}</h1>
        <p className="text-muted-foreground text-sm">{t('settings.subtitle')}</p>
      </div>
      {settings.isPending ? (
        <Skeleton className="h-96" />
      ) : settings.isError ? (
        <p className="text-destructive text-sm">{t('settings.loadError', { error: settings.error.message })}</p>
      ) : (
        // key: nach dem Speichern (oder Änderung in anderem Tab) neu initialisieren
        <SettingsForm key={JSON.stringify(settings.data)} initial={settings.data} saved={saved} setSaved={setSaved} />
      )}
      <Webhooks />
      <SystemInfo />
    </div>
  )
}
