export type Family = 'ipv4' | 'ipv6'
export type IPStatus = 'ok' | 'unconfirmed' | 'unavailable' | 'pending'

export interface Observation {
  source: string
  ip?: string
  error?: string
}

export interface FamilyState {
  ip?: string
  since?: string
  status: IPStatus
  message?: string
  votes: number
  responses: number
  sources: Observation[]
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
  last_checked_at?: string
  last_changed_at?: string
  created_at: string
  updated_at: string
}

export interface RecordInput {
  zone_id: string
  name: string
  type: RecordType
  proxied: boolean
  ttl: number
  enabled: boolean
}

export interface Zone {
  id: string
  name: string
}

export type UpdateResult = 'created' | 'adopted' | 'updated' | 'error'
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
  const res = await fetch(path, {
    method,
    credentials: 'same-origin',
    headers: body !== undefined ? { 'Content-Type': 'application/json' } : undefined,
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
  syncAll: () => request<DnsRecord[]>('POST', '/api/records/sync'),
  updates: (limit = 20) => request<UpdateLogEntry[]>('GET', `/api/updates?limit=${limit}`),
  tunnels: () => request<TunnelOverview>('GET', '/api/tunnels'),
  refreshTunnels: () => request<TunnelOverview>('POST', '/api/tunnels/refresh'),
}
