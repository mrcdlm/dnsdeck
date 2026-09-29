import { Loader2 } from 'lucide-react'
import type { ReactNode } from 'react'
import { Navigate } from 'react-router'

import { useSession } from '@/lib/queries'

export function RequireAuth({ children }: { children: ReactNode }) {
  const { data, isPending, isError } = useSession()

  if (isPending) {
    return (
      <div className="grid min-h-svh place-items-center">
        <Loader2 className="text-muted-foreground size-6 animate-spin" />
      </div>
    )
  }
  if (isError || !data.authenticated) {
    return <Navigate to="/login" replace />
  }
  return children
}
