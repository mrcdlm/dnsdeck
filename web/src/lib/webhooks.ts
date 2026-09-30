import type de from '@/locales/de.json'

import type { NotifyEventType, Webhook, WebhookInput } from './api'

/** Env-Variablen der Vorlagen – jede hat eine Erklärung unter webhooks.env.<name> */
export type PresetEnv = keyof (typeof de)['webhooks']['env']

// Beschriftungen: events.<typ>.label / .hint (locales)
export const eventTypes: NotifyEventType[] = ['ip_change', 'update_failed', 'update_recovered', 'tunnel_status']

export interface Preset {
  id: string
  /** Anzeigename; Produktnamen werden nicht übersetzt ("generic" über locales) */
  label: string
  /** Env-Variablen, die gesetzt werden müssen; Erklärung: webhooks.env.<name> */
  env: PresetEnv[]
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
    label: '',
    env: [],
    webhook: { method: 'POST', url: 'https://example.com/hook', headers: [], content_type: json, body_template: '' },
  },
  {
    id: 'ntfy',
    label: 'ntfy',
    env: ['WEBHOOK_NTFY_TOPIC'],
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
    env: ['WEBHOOK_GOTIFY_TOKEN'],
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
    env: ['WEBHOOK_DISCORD_URL'],
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
    env: ['WEBHOOK_SLACK_URL'],
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
    env: ['WEBHOOK_TELEGRAM_TOKEN', 'WEBHOOK_TELEGRAM_CHAT_ID'],
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
    env: ['WEBHOOK_HA_ID'],
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
