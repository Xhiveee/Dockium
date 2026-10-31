import type { ReactNode } from 'react'
import { cx } from '../lib/cx'
import styles from './Panel.module.css'

export function Panel({
  title,
  icon,
  action,
  children,
  flush = false,
  className,
}: {
  title?: ReactNode
  icon?: ReactNode
  action?: ReactNode
  children: ReactNode
  flush?: boolean
  className?: string
}) {
  return (
    <section className={cx(styles.panel, className)}>
      {title ? (
        <header className={styles.header}>
          <div className={styles.titleRow}>
            {icon ? <span className={styles.icon} aria-hidden>{icon}</span> : null}
            <h2 className={styles.title}>{title}</h2>
          </div>
          {action ? <div className={styles.action}>{action}</div> : null}
        </header>
      ) : null}
      <div className={cx(styles.body, flush && styles.flush)}>{children}</div>
    </section>
  )
}
