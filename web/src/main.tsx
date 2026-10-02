import { MutationCache, QueryCache, QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { StrictMode } from 'react'
import { createRoot } from 'react-dom/client'
import { BrowserRouter } from 'react-router'

import './i18n'
import App from './App.tsx'
import { ApiError } from './lib/api.ts'
import { keys } from './lib/queries.ts'
import './index.css'

// Ein 401 bei einer beliebigen Anfrage bedeutet: Session abgelaufen →
// Session-Status zurücksetzen, RequireAuth leitet dann zum Login.
function onError(err: unknown) {
  if (err instanceof ApiError && err.status === 401) {
    queryClient.setQueryData(keys.session, { authenticated: false })
  }
}

const queryClient = new QueryClient({
  queryCache: new QueryCache({ onError }),
  mutationCache: new MutationCache({ onError }),
  defaultOptions: {
    queries: {
      retry: (count, err) => !(err instanceof ApiError && err.status < 500) && count < 2,
    },
  },
})

// Service Worker (installierbare App, Offline-Hülle) nur im Production-Build;
// der Name des Haupt-Bundles enthält den Build-Hash und versioniert den Cache.
if (import.meta.env.PROD && 'serviceWorker' in navigator) {
  const build = new URL(import.meta.url).pathname.split('/').pop() ?? ''
  window.addEventListener('load', () => {
    navigator.serviceWorker.register(`/sw.js?build=${encodeURIComponent(build)}`).catch(() => {
      // Ohne HTTPS (außer localhost) nicht verfügbar – die App läuft trotzdem.
    })
  })
}

createRoot(document.getElementById('root')!).render(
  <StrictMode>
    <QueryClientProvider client={queryClient}>
      <BrowserRouter>
        <App />
      </BrowserRouter>
    </QueryClientProvider>
  </StrictMode>,
)
