import { createContext, useContext } from 'react'

export const LiveContext = createContext(false)

/** true, solange die Live-Verbindung (SSE) steht. */
export function useLive() {
  return useContext(LiveContext)
}
