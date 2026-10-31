import { cx } from '../lib/cx'
import styles from './Logo.module.css'

export function LogoMark({ size = 28 }: { size?: number }) {
  return (
    <svg
      width={size}
      height={size}
      viewBox="0 0 28 28"
      fill="none"
      aria-hidden
      className={styles.mark}
    >
      <rect x="3" y="4" width="22" height="5" rx="1.6" fill="currentColor" />
      <rect x="3" y="11.5" width="22" height="5" rx="1.6" fill="currentColor" opacity="0.62" />
      <rect x="3" y="19" width="14" height="5" rx="1.6" fill="currentColor" opacity="0.34" />
      <circle cx="22.6" cy="21.5" r="1.7" fill="var(--success)" />
    </svg>
  )
}

export function Logo({ compact = false }: { compact?: boolean }) {
  return (
    <span className={cx(styles.logo, compact && styles.compact)}>
      <LogoMark />
      {compact ? null : (
        <span className={styles.word}>
          dockium<span className={styles.dot}>.</span>
        </span>
      )}
    </span>
  )
}
