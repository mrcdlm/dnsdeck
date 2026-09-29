import { MutationCache, QueryCache, QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { StrictMode } from 'react'
import { createRoot } from 'react-dom/client'
import { BrowserRouter } from 'react-router'

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

createRoot(document.getElementById('root')!).render(
  <StrictMode>
    <QueryClientProvider client={queryClient}>
      <BrowserRouter>
        <App />
      </BrowserRouter>
    </QueryClientProvider>
  </StrictMode>,
)
