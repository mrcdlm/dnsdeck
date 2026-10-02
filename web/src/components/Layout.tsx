import { Activity, History, LayoutDashboard, ListTree, LogOut, Network, Settings, type LucideIcon } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { NavLink, Outlet } from 'react-router'

import { LiveProvider } from '@/components/LiveProvider'
import { Logo } from '@/components/Logo'
import { Preferences } from '@/components/Preferences'
import { Button } from '@/components/ui/button'
import { useLive } from '@/lib/live-context'
import { useLogout } from '@/lib/queries'
import { cn } from '@/lib/utils'

type NavKey = 'dashboard' | 'records' | 'tunnels' | 'checks' | 'history' | 'settings'

const nav: { to: string; key: NavKey; icon: LucideIcon }[] = [
  { to: '/', key: 'dashboard', icon: LayoutDashboard },
  { to: '/records', key: 'records', icon: ListTree },
  { to: '/tunnels', key: 'tunnels', icon: Network },
  { to: '/erreichbarkeit', key: 'checks', icon: Activity },
  { to: '/verlauf', key: 'history', icon: History },
  { to: '/einstellungen', key: 'settings', icon: Settings },
]

function NavEntry({ item, compact }: { item: (typeof nav)[number]; compact?: boolean }) {
  const { t } = useTranslation()
  const Icon = item.icon
  return (
    <NavLink
      to={item.to}
      end
      className={({ isActive }) =>
        cn(
          'flex items-center gap-2 rounded-md px-3 py-2 text-sm font-medium transition-colors',
          compact && 'shrink-0 px-2.5 py-1.5',
          isActive
            ? 'bg-accent text-accent-foreground'
            : 'text-muted-foreground hover:bg-accent/50 hover:text-foreground',
        )
      }
    >
      <Icon className="size-4" />
      {t(`nav.${item.key}`)}
    </NavLink>
  )
}

function LiveIndicator({ compact }: { compact?: boolean }) {
  const { t } = useTranslation()
  const live = useLive()
  return (
    <span
      className="text-muted-foreground flex items-center gap-2 text-xs"
      title={live ? t('live.onHint') : t('live.offHint')}
    >
      <span className={cn('size-2 rounded-full', live ? 'bg-success' : 'bg-muted-foreground/50')} />
      {!compact && (live ? t('live.on') : t('live.off'))}
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
  const { t } = useTranslation()
  const logout = useLogout()

  return (
    <div className="flex min-h-svh flex-col md:flex-row">
      {/* Desktop: Seitenleiste */}
      <aside className="bg-card/40 sticky top-0 hidden h-svh w-60 shrink-0 flex-col overflow-y-auto border-r p-4 md:flex">
        <Logo className="px-2 py-1" />
        <nav className="mt-6 flex flex-col gap-1">
          {nav.map((item) => (
            <NavEntry key={item.to} item={item} />
          ))}
        </nav>
        <div className="mt-auto flex items-center justify-between gap-2 px-3 py-2">
          <LiveIndicator />
          <Preferences />
        </div>
        <Button
          variant="ghost"
          className="text-muted-foreground justify-start"
          onClick={() => logout.mutate()}
          disabled={logout.isPending}
        >
          <LogOut />
          {t('auth.logout')}
        </Button>
      </aside>

      {/* Mobil: Kopfzeile mit horizontal scrollbarer Navigation */}
      <header className="bg-background/80 sticky top-0 z-10 border-b backdrop-blur md:hidden">
        <div className="flex items-center justify-between gap-2 px-4 py-3">
          <div className="flex items-center gap-3">
            <Logo />
            <LiveIndicator compact />
          </div>
          <div className="flex items-center gap-1">
            <Preferences />
            <Button
              variant="ghost"
              size="icon"
              aria-label={t('auth.logout')}
              onClick={() => logout.mutate()}
              disabled={logout.isPending}
            >
              <LogOut />
            </Button>
          </div>
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
