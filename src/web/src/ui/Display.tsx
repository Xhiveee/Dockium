import { Check, Copy, type LucideIcon } from 'lucide-react'
import { useMemo, useState, type ReactNode } from 'react'
import { cx } from '../lib/cx'
import { copyText } from '../lib/format'
import { Button } from './Button'
import { useToast } from './Toast'
import styles from './Display.module.css'

export type BadgeTone = 'neutral' | 'success' | 'warning' | 'danger' | 'info' | 'accent'

export function Badge({
  tone = 'neutral',
  children,
  dot = false,
  className,
}: {
  tone?: BadgeTone
  children: ReactNode
  dot?: boolean
  className?: string
}) {
  return (
    <span className={cx(styles.badge, styles[tone], className)}>
      {dot ? <span className={styles.badgeDot} aria-hidden /> : null}
      {children}
    </span>
  )
}

export function StatusDot({ tone = 'neutral', pulse = false }: { tone?: BadgeTone; pulse?: boolean }) {
  return <span className={cx(styles.statusDot, styles[tone], pulse && styles.pulse)} aria-hidden />
}

export function Meter({
  label,
  value,
  percent,
  tone = 'accent',
  detail,
  compact = false,
}: {
  label?: ReactNode
  value?: ReactNode
  percent: number
  tone?: 'accent' | 'success' | 'warning' | 'danger' | 'info'
  detail?: ReactNode
  compact?: boolean
}) {
  const clamped = Math.max(0, Math.min(100, Number.isFinite(percent) ? percent : 0))
  return (
    <div className={cx(styles.meter, compact && styles.meterCompact)}>
      {!compact && (label || value) ? (
        <div className={styles.meterHead}>
          {label ? <span className={styles.meterLabel}>{label}</span> : <span />}
          {value ? <span className={styles.meterValue}>{value}</span> : null}
        </div>
      ) : null}
      <div
        className={styles.track}
        role="progressbar"
        aria-valuenow={Math.round(clamped)}
        aria-valuemin={0}
        aria-valuemax={100}
        aria-label={typeof label === 'string' ? label : undefined}
      >
        <span className={cx(styles.fill, styles[tone])} style={{ width: `${clamped}%` }} />
      </div>
      {detail ? <p className={styles.meterDetail}>{detail}</p> : null}
    </div>
  )
}

export function Sparkline({
  values,
  tone = 'accent',
  height = 30,
  label,
}: {
  values: number[]
  tone?: 'accent' | 'success' | 'warning' | 'danger' | 'info' | 'neutral'
  height?: number
  label?: string
}) {
  const geometry = useMemo(() => {
    const data = values.length ? values : [0]
    if (data.length < 2) {
      const y = height - 2
      return { line: `0,${y} 100,${y}`, area: '' }
    }
    const max = Math.max(1, ...data)
    const step = 100 / (data.length - 1)
    const points = data.map((value, index) => {
      const x = index * step
      const y = height - 2 - (Math.max(0, value) / max) * (height - 4)
      return `${x.toFixed(2)},${y.toFixed(2)}`
    })
    return { line: points.join(' '), area: `0,${height} ${points.join(' ')} 100,${height}` }
  }, [values, height])

  return (
    <svg
      className={cx(styles.sparkline, styles[tone])}
      viewBox={`0 0 100 ${height}`}
      preserveAspectRatio="none"
      style={{ height: `${height}px` }}
      role="img"
      aria-label={label ?? 'График нагрузки'}
    >
      {geometry.area ? <polygon points={geometry.area} className={styles.sparkArea} /> : null}
      <polyline points={geometry.line} className={styles.sparkLine} vectorEffect="non-scaling-stroke" />
    </svg>
  )
}

export function EmptyState({
  icon: Icon,
  title,
  description,
  action,
  compact = false,
  className,
}: {
  icon?: LucideIcon
  title: string
  description?: ReactNode
  action?: ReactNode
  compact?: boolean
  className?: string
}) {
  return (
    <div className={cx(styles.empty, compact && styles.emptyCompact, className)}>
      {Icon ? (
        <span className={styles.emptyIcon} aria-hidden>
          <Icon size={18} strokeWidth={1.7} />
        </span>
      ) : null}
      <p className={styles.emptyTitle}>{title}</p>
      {description ? <p className={styles.emptyDescription}>{description}</p> : null}
      {action ? <div className={styles.emptyAction}>{action}</div> : null}
    </div>
  )
}

export function Skeleton({
  width = '100%',
  height = 14,
  radius = 6,
  className,
}: {
  width?: number | string
  height?: number | string
  radius?: number
  className?: string
}) {
  return (
    <span
      className={cx(styles.skeleton, className)}
      style={{ width, height, borderRadius: radius }}
      aria-hidden
    />
  )
}

export function InlineNotice({
  tone = 'warning',
  icon: Icon,
  title,
  children,
  action,
}: {
  tone?: 'warning' | 'danger' | 'info' | 'success'
  icon?: LucideIcon
  title: ReactNode
  children?: ReactNode
  action?: ReactNode
}) {
  return (
    <div className={cx(styles.notice, styles[`notice-${tone}`])} role={tone === 'danger' ? 'alert' : 'status'}>
      {Icon ? (
        <span className={styles.noticeIcon} aria-hidden>
          <Icon size={16} />
        </span>
      ) : null}
      <div className={styles.noticeBody}>
        <p className={styles.noticeTitle}>{title}</p>
        {children ? <div className={styles.noticeText}>{children}</div> : null}
      </div>
      {action ? <div className={styles.noticeAction}>{action}</div> : null}
    </div>
  )
}

export function PageHeader({
  title,
  badge,
  description,
  actions,
  back,
  className,
}: {
  title: ReactNode
  badge?: ReactNode
  description?: ReactNode
  actions?: ReactNode
  back?: ReactNode
  className?: string
}) {
  return (
    <div className={cx(styles.pageHeader, className)}>
      <div className={styles.pageHeading}>
        {back}
        <div className={styles.pageTitleRow}>
          <h1 className={styles.pageTitle}>{title}</h1>
          {badge}
        </div>
        {description ? <div className={styles.pageDescription}>{description}</div> : null}
      </div>
      {actions ? <div className={styles.pageActions}>{actions}</div> : null}
    </div>
  )
}

export function KeyValue({ items, columns = 2 }: { items: { label: string; value: ReactNode }[]; columns?: number }) {
  return (
    <dl className={styles.kv} style={{ gridTemplateColumns: `repeat(${columns}, minmax(0, 1fr))` }}>
      {items.map((item) => (
        <div key={item.label} className={styles.kvItem}>
          <dt className={styles.kvLabel}>{item.label}</dt>
          <dd className={styles.kvValue}>{item.value}</dd>
        </div>
      ))}
    </dl>
  )
}

export function CopyField({ value, label, mono = true }: { value: string; label?: string; mono?: boolean }) {
  const [copied, setCopied] = useState(false)
  const toast = useToast()
  return (
    <div className={styles.copyField}>
      {label ? <span className={styles.copyLabel}>{label}</span> : null}
      <div className={styles.copyRow}>
        <code className={cx(styles.copyValue, mono && 'mono')}>{value}</code>
        <Button
          variant="ghost"
          size="sm"
          iconOnly
          aria-label="Скопировать"
          icon={copied ? <Check size={14} /> : <Copy size={14} />}
          onClick={async () => {
            if (await copyText(value)) {
              setCopied(true)
              window.setTimeout(() => setCopied(false), 1600)
            } else {
              toast.error('Не удалось скопировать', 'Выделите значение вручную')
            }
          }}
        />
      </div>
    </div>
  )
}

export function CountBadge({ value }: { value: number }) {
  return <span className={styles.countBadge}>{value.toLocaleString('ru-RU')}</span>
}
