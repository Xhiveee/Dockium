import {
  createContext,
  useCallback,
  useContext,
  useEffect,
  useMemo,
  useRef,
  useState,
  type ReactNode,
} from 'react'
import { createPortal } from 'react-dom'
import { AlertTriangle, CheckCircle2, Info, X, XCircle } from 'lucide-react'
import styles from './Toast.module.css'

export type ToastTone = 'success' | 'error' | 'info' | 'warning'

type ToastInput = {
  tone: ToastTone
  title: string
  description?: string
  duration?: number
}

type ToastItem = ToastInput & {
  id: number
  key: string
  createdAt: number
  closing?: boolean
}

type ToastApi = {
  push: (toast: ToastInput) => void
  success: (title: string, description?: string) => void
  error: (title: string, description?: string) => void
  info: (title: string, description?: string) => void
  warning: (title: string, description?: string) => void
  dismiss: (id: number) => void
}

const ToastContext = createContext<ToastApi | null>(null)

const DEFAULT_DURATION: Record<ToastTone, number> = {
  success: 4500,
  info: 5200,
  warning: 6500,
  error: 8200,
}

const ICONS: Record<ToastTone, ReactNode> = {
  success: <CheckCircle2 size={17} aria-hidden />,
  error: <XCircle size={17} aria-hidden />,
  info: <Info size={17} aria-hidden />,
  warning: <AlertTriangle size={17} aria-hidden />,
}

export function ToastProvider({ children }: { children: ReactNode }) {
  const [items, setItems] = useState<ToastItem[]>([])
  const counter = useRef(0)
  const timers = useRef(new Map<number, number>())
  const itemsRef = useRef<ToastItem[]>([])

  useEffect(() => {
    itemsRef.current = items
  }, [items])

  const dismiss = useCallback((id: number) => {
    setItems((current) => current.filter((item) => item.id !== id))
    const timer = timers.current.get(id)
    if (timer) {
      window.clearTimeout(timer)
      timers.current.delete(id)
    }
  }, [])

  const scheduleDismiss = useCallback(
    (id: number, duration: number) => {
      const existing = timers.current.get(id)
      if (existing) window.clearTimeout(existing)
      const timer = window.setTimeout(() => dismiss(id), duration)
      timers.current.set(id, timer)
    },
    [dismiss],
  )

  const push = useCallback(
    (input: ToastInput) => {
      const key = `${input.tone}:${input.title}:${input.description ?? ''}`
      const duration = input.duration ?? DEFAULT_DURATION[input.tone]
      const existing = itemsRef.current.find((item) => item.key === key)
      if (existing) {
        scheduleDismiss(existing.id, duration)
        return
      }
      counter.current += 1
      const id = counter.current
      const item: ToastItem = {
        ...input,
        id,
        key,
        createdAt: Date.now(),
        duration,
      }
      setItems((current) => {
        const next = [...current, item]
        return next.length > 4 ? next.slice(next.length - 4) : next
      })
      scheduleDismiss(id, duration)
    },
    [scheduleDismiss],
  )

  const api = useMemo<ToastApi>(
    () => ({
      push,
      dismiss,
      success: (title, description) => push({ tone: 'success', title, description }),
      error: (title, description) => push({ tone: 'error', title, description }),
      info: (title, description) => push({ tone: 'info', title, description }),
      warning: (title, description) => push({ tone: 'warning', title, description }),
    }),
    [push, dismiss],
  )

  useEffect(() => {
    const currentTimers = timers.current
    return () => {
      currentTimers.forEach((timer) => window.clearTimeout(timer))
      currentTimers.clear()
    }
  }, [])

  return (
    <ToastContext.Provider value={api}>
      {children}
      {createPortal(
        <div className={styles.viewport} aria-live="polite" aria-relevant="additions">
          {items.map((item) => (
            <div
              key={item.id}
              className={`${styles.toast} ${styles[item.tone]}`}
              role={item.tone === 'error' ? 'alert' : 'status'}
            >
              <span className={styles.icon}>{ICONS[item.tone]}</span>
              <div className={styles.body}>
                <p className={styles.title}>{item.title}</p>
                {item.description ? <p className={styles.description}>{item.description}</p> : null}
              </div>
              <button
                type="button"
                className={styles.close}
                aria-label="Закрыть уведомление"
                onClick={() => dismiss(item.id)}
              >
                <X size={14} aria-hidden />
              </button>
            </div>
          ))}
        </div>,
        document.body,
      )}
    </ToastContext.Provider>
  )
}

export function useToast(): ToastApi {
  const context = useContext(ToastContext)
  if (!context) throw new Error('useToast must be used inside ToastProvider')
  return context
}
