import { useEffect, useRef, useState } from 'react'
import { NavLink, Outlet, useLocation, useNavigate } from 'react-router-dom'
import {
  Boxes,
  Layers,
  LayoutDashboard,
  LogOut,
  Menu,
  Server,
  Settings,
  ShieldCheck,
  Users,
  X,
} from 'lucide-react'
import { useLive } from '../data/live'
import { useSession } from '../data/session'
import { roleLabel } from '../lib/format'
import { cx } from '../lib/cx'
import { Button } from '../ui/Button'
import { Logo } from '../ui/Logo'
import styles from './AppLayout.module.css'

const navItems = [
  { to: '/', label: 'Обзор', icon: LayoutDashboard, end: true },
  { to: '/instances', label: 'Экземпляры', icon: Boxes },
  { to: '/agents', label: 'Агенты', icon: Server, admin: true },
  { to: '/images', label: 'Образы', icon: Layers, admin: true },
  { to: '/users', label: 'Пользователи', icon: Users, admin: true },
  { to: '/security', label: 'Безопасность', icon: ShieldCheck, admin: true },
]

const statusLabels = {
  live: 'Онлайн',
  connecting: 'Подключение',
  reconnecting: 'Переподключение',
  offline: 'Офлайн',
} as const

export function AppLayout() {
  const { user, isAdmin, signOut, version } = useSession()
  const live = useLive()
  const navigate = useNavigate()
  const location = useLocation()
  const [drawerOpen, setDrawerOpen] = useState(false)
  const drawerRef = useRef<HTMLDivElement | null>(null)
  const mainRef = useRef<HTMLElement | null>(null)

  useEffect(() => {
    setDrawerOpen(false)
    mainRef.current?.scrollTo({ top: 0, behavior: 'instant' })
    const path = location.pathname
    const title =
      path === '/'
        ? 'Обзор'
        : path.startsWith('/instances/')
          ? 'Экземпляр'
          : path.startsWith('/instances')
            ? 'Экземпляры'
            : path.startsWith('/agents')
              ? 'Агенты'
              : path.startsWith('/images')
                ? 'Образы'
                : path.startsWith('/users')
                  ? 'Пользователи'
                  : path.startsWith('/security')
                    ? 'Безопасность'
                    : path.startsWith('/settings')
                      ? 'Настройки'
                      : 'Dockium'
    document.title = title === 'Dockium' ? title : `${title} · Dockium`
  }, [location.pathname])

  useEffect(() => {
    if (!drawerOpen) return
    const previous = document.body.style.overflow
    document.body.style.overflow = 'hidden'
    const onKeyDown = (event: KeyboardEvent) => {
      if (event.key === 'Escape') setDrawerOpen(false)
    }
    document.addEventListener('keydown', onKeyDown)
    return () => {
      document.body.style.overflow = previous
      document.removeEventListener('keydown', onKeyDown)
    }
  }, [drawerOpen])

  const initials = (user?.login ?? '?').slice(0, 1).toUpperCase()
  const visibleNav = navItems.filter((item) => !item.admin || isAdmin)

  const sidebar = (
    <div ref={drawerRef} className={styles.sidebarInner}>
      <div className={styles.brand}>
        <NavLink to="/" aria-label="Dockium — на главную">
          <Logo />
        </NavLink>
        <button
          type="button"
          className={styles.drawerClose}
          aria-label="Закрыть меню"
          onClick={() => setDrawerOpen(false)}
        >
          <X size={16} />
        </button>
      </div>

      <nav className={styles.nav} aria-label="Основная навигация">
        {visibleNav.map((item) => (
          <NavLink
            key={item.to}
            to={item.to}
            end={item.end}
            className={({ isActive }) => cx(styles.navLink, isActive && styles.active)}
          >
            <item.icon size={16} strokeWidth={1.8} aria-hidden />
            <span>{item.label}</span>
          </NavLink>
        ))}
        <div className={styles.navDivider} />
        <NavLink to="/settings" className={({ isActive }) => cx(styles.navLink, isActive && styles.active)}>
          <Settings size={16} strokeWidth={1.8} aria-hidden />
          <span>Настройки</span>
        </NavLink>
      </nav>

      <div className={styles.sidebarFooter}>
        <div className={styles.liveRow} title="Состояние потока событий">
          <span className={cx(styles.liveDot, styles[live.status])} aria-hidden />
          <span className={styles.liveLabel}>{statusLabels[live.status]}</span>
          <span className={styles.version}>v{version || '0.1.0'}</span>
        </div>
        <div className={styles.userRow}>
          <NavLink to="/settings" className={styles.userLink}>
            <span className={styles.avatar} aria-hidden>{initials}</span>
            <span className={styles.userText}>
              <span className={styles.userName}>{user?.login}</span>
              <span className={styles.userRole}>{roleLabel(user?.role ?? 'user')}</span>
            </span>
          </NavLink>
          <Button
            variant="ghost"
            size="sm"
            iconOnly
            aria-label="Выйти"
            icon={<LogOut size={15} />}
            onClick={async () => {
              await signOut()
              navigate('/login')
            }}
          />
        </div>
      </div>
    </div>
  )

  return (
    <div className={styles.layout}>
      <a className={styles.skip} href="#main-content">Перейти к содержимому</a>
      <aside className={styles.sidebar}>{sidebar}</aside>
      {drawerOpen ? (
        <div className={styles.drawer}>
          <div className={styles.drawerBackdrop} onClick={() => setDrawerOpen(false)} aria-hidden />
          <div className={styles.drawerPanel}>{sidebar}</div>
        </div>
      ) : null}
      <div className={styles.workspace}>
        <header className={styles.mobileHeader}>
          <button
            type="button"
            className={styles.menuButton}
            aria-label="Открыть меню"
            onClick={() => setDrawerOpen(true)}
          >
            <Menu size={18} />
          </button>
          <Logo compact />
          <span className={cx(styles.mobileStatus, styles[`dot-${live.status}`])} aria-hidden />
        </header>
        <main className={styles.main} id="main-content" tabIndex={-1} ref={mainRef}>
          <div className={styles.content} key={location.pathname}>
            <Outlet />
          </div>
        </main>
      </div>
    </div>
  )
}
