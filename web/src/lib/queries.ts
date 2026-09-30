import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'

import { api, type RecordInput } from './api'
import { useLive } from './live-context'
import { keys } from './queries-keys'

export { keys }

// Ohne Live-Verbindung (SSE) wird regelmäßig nachgeladen; mit ihr nur noch
// selten als Absicherung.
const POLL_MS = 30_000
const LIVE_POLL_MS = 5 * 60_000

function usePollInterval() {
  return useLive() ? LIVE_POLL_MS : POLL_MS
}

export function useSession() {
  return useQuery({ queryKey: keys.session, queryFn: api.session, staleTime: 60_000 })
}

export function useIP() {
  const poll = usePollInterval()
  return useQuery({ queryKey: keys.ip, queryFn: api.ip, refetchInterval: poll })
}

export function useIPHistory(limit = 20) {
  const poll = usePollInterval()
  return useQuery({
    queryKey: [...keys.ipHistory, limit],
    queryFn: () => api.ipHistory(limit),
    refetchInterval: poll,
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
  const poll = usePollInterval()
  return useQuery({ queryKey: keys.records, queryFn: api.records, refetchInterval: poll })
}

export function useUpdates(limit = 20) {
  const poll = usePollInterval()
  return useQuery({
    queryKey: [...keys.updates, limit],
    queryFn: () => api.updates(limit),
    refetchInterval: poll,
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

export function useTunnels() {
  const poll = usePollInterval()
  return useQuery({ queryKey: keys.tunnels, queryFn: api.tunnels, refetchInterval: poll })
}

export function useRefreshTunnels() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: api.refreshTunnels,
    onSuccess: (o) => qc.setQueryData(keys.tunnels, o),
  })
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
