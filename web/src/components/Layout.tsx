import { History, LayoutDashboard, ListTree, LogOut, Network, Settings, type LucideIcon } from 'lucide-react'
import { NavLink, Outlet } from 'react-router'

import { LiveProvider } from '@/components/LiveProvider'
import { Logo } from '@/components/Logo'
import { Button } from '@/components/ui/button'
import { useLive } from '@/lib/live-context'
import { useLogout } from '@/lib/queries'
import { cn } from '@/lib/utils'

interface NavItem {
  to: string
  label: string
  icon: LucideIcon
  soon?: boolean
}

// Nicht aktive Seiten werden in späteren Meilensteinen freigeschaltet.
const nav: NavItem[] = [
  { to: '/', label: 'Dashboard', icon: LayoutDashboard },
  { to: '/records', label: 'Records', icon: ListTree },
  { to: '/tunnels', label: 'Tunnels', icon: Network },
  { to: '/verlauf', label: 'Verlauf', icon: History, soon: true },
  { to: '/einstellungen', label: 'Einstellungen', icon: Settings, soon: true },
]

function NavEntry({ item, compact }: { item: NavItem; compact?: boolean }) {
  const Icon = item.icon
  const base = cn(
    'flex items-center gap-2 rounded-md px-3 py-2 text-sm font-medium transition-colors',
    compact && 'shrink-0 px-2.5 py-1.5',
  )

  if (item.soon) {
    return (
      <span
        className={cn(base, 'text-muted-foreground/50 cursor-not-allowed')}
        title="Kommt in einem späteren Meilenstein"
        aria-disabled="true"
      >
        <Icon className="size-4" />
        {item.label}
        {!compact && <span className="ml-auto text-[10px] tracking-wide uppercase">bald</span>}
      </span>
    )
  }

  return (
    <NavLink
      to={item.to}
      end
      className={({ isActive }) =>
        cn(
          base,
          isActive
            ? 'bg-accent text-accent-foreground'
            : 'text-muted-foreground hover:bg-accent/50 hover:text-foreground',
        )
      }
    >
      <Icon className="size-4" />
      {item.label}
    </NavLink>
  )
}

function LiveIndicator({ compact }: { compact?: boolean }) {
  const live = useLive()
  return (
    <span
      className="text-muted-foreground flex items-center gap-2 text-xs"
      title={live ? 'Änderungen erscheinen sofort' : 'Keine Live-Verbindung – Daten werden alle 30 s nachgeladen'}
    >
      <span className={cn('size-2 rounded-full', live ? 'bg-success' : 'bg-muted-foreground/50')} />
      {!compact && (live ? 'Live' : 'Offline – lädt alle 30 s')}
    </span>
  )
}

export function Layout() {
  return (
    <LiveProvider>
      <Shell />
    </LiveProvider>
  )
}

function Shell() {
  const logout = useLogout()

  return (
    <div className="flex min-h-svh flex-col md:flex-row">
      {/* Desktop: Seitenleiste */}
      <aside className="bg-card/40 hidden w-60 shrink-0 flex-col border-r p-4 md:flex">
        <Logo className="px-2 py-1" />
        <nav className="mt-6 flex flex-col gap-1">
          {nav.map((item) => (
            <NavEntry key={item.to} item={item} />
          ))}
        </nav>
        <div className="mt-auto px-3 py-2">
          <LiveIndicator />
        </div>
        <Button
          variant="ghost"
          className="text-muted-foreground justify-start"
          onClick={() => logout.mutate()}
          disabled={logout.isPending}
        >
          <LogOut />
          Abmelden
        </Button>
      </aside>

      {/* Mobil: Kopfzeile mit horizontal scrollbarer Navigation */}
      <header className="bg-background/80 sticky top-0 z-10 border-b backdrop-blur md:hidden">
        <div className="flex items-center justify-between px-4 py-3">
          <div className="flex items-center gap-3">
            <Logo />
            <LiveIndicator compact />
          </div>
          <Button
            variant="ghost"
            size="icon"
            aria-label="Abmelden"
            onClick={() => logout.mutate()}
            disabled={logout.isPending}
          >
            <LogOut />
          </Button>
        </div>
        <nav className="flex gap-1 overflow-x-auto px-3 pb-2">
          {nav.map((item) => (
            <NavEntry key={item.to} item={item} compact />
          ))}
        </nav>
      </header>

      <main className="min-w-0 flex-1 px-4 py-6 md:px-8 md:py-8">
        <div className="mx-auto max-w-5xl">
          <Outlet />
        </div>
      </main>
    </div>
  )
}
