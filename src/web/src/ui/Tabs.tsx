import { useLayoutEffect, useRef, useState, type ReactNode } from 'react'
import { cx } from '../lib/cx'
import styles from './Tabs.module.css'

export type TabItem<T extends string> = {
  id: T
  label: ReactNode
  count?: number
  icon?: ReactNode
  disabled?: boolean
}

type TabsProps<T extends string> = {
  items: TabItem<T>[]
  value: T
  onChange: (value: T) => void
  variant?: 'underline' | 'pill'
  size?: 'sm' | 'md'
  ariaLabel?: string
  className?: string
}

export function Tabs<T extends string>({
  items,
  value,
  onChange,
  variant = 'underline',
  size = 'md',
  ariaLabel,
  className,
}: TabsProps<T>) {
  const containerRef = useRef<HTMLDivElement | null>(null)
  const buttons = useRef(new Map<string, HTMLButtonElement>())
  const [indicator, setIndicator] = useState<{ left: number; width: number; visible: boolean }>({
    left: 0,
    width: 0,
    visible: false,
  })

  useLayoutEffect(() => {
    const measure = () => {
      const active = buttons.current.get(value)
      const container = containerRef.current
      if (!active || !container) return
      const containerRect = container.getBoundingClientRect()
      const activeRect = active.getBoundingClientRect()
      setIndicator({
        left: activeRect.left - containerRect.left,
        width: activeRect.width,
        visible: true,
      })
      if (activeRect.right > containerRect.right) {
        container.scrollLeft += activeRect.right - containerRect.right + 8
      } else if (activeRect.left < containerRect.left) {
        container.scrollLeft -= containerRect.left - activeRect.left + 8
      }
    }
    measure()
    const observer = new ResizeObserver(measure)
    if (containerRef.current) observer.observe(containerRef.current)
    return () => observer.disconnect()
  }, [value, items])

  return (
    <div
      ref={containerRef}
      className={cx(styles.tabs, styles[variant], styles[size], className)}
      role="tablist"
      aria-label={ariaLabel}
    >
      <span
        className={styles.indicator}
        style={{
          transform: `translateX(${indicator.left}px)`,
          width: indicator.width,
          opacity: indicator.visible ? 1 : 0,
        }}
        aria-hidden
      />
      {items.map((item) => (
        <button
          key={item.id}
          ref={(element) => {
            if (element) buttons.current.set(item.id, element)
            else buttons.current.delete(item.id)
          }}
          type="button"
          role="tab"
          aria-selected={item.id === value}
          disabled={item.disabled}
          className={cx(styles.tab, item.id === value && styles.active)}
          onClick={() => onChange(item.id)}
        >
          {item.icon ? <span className={styles.icon} aria-hidden>{item.icon}</span> : null}
          <span>{item.label}</span>
          {typeof item.count === 'number' ? <span className={styles.count}>{item.count}</span> : null}
        </button>
      ))}
    </div>
  )
}
