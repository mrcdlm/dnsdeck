import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'

import { api, type RecordInput } from './api'

export const keys = {
  session: ['session'] as const,
  ip: ['ip'] as const,
  ipHistory: ['ip', 'history'] as const,
  zones: ['zones'] as const,
  records: ['records'] as const,
  updates: ['updates'] as const,
}

// Bis SSE (Meilenstein 3) verfügbar ist, wird regelmäßig nachgeladen.
const POLL_MS = 30_000

export function useSession() {
  return useQuery({ queryKey: keys.session, queryFn: api.session, staleTime: 60_000 })
}

export function useIP() {
  return useQuery({ queryKey: keys.ip, queryFn: api.ip, refetchInterval: POLL_MS })
}

export function useIPHistory(limit = 20) {
  return useQuery({
    queryKey: [...keys.ipHistory, limit],
    queryFn: () => api.ipHistory(limit),
    refetchInterval: POLL_MS,
  })
}

/** Nach jedem Abgleich ändern sich IP, Records und Update-Log gemeinsam. */
function useInvalidateAll() {
  const qc = useQueryClient()
  return () =>
    Promise.all([
      qc.invalidateQueries({ queryKey: keys.ip }),
      qc.invalidateQueries({ queryKey: keys.records }),
      qc.invalidateQueries({ queryKey: keys.updates }),
    ])
}

/** IP prüfen und alle Records abgleichen. */
export function useRefreshIP() {
  const qc = useQueryClient()
  const invalidate = useInvalidateAll()
  return useMutation({
    mutationFn: api.refreshIP,
    onSuccess: (state) => {
      qc.setQueryData(keys.ip, state)
      return invalidate()
    },
  })
}

export function useZones(enabled = true) {
  return useQuery({ queryKey: keys.zones, queryFn: api.zones, staleTime: 5 * 60_000, retry: false, enabled })
}

export function useRecords() {
  return useQuery({ queryKey: keys.records, queryFn: api.records, refetchInterval: POLL_MS })
}

export function useUpdates(limit = 20) {
  return useQuery({
    queryKey: [...keys.updates, limit],
    queryFn: () => api.updates(limit),
    refetchInterval: POLL_MS,
  })
}

export function useSaveRecord() {
  const invalidate = useInvalidateAll()
  return useMutation({
    mutationFn: ({ id, input }: { id?: number; input: RecordInput }) =>
      id ? api.updateRecord(id, input) : api.createRecord(input),
    onSuccess: invalidate,
  })
}

export function useDeleteRecord() {
  const invalidate = useInvalidateAll()
  return useMutation({ mutationFn: api.deleteRecord, onSuccess: invalidate })
}

export function useSyncRecord() {
  const invalidate = useInvalidateAll()
  return useMutation({ mutationFn: api.syncRecord, onSettled: invalidate })
}

export function useLogin() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: api.login,
    onSuccess: (session) => qc.setQueryData(keys.session, session),
  })
}

export function useLogout() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: api.logout,
    onSettled: () => {
      // Kein qc.clear(): das entfernt auch die Session-Query, ohne ihre
      // Beobachter zu benachrichtigen – RequireAuth bekäme den Logout nie mit.
      qc.setQueryData(keys.session, { authenticated: false })
      qc.removeQueries({ predicate: (q) => q.queryKey[0] !== keys.session[0] })
    },
  })
}
