import { Bell, BellOff, Calendar, ChevronDown, ChevronRight, GitBranch, LogOut, Moon, Settings, Siren, Sun } from 'lucide-react'
import { useState } from 'react'
import { NavLink, Outlet, useLocation, useParams } from 'react-router-dom'

import { SessionBanner } from '@/auth/SessionBanner'
import { useAuth } from '@/auth/useAuth'
import { usePermissions } from '@/auth/usePermissions'
import { useAudioEnabled } from '@/hooks/useAudioEnabled'
import { useTheme } from '@/hooks/useTheme'
import { cn } from '@/lib/utils'

const navLinkClass = ({ isActive }: { isActive: boolean }) =>
  cn(
    'flex items-center gap-2.5 rounded-md px-3 py-2 text-sm font-medium transition-colors',
    isActive ? 'bg-accent text-accent-foreground' : 'text-muted-foreground hover:bg-accent/50 hover:text-foreground',
  )

const subNavLinkClass = ({ isActive }: { isActive: boolean }) =>
  cn(
    'flex items-center rounded-md py-1.5 pl-9 pr-3 text-sm font-medium transition-colors',
    isActive ? 'bg-accent text-accent-foreground' : 'text-muted-foreground hover:bg-accent/50 hover:text-foreground',
  )

export function GlobalLayout() {
  const { tenant } = useParams<{ tenant: string }>()
  const { user, signOut } = useAuth()
  const permissions = usePermissions()
  const isAdmin = tenant ? permissions[tenant] === 'admin' : false
  const { scheme, toggle: toggleTheme } = useTheme()
  const [audioEnabled, setAudioEnabled] = useAudioEnabled()

  const location = useLocation()
  const [settingsOpen, setSettingsOpen] = useState(
    tenant ? location.pathname.startsWith(`/${tenant}/settings`) : false,
  )

  return (
    <div className="flex h-screen flex-col bg-background text-foreground">
      <SessionBanner />

      <div className="flex flex-1 overflow-hidden">
        {/* Sidebar */}
        <aside className="flex w-52 flex-shrink-0 flex-col border-r">
          <div className="flex h-12 items-center border-b px-4">
            <span className="text-sm font-semibold">{tenant ?? 'SRE OnCall'}</span>
          </div>

          {tenant && (
            <nav className="flex flex-col gap-1 p-2">
              <NavLink to={`/${tenant}/incidents`} className={navLinkClass}>
                <Siren size={16} />
                Инциденты
              </NavLink>
              <NavLink to={`/${tenant}/schedules`} className={navLinkClass}>
                <Calendar size={16} />
                Расписания
              </NavLink>
              <NavLink to={`/${tenant}/escalations`} className={navLinkClass}>
                <GitBranch size={16} />
                Эскалации
              </NavLink>
              <div>
                <button
                  type="button"
                  onClick={() => setSettingsOpen((o) => !o)}
                  className={cn(
                    'flex w-full items-center gap-2.5 rounded-md px-3 py-2 text-sm font-medium transition-colors',
                    'text-muted-foreground hover:bg-accent/50 hover:text-foreground',
                  )}
                  aria-expanded={settingsOpen}
                >
                  <Settings size={16} />
                  Настройки
                  {settingsOpen ? (
                    <ChevronDown size={14} className="ml-auto" />
                  ) : (
                    <ChevronRight size={14} className="ml-auto" />
                  )}
                </button>
                {settingsOpen && (
                  <div className="mt-1 flex flex-col gap-1">
                    <NavLink to={`/${tenant}/settings/profile`} className={subNavLinkClass}>
                      Мой профиль
                    </NavLink>
                    {isAdmin && (
                      <>
                        <NavLink to={`/${tenant}/settings/webhook-tokens`} className={subNavLinkClass}>
                          Webhook-токены
                        </NavLink>
                        <NavLink to={`/${tenant}/settings/notifications`} className={subNavLinkClass}>
                          Конфигурация уведомлений
                        </NavLink>
                        <NavLink to={`/${tenant}/settings/members`} className={subNavLinkClass}>
                          Участники команды
                        </NavLink>
                      </>
                    )}
                  </div>
                )}
              </div>
            </nav>
          )}
        </aside>

        {/* Main area */}
        <div className="flex flex-1 flex-col overflow-hidden">
          {/* Header */}
          <header className="flex h-12 flex-shrink-0 items-center justify-end gap-3 border-b px-4">
            <span className="mr-auto text-sm text-muted-foreground">
              {user?.profile.preferred_username}
            </span>

            <button
              onClick={() => setAudioEnabled(!audioEnabled)}
              aria-label={audioEnabled ? 'Выключить звук' : 'Включить звук'}
              className="rounded p-1.5 text-muted-foreground hover:bg-accent hover:text-foreground"
            >
              {audioEnabled ? <Bell size={16} /> : <BellOff size={16} />}
            </button>

            <button
              onClick={toggleTheme}
              aria-label={scheme === 'dark' ? 'Светлая тема' : 'Тёмная тема'}
              className="rounded p-1.5 text-muted-foreground hover:bg-accent hover:text-foreground"
            >
              {scheme === 'dark' ? <Sun size={16} /> : <Moon size={16} />}
            </button>

            <button
              onClick={() => signOut()}
              aria-label="Выйти"
              className="rounded p-1.5 text-muted-foreground hover:bg-accent hover:text-foreground"
            >
              <LogOut size={16} />
            </button>
          </header>

          <main className="flex-1 overflow-auto">
            <Outlet />
          </main>
        </div>
      </div>
    </div>
  )
}
