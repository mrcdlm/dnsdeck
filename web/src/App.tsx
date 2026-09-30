import { Navigate, Route, Routes } from 'react-router'

import { Layout } from '@/components/Layout'
import { RequireAuth } from '@/components/RequireAuth'
import { Dashboard } from '@/pages/Dashboard'
import { History } from '@/pages/History'
import { Login } from '@/pages/Login'
import { Records } from '@/pages/Records'
import { Settings } from '@/pages/Settings'
import { Tunnels } from '@/pages/Tunnels'

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
        <Route path="records" element={<Records />} />
        <Route path="tunnels" element={<Tunnels />} />
        <Route path="verlauf" element={<History />} />
        <Route path="einstellungen" element={<Settings />} />
      </Route>
      <Route path="*" element={<Navigate to="/" replace />} />
    </Routes>
  )
}
