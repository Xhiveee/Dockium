import { useEffect, useId, useRef, useState, type ReactNode } from 'react'
import { createPortal } from 'react-dom'
import { MoreHorizontal } from 'lucide-react'
import { cx } from '../lib/cx'
import { Button } from './Button'
import styles from './Menu.module.css'

export type MenuItem = {
  id: string
  label: string
  icon?: ReactNode
  onSelect: () => void
  danger?: boolean
  disabled?: boolean
  separator?: boolean
}

type MenuProps = {
  items: MenuItem[]
  label?: string
  align?: 'start' | 'end'
  size?: 'sm' | 'md'
}

export function Menu({ items, label = 'Действия', align = 'end', size = 'sm' }: MenuProps) {
  const [open, setOpen] = useState(false)
  const [position, setPosition] = useState<{ top: number; left: number }>({ top: 0, left: 0 })
  const triggerRef = useRef<HTMLSpanElement | null>(null)
  const menuRef = useRef<HTMLDivElement | null>(null)
  const menuId = useId()

  useEffect(() => {
    if (!open) return
    const rect = triggerRef.current?.getBoundingClientRect()
    const panelWidth = 196
    if (rect) {
      const left = align === 'end' ? Math.min(rect.right - panelWidth, window.innerWidth - panelWidth - 12) : rect.left
      setPosition({
        top: Math.min(rect.bottom + 6, window.innerHeight - 320),
        left: Math.max(12, left),
      })
    }
    const onPointerDown = (event: PointerEvent) => {
      const target = event.target as Node
      if (menuRef.current?.contains(target) || triggerRef.current?.contains(target)) return
      setOpen(false)
    }
    const onKeyDown = (event: globalThis.KeyboardEvent) => {
      if (event.key === 'Escape') setOpen(false)
    }
    const onScroll = () => setOpen(false)
    document.addEventListener('pointerdown', onPointerDown, true)
    document.addEventListener('keydown', onKeyDown)
    window.addEventListener('scroll', onScroll, true)
    window.addEventListener('resize', onScroll)
    const timer = window.setTimeout(() => {
      menuRef.current?.querySelector<HTMLButtonElement>('button:not([disabled])')?.focus()
    }, 20)
    return () => {
      window.clearTimeout(timer)
      document.removeEventListener('pointerdown', onPointerDown, true)
      document.removeEventListener('keydown', onKeyDown)
      window.removeEventListener('scroll', onScroll, true)
      window.removeEventListener('resize', onScroll)
    }
  }, [open, align])

  if (!items.length) return null

  return (
    <>
      <span ref={triggerRef} className={styles.trigger}>
        <Button
          variant="ghost"
          size={size}
          iconOnly
          aria-label={label}
          aria-haspopup="menu"
          aria-expanded={open}
          aria-controls={open ? menuId : undefined}
          icon={<MoreHorizontal size={16} />}
          onClick={() => setOpen((value) => !value)}
        />
      </span>
      {open
        ? createPortal(
            <div
              ref={menuRef}
              id={menuId}
              role="menu"
              className={styles.menu}
              style={{ top: position.top, left: position.left }}
            >
              {items.map((item) => (
                <div key={item.id}>
                  {item.separator ? <div className={styles.separator} role="separator" /> : null}
                  <button
                    type="button"
                    role="menuitem"
                    disabled={item.disabled}
                    className={cx(styles.item, item.danger && styles.danger)}
                    onClick={() => {
                      setOpen(false)
                      item.onSelect()
                    }}
                  >
                    {item.icon ? <span className={styles.icon} aria-hidden>{item.icon}</span> : null}
                    <span>{item.label}</span>
                  </button>
                </div>
              ))}
            </div>,
            document.body,
          )
        : null}
    </>
  )
}
