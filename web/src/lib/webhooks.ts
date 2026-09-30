import type { NotifyEventType, Webhook, WebhookInput } from './api'

export const eventInfo: Record<NotifyEventType, { label: string; hint: string }> = {
  ip_change: { label: 'IP-Wechsel', hint: 'Die öffentliche IPv4 oder IPv6 hat sich geändert.' },
  update_failed: { label: 'DNS-Update fehlgeschlagen', hint: 'Einmal je neuem Fehler, nicht bei jeder Wiederholung.' },
  update_recovered: { label: 'DNS-Update wieder OK', hint: 'Ein zuvor fehlerhafter Record ist wieder aktuell.' },
  tunnel_status: { label: 'Tunnel-Statuswechsel', hint: 'z. B. verbunden → getrennt.' },
}

export const eventTypes = Object.keys(eventInfo) as NotifyEventType[]

export interface Preset {
  id: string
  label: string
  /** Kurze Erklärung, welche Env-Variablen gesetzt werden müssen */
  env: { name: string; hint: string }[]
  webhook: Omit<WebhookInput, 'name' | 'enabled' | 'events'>
}

const json = 'application/json'

/**
 * Vorlagen sind nur Startpunkte: technisch ist jeder Webhook gleich (Methode,
 * URL, Header, Body-Template). Geheimnisse stehen als ${WEBHOOK_…} bzw.
 * {{env "WEBHOOK_…"}} darin; die Werte kommen in deploy/.env.
 */
export const presets: Preset[] = [
  {
    id: 'generic',
    label: 'Allgemein (JSON)',
    env: [],
    webhook: { method: 'POST', url: 'https://example.com/hook', headers: [], content_type: json, body_template: '' },
  },
  {
    id: 'ntfy',
    label: 'ntfy',
    env: [{ name: 'WEBHOOK_NTFY_TOPIC', hint: 'Topic-Name (wie ein Passwort behandeln)' }],
    webhook: {
      method: 'POST',
      url: 'https://ntfy.sh',
      headers: [],
      content_type: json,
      body_template: `{
  "topic": {{json (env "WEBHOOK_NTFY_TOPIC")}},
  "title": {{json .Title}},
  "message": {{json .Message}},
  "priority": {{.Priority}}
}`,
    },
  },
  {
    id: 'gotify',
    label: 'Gotify',
    env: [{ name: 'WEBHOOK_GOTIFY_TOKEN', hint: 'App-Token aus Gotify' }],
    webhook: {
      method: 'POST',
      url: 'https://gotify.example.com/message',
      headers: [{ name: 'X-Gotify-Key', value: '${WEBHOOK_GOTIFY_TOKEN}' }],
      content_type: json,
      // Gotify-Skala 0–10 (dnsdeck: 2–4)
      body_template: `{
  "title": {{json .Title}},
  "message": {{json .Message}},
  "priority": {{mul .Priority 2}}
}`,
    },
  },
  {
    id: 'discord',
    label: 'Discord',
    env: [{ name: 'WEBHOOK_DISCORD_URL', hint: 'komplette Webhook-URL aus den Kanaleinstellungen' }],
    webhook: {
      method: 'POST',
      url: '${WEBHOOK_DISCORD_URL}',
      headers: [],
      content_type: json,
      body_template: `{"content": {{json (printf "**%s**\\n%s" .Title .Message)}}}`,
    },
  },
  {
    id: 'slack',
    label: 'Slack',
    env: [{ name: 'WEBHOOK_SLACK_URL', hint: 'Incoming-Webhook-URL' }],
    webhook: {
      method: 'POST',
      url: '${WEBHOOK_SLACK_URL}',
      headers: [],
      content_type: json,
      body_template: `{"text": {{json (printf "*%s*\\n%s" .Title .Message)}}}`,
    },
  },
  {
    id: 'telegram',
    label: 'Telegram',
    env: [
      { name: 'WEBHOOK_TELEGRAM_TOKEN', hint: 'Bot-Token von @BotFather' },
      { name: 'WEBHOOK_TELEGRAM_CHAT_ID', hint: 'Chat-ID des Empfängers' },
    ],
    webhook: {
      method: 'POST',
      url: 'https://api.telegram.org/bot${WEBHOOK_TELEGRAM_TOKEN}/sendMessage',
      headers: [],
      content_type: json,
      body_template: `{
  "chat_id": {{json (env "WEBHOOK_TELEGRAM_CHAT_ID")}},
  "text": {{json (printf "%s\\n%s" .Title .Message)}}
}`,
    },
  },
  {
    id: 'homeassistant',
    label: 'Home Assistant',
    env: [{ name: 'WEBHOOK_HA_ID', hint: 'Webhook-ID der Automation' }],
    webhook: {
      method: 'POST',
      url: 'http://homeassistant.local:8123/api/webhook/${WEBHOOK_HA_ID}',
      headers: [],
      content_type: json,
      body_template: '',
    },
  },
]

/** Bearbeitbare Felder eines gespeicherten Webhooks. */
export function webhookInput(w: Webhook): WebhookInput {
  const { name, enabled, method, url, headers, content_type, body_template, events } = w
  return { name, enabled, method, url, headers, content_type, body_template, events }
}

export const contentTypes = ['application/json', 'text/plain; charset=utf-8', 'application/x-www-form-urlencoded']
