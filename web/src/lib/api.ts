import { currentLang } from '@/i18n'

export type Family = 'ipv4' | 'ipv6'
export type IPStatus = 'ok' | 'unconfirmed' | 'unavailable' | 'pending'

export interface Observation {
  source: string
  ip?: string
  error?: string
}

/** Netz/Anbieter einer öffentlichen Adresse (Autonomes System). */
export interface ISPInfo {
  asn: number
  name?: string
  prefix?: string
  country?: string
  registry?: string
  hostname?: string
}

export type BlocklistStatus = 'clean' | 'listed' | 'policy' | 'refused' | 'error'

/** Ergebnis einer Sperrliste (DNSBL). */
export interface BlocklistEntry {
  name: string
  zone: string
  status: BlocklistStatus
  codes?: string[]
  /** bekannte Teillisten, z. B. "SBL, XBL" (Spamhaus) */
  detail?: string
}

/** Sperrlisten-Prüfung der öffentlichen IPv4. */
export interface Blocklist {
  checked_at: string
  status: 'clean' | 'listed' | 'unknown'
  lists: BlocklistEntry[]
}

export interface FamilyState {
  ip?: string
  since?: string
  status: IPStatus
  message?: string
  votes: number
  responses: number
  sources: Observation[]
  isp?: ISPInfo
  blocklist?: Blocklist
}

export interface IPState {
  ipv4: FamilyState
  ipv6: FamilyState
  last_checked?: string
}

export interface IPChange {
  id: number
  family: Family
  ip: string
  previous_ip?: string
  detected_at: string
}

export type RecordType = 'A' | 'AAAA'
export type RecordStatus = 'pending' | 'ok' | 'error' | 'skipped' | 'paused'

export interface DnsRecord {
  id: number
  provider: string
  zone_id: string
  zone_name: string
  name: string
  type: RecordType
  proxied: boolean
  ttl: number
  enabled: boolean
  /** Proxy/TTL in dnsdeck geändert, aber noch nicht zu Cloudflare übertragen */
  settings_pending: boolean
  current_ip?: string
  status: RecordStatus
  message?: string
  /** Ergebnis der letzten DNS-Verbreitungsprüfung */
  propagation?: Propagation
  /** Die Prüfung wiederholt sich gerade (nach einer Änderung) */
  propagation_watching?: boolean
  last_checked_at?: string
  last_changed_at?: string
  created_at: string
  updated_at: string
}

export type PropagationStatus = 'propagated' | 'partial' | 'pending' | 'error'

export interface PropagationAnswer {
  resolver: string
  addr?: string
  authoritative?: boolean
  values?: string[]
  /** verbleibende Cache-Zeit in Sekunden */
  ttl: number
  rcode?: string
  error?: 'timeout' | 'failed'
  match: boolean
}

export interface Propagation {
  checked_at: string
  type: RecordType
  /** erwartete IP; leer bei proxied */
  expected?: string
  proxied: boolean
  status: PropagationStatus
  matching: number
  total: number
  errors: number
  /** spätestens nach so vielen Sekunden ab checked_at überall aktuell */
  max_wait?: number
  answers: PropagationAnswer[]
}

export interface RecordInput {
  zone_id: string
  name: string
  type: RecordType
  proxied: boolean
  ttl: number
  enabled: boolean
  /** Erreichbarkeit von https://<name>/ prüfen (fehlt = unverändert) */
  probe?: boolean
}

export interface Zone {
  id: string
  name: string
}

export type UpdateResult = 'created' | 'adopted' | 'updated' | 'recovered' | 'error'
export type UpdateTrigger = 'scheduled' | 'manual' | 'record_saved'

export interface UpdateLogEntry {
  id: number
  record_id?: number
  record_name: string
  record_type: RecordType
  trigger: UpdateTrigger
  result: UpdateResult
  old_ip?: string
  new_ip?: string
  message?: string
  created_at: string
}

export type TunnelStatus = 'healthy' | 'degraded' | 'down' | 'inactive'
/** Status je Balkenstück; '' = unbekannt (nicht beobachtet) */
export type BucketStatus = TunnelStatus | ''

export interface TunnelConnection {
  id: string
  colo_name: string
  client_id: string
  client_version: string
  opened_at: string
  origin_ip: string
  is_pending_reconnect: boolean
}

export interface Uptime {
  percent: number | null
  observed: number
  start: string
  bucket_seconds: number
  buckets: BucketStatus[]
}

export interface Tunnel {
  id: string
  name: string
  status: TunnelStatus
  created_at: string
  conns_active_at?: string
  conns_inactive_at?: string
  connections: TunnelConnection[]
  remote_config: boolean
  first_seen_at: string
  last_seen_at: string
  status_since?: string
  uptime: Record<'24h' | '7d', Uptime>
}

export interface TunnelOverview {
  configured: boolean
  last_poll?: string
  error?: string
  interval_seconds: number
  tunnels: Tunnel[]
}

export interface TunnelChange {
  tunnel_id: string
  tunnel_name: string
  from?: TunnelStatus
  to: TunnelStatus
  at: string
}

export type NotifyEventType =
  | 'ip_change'
  | 'update_failed'
  | 'update_recovered'
  | 'tunnel_status'
  | 'site_down'
  | 'site_recovered'
  | 'cert_expiring'
  | 'blocklist_listed'

export type ProbeStatus = 'pending' | 'up' | 'expiring' | 'tls_error' | 'down' | 'paused'

/** Erreichbarkeitsprüfung (HTTP/HTTPS) einer Adresse. */
export interface Probe {
  id: number
  /** gesetzt = Prüfung von https://<record_name>/, folgt dem Record */
  record_id?: number
  record_name?: string
  url: string
  enabled: boolean
  status: ProbeStatus
  http_status?: number
  latency_ms?: number
  /** Fehlschläge in Folge */
  fail_count: number
  message?: string
  tls_not_after?: string
  tls_issuer?: string
  tls_valid?: boolean
  last_checked_at?: string
  last_changed_at?: string
}

export interface ProbeInput {
  url?: string
  enabled?: boolean
}

export interface Settings {
  ip_check_interval_seconds: number
  tunnel_interval_seconds: number
  notify_language: 'de' | 'en'
  probe_interval_seconds: number
  tls_warn_days: number
  ip_sources: { name: string; enabled: boolean }[]
  limits: Record<string, number>
}

export type WebhookMethod = 'POST' | 'PUT' | 'PATCH' | 'GET'

export interface WebhookInput {
  name: string
  enabled: boolean
  method: WebhookMethod
  url: string
  headers: { name: string; value: string }[]
  content_type: string
  body_template: string
  events: NotifyEventType[]
}

export interface Webhook extends WebhookInput {
  id: number
  last_sent_at?: string
  last_error?: string
  created_at: string
  updated_at: string
  /** verwendete, aber nicht gesetzte Env-Variablen (nur Namen) */
  missing_env: string[]
}

export interface WebhookPreview {
  method: string
  url: string
  headers: Record<string, string[]>
  body: string
}

export interface TestResult {
  ok: boolean
  error?: string
}

export interface Info {
  version: string
  cf_token_set: boolean
  cf_account_set: boolean
  data_dir: string
  /** DNS-Verbreitungsprüfung aktiv */
  dnscheck: boolean
}

export interface IPHistoryFilter {
  family?: Family
  before_id?: number
}

export interface UpdateLogFilter {
  record_id?: number
  result?: UpdateResult
  before_id?: number
}

export interface TunnelHistoryFilter {
  tunnel_id?: string
  before?: string
}

function qs(params: Record<string, string | number | undefined>): string {
  const p = new URLSearchParams()
  for (const [k, v] of Object.entries(params)) {
    if (v !== undefined && v !== '') p.set(k, String(v))
  }
  const s = p.toString()
  return s ? `?${s}` : ''
}

export interface Session {
  authenticated: boolean
}

export class ApiError extends Error {
  readonly status: number

  constructor(status: number, message: string) {
    super(message)
    this.status = status
  }
}

async function request<T>(method: string, path: string, body?: unknown): Promise<T> {
  const headers: Record<string, string> = { 'Accept-Language': currentLang() } // Server-Meldungen in UI-Sprache
  if (body !== undefined) headers['Content-Type'] = 'application/json'
  const res = await fetch(path, {
    method,
    credentials: 'same-origin',
    headers,
    body: body !== undefined ? JSON.stringify(body) : undefined,
  })
  if (res.status === 204) return undefined as T
  if (!res.ok) {
    let msg = `HTTP ${res.status}`
    try {
      const data = (await res.json()) as { error?: string }
      if (data.error) msg = data.error
    } catch {
      // keine JSON-Antwort
    }
    throw new ApiError(res.status, msg)
  }
  return (await res.json()) as T
}

export const api = {
  session: () => request<Session>('GET', '/api/session'),
  login: (password: string) => request<Session>('POST', '/api/login', { password }),
  logout: () => request<Session>('POST', '/api/logout'),
  ip: () => request<IPState>('GET', '/api/ip'),
  ipHistory: (limit = 20) => request<IPChange[]>('GET', `/api/ip/history?limit=${limit}`),
  refreshIP: () => request<IPState>('POST', '/api/ip/refresh'),
  zones: () => request<Zone[]>('GET', '/api/zones'),
  records: () => request<DnsRecord[]>('GET', '/api/records'),
  createRecord: (r: RecordInput) => request<DnsRecord>('POST', '/api/records', r),
  updateRecord: (id: number, r: RecordInput) => request<DnsRecord>('PUT', `/api/records/${id}`, r),
  deleteRecord: (id: number) => request<void>('DELETE', `/api/records/${id}`),
  syncRecord: (id: number) => request<DnsRecord>('POST', `/api/records/${id}/sync`),
  checkPropagation: (id: number) => request<DnsRecord>('POST', `/api/records/${id}/propagation`),
  syncAll: () => request<DnsRecord[]>('POST', '/api/records/sync'),
  updates: (limit = 20) => request<UpdateLogEntry[]>('GET', `/api/updates?limit=${limit}`),
  tunnels: () => request<TunnelOverview>('GET', '/api/tunnels'),
  refreshTunnels: () => request<TunnelOverview>('POST', '/api/tunnels/refresh'),
  ipHistoryPage: (f: IPHistoryFilter, limit = 50) =>
    request<IPChange[]>('GET', `/api/ip/history${qs({ ...f, limit })}`),
  updatesPage: (f: UpdateLogFilter, limit = 50) =>
    request<UpdateLogEntry[]>('GET', `/api/updates${qs({ ...f, limit })}`),
  tunnelHistoryPage: (f: TunnelHistoryFilter, limit = 50) =>
    request<TunnelChange[]>('GET', `/api/tunnels/history${qs({ ...f, limit })}`),
  settings: () => request<Settings>('GET', '/api/settings'),
  saveSettings: (s: Settings) => request<Settings>('PUT', '/api/settings', s),
  webhooks: () => request<Webhook[]>('GET', '/api/webhooks'),
  createWebhook: (w: WebhookInput) => request<Webhook>('POST', '/api/webhooks', w),
  updateWebhook: (id: number, w: WebhookInput) => request<Webhook>('PUT', `/api/webhooks/${id}`, w),
  deleteWebhook: (id: number) => request<void>('DELETE', `/api/webhooks/${id}`),
  testWebhook: (id: number) => request<TestResult>('POST', `/api/webhooks/${id}/test`),
  previewWebhook: (w: WebhookInput, eventType: string) =>
    request<WebhookPreview>('POST', '/api/webhooks/preview', { ...w, event_type: eventType }),
  probes: () => request<Probe[]>('GET', '/api/probes'),
  createProbe: (p: ProbeInput) => request<Probe>('POST', '/api/probes', p),
  updateProbe: (id: number, p: ProbeInput) => request<Probe>('PUT', `/api/probes/${id}`, p),
  deleteProbe: (id: number) => request<void>('DELETE', `/api/probes/${id}`),
  runProbe: (id: number) => request<Probe>('POST', `/api/probes/${id}/run`),
  info: () => request<Info>('GET', '/api/info'),
}
