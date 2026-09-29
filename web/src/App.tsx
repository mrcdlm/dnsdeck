import { Navigate, Route, Routes } from 'react-router'

import { Layout } from '@/components/Layout'
import { RequireAuth } from '@/components/RequireAuth'
import { Dashboard } from '@/pages/Dashboard'
import { Login } from '@/pages/Login'
import { Records } from '@/pages/Records'

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
      </Route>
      <Route path="*" element={<Navigate to="/" replace />} />
    </Routes>
  )
}
