import { Loader2 } from 'lucide-react'
import { lazy, Suspense, type ReactNode } from 'react'
import { Navigate, Route, Routes } from 'react-router'

import { Layout } from '@/components/Layout'
import { RequireAuth } from '@/components/RequireAuth'
import { Dashboard } from '@/pages/Dashboard'
import { Login } from '@/pages/Login'

// Unterseiten erst bei Bedarf laden (kleineres Start-Bundle).
const Records = lazy(() => import('@/pages/Records').then((m) => ({ default: m.Records })))
const Tunnels = lazy(() => import('@/pages/Tunnels').then((m) => ({ default: m.Tunnels })))
const Checks = lazy(() => import('@/pages/Checks').then((m) => ({ default: m.Checks })))
const Speedtest = lazy(() => import('@/pages/Speedtest').then((m) => ({ default: m.Speedtest })))
const History = lazy(() => import('@/pages/History').then((m) => ({ default: m.History })))
const Settings = lazy(() => import('@/pages/Settings').then((m) => ({ default: m.Settings })))

function Lazy({ children }: { children: ReactNode }) {
  return (
    <Suspense
      fallback={
        <div className="grid place-items-center py-24">
          <Loader2 className="text-muted-foreground size-6 animate-spin" />
        </div>
      }
    >
      {children}
    </Suspense>
  )
}

export default function App() {
  return (
    <Routes>
      <Route path="/login" element={<Login />} />
      <Route
        element={
          <RequireAuth>
            <Layout />
          </RequireAuth>
        }
      >
        <Route index element={<Dashboard />} />
        <Route path="records" element={<Lazy><Records /></Lazy>} />
        <Route path="tunnels" element={<Lazy><Tunnels /></Lazy>} />
        <Route path="erreichbarkeit" element={<Lazy><Checks /></Lazy>} />
        <Route path="speedtest" element={<Lazy><Speedtest /></Lazy>} />
        <Route path="verlauf"element={<Lazy><History /></Lazy>} />
        <Route path="einstellungen" element={<Lazy><Settings /></Lazy>} />
      </Route>
      <Route path="*" element={<Navigate to="/" replace />} />
    </Routes>
  )
}
