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
}
