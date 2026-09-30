import { useQueryClient } from '@tanstack/react-query'
import { useEffect, useState, type ReactNode } from 'react'

import { LiveContext } from '@/lib/live-context'
import { keys } from '@/lib/queries-keys'

/** Welche Queries ein Ereignisthema betrifft. */
const topicKeys: Record<string, readonly (readonly string[])[]> = {
  ip: [keys.ip], // inkl. ['ip', 'history']
  records: [keys.records],
  updates: [keys.updates],
  tunnels: [keys.tunnels],
}

/**
 * Hält eine Server-Sent-Events-Verbindung offen und lädt bei jedem Ereignis
 * die betroffenen Daten nach. Bricht die Verbindung ab, verbindet der Browser
 * selbst neu; bei endgültigem Abbruch (z. B. 401) wird nach einer Pause neu
 * versucht und die Session geprüft.
 */
export function LiveProvider({ children }: { children: ReactNode }) {
  const qc = useQueryClient()
  const [connected, setConnected] = useState(false)

  useEffect(() => {
    let es: EventSource | undefined
    let retry: ReturnType<typeof setTimeout> | undefined
    let opened = false
    let stopped = false

    function connect() {
      es = new EventSource('/api/events')
      es.onopen = () => {
        setConnected(true)
        // Nach einem Neuverbinden könnten Ereignisse verpasst worden sein.
        if (opened) void qc.invalidateQueries({ predicate: (q) => q.queryKey[0] !== keys.session[0] })
        opened = true
      }
      es.addEventListener('change', (e) => {
        for (const key of topicKeys[(e as MessageEvent<string>).data] ?? []) {
          void qc.invalidateQueries({ queryKey: key })
        }
      })
      es.addEventListener('session', () => {
        qc.setQueryData(keys.session, { authenticated: false })
      })
      es.onerror = () => {
        setConnected(false)
        if (es?.readyState === EventSource.CLOSED && !stopped) {
          // Endgültig geschlossen (z. B. 401): Session prüfen, später erneut versuchen.
          void qc.invalidateQueries({ queryKey: keys.session })
          retry = setTimeout(connect, 10_000)
        }
      }
    }

    connect()
    return () => {
      stopped = true
      clearTimeout(retry)
      es?.close()
    }
  }, [qc])

  return <LiveContext.Provider value={connected}>{children}</LiveContext.Provider>
}
