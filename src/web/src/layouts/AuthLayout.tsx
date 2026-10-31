import type { ReactNode } from 'react'
import { useSession } from '../data/session'
import { Logo, LogoMark } from '../ui/Logo'
import styles from './AuthLayout.module.css'

export function AuthLayout({ children }: { children: ReactNode }) {
  const { setup, initialized, version } = useSession()
  return (
    <div className={styles.page}>
      <aside className={styles.aside}>
        <div className={styles.brand}>
          <Logo />
        </div>
        <div className={styles.statement}>
          <LogoMark size={34} />
          <h1 className={styles.headline}>Экземпляры, консоль и файлы на ваших Docker-хостах.</h1>
          <p className={styles.sub}>
            Dockium связывает панель и агентов постоянным соединением, поэтому состояние
            экземпляров и метрики приходят без перезагрузки страницы.
          </p>
        </div>
        <div className={styles.asideFooter}>
          <span className={styles.statusLine}>
            <span className={styles.dot} aria-hidden />
            {initialized ? 'Панель настроена' : setup ? 'Первый запуск' : 'Проверка панели'}
          </span>
          <span className={styles.asideVersion}>v{version || '0.1.0'}</span>
        </div>
      </aside>
      <main className={styles.main}>
        <div className={styles.card}>{children}</div>
        <span className={styles.mobileVersion}>Dockium · v{version || '0.1.0'}</span>
      </main>
    </div>
  )
}
