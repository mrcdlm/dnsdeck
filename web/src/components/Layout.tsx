import { History, LayoutDashboard, ListTree, LogOut, Network, Settings, type LucideIcon } from 'lucide-react'
import { NavLink, Outlet } from 'react-router'

import { Logo } from '@/components/Logo'
import { Button } from '@/components/ui/button'
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
  { to: '/tunnels', label: 'Tunnels', icon: Network, soon: true },
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

export function Layout() {
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
        <Button
          variant="ghost"
          className="text-muted-foreground mt-auto justify-start"
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
          <Logo />
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
