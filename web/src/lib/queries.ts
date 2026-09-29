import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'

import { api } from './api'

export const keys = {
  session: ['session'] as const,
  ip: ['ip'] as const,
  ipHistory: ['ip', 'history'] as const,
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

export function useRefreshIP() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: api.refreshIP,
    onSuccess: (state) => {
      qc.setQueryData(keys.ip, state)
      qc.invalidateQueries({ queryKey: keys.ipHistory })
    },
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
      qc.clear()
      qc.setQueryData(keys.session, { authenticated: false })
    },
  })
}
