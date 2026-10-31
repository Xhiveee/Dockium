import {
  forwardRef,
  useCallback,
  useEffect,
  useRef,
  type ButtonHTMLAttributes,
  type ReactNode,
} from 'react'
import { cx } from '../lib/cx'
import styles from './Button.module.css'

export type ButtonVariant = 'primary' | 'secondary' | 'ghost' | 'subtle' | 'danger' | 'dangerSolid'
export type ButtonSize = 'sm' | 'md' | 'lg'

export type ButtonProps = ButtonHTMLAttributes<HTMLButtonElement> & {
  variant?: ButtonVariant
  size?: ButtonSize
  icon?: ReactNode
  iconPosition?: 'start' | 'end'
  iconOnly?: boolean
  loading?: boolean
  fullWidth?: boolean
}

const PRESS_DURATION = 90

export const Button = forwardRef<HTMLButtonElement, ButtonProps>(function Button(
  {
    variant = 'secondary',
    size = 'md',
    icon,
    iconPosition = 'start',
    iconOnly = false,
    loading = false,
    fullWidth = false,
    className,
    children,
    disabled,
    type = 'button',
    onPointerDown,
    onPointerUp,
    onPointerLeave,
    onPointerCancel,
    onKeyDown,
    onKeyUp,
    ...rest
  },
  forwardedRef,
) {
  const innerRef = useRef<HTMLButtonElement | null>(null)
  const pressStarted = useRef(0)
  const releaseTimer = useRef<number | null>(null)

  const setRefs = useCallback(
    (element: HTMLButtonElement | null) => {
      innerRef.current = element
      if (typeof forwardedRef === 'function') forwardedRef(element)
      else if (forwardedRef) forwardedRef.current = element
    },
    [forwardedRef],
  )

  useEffect(
    () => () => {
      if (releaseTimer.current) window.clearTimeout(releaseTimer.current)
    },
    [],
  )

  const press = useCallback(() => {
    const element = innerRef.current
    if (!element || element.disabled) return
    element.getAnimations().forEach((animation) => animation.cancel())
    element.animate(
      [
        { transform: 'scale(1) translateY(0)' },
        { transform: 'scale(0.965) translateY(1px)' },
      ],
      { duration: PRESS_DURATION, easing: 'cubic-bezier(0.2, 0.7, 0.3, 1)', fill: 'forwards' },
    )
    pressStarted.current = performance.now()
  }, [])

  const release = useCallback(() => {
    const element = innerRef.current
    if (!element || !pressStarted.current) return
    const elapsed = performance.now() - pressStarted.current
    pressStarted.current = 0
    const start = () => {
      element.animate(
        [
          { transform: 'scale(0.965) translateY(1px)' },
          { transform: 'scale(1.008) translateY(0)' },
          { transform: 'scale(1) translateY(0)' },
        ],
        { duration: 260, easing: 'cubic-bezier(0.34, 1.3, 0.64, 1)', fill: 'forwards' },
      )
    }
    if (elapsed < PRESS_DURATION) {
      if (releaseTimer.current) window.clearTimeout(releaseTimer.current)
      releaseTimer.current = window.setTimeout(start, PRESS_DURATION - elapsed)
    } else {
      start()
    }
  }, [])

  const iconNode = loading ? (
    <span className={styles.spinner} aria-hidden />
  ) : icon ? (
    <span className={styles.icon} aria-hidden>
      {icon}
    </span>
  ) : null

  return (
    <button
      {...rest}
      ref={setRefs}
      type={type}
      disabled={disabled || loading}
      aria-busy={loading || undefined}
      className={cx(
        styles.button,
        styles[variant],
        styles[size],
        iconOnly && styles.iconOnly,
        fullWidth && styles.fullWidth,
        className,
      )}
      onPointerDown={(event) => {
        onPointerDown?.(event)
        if (event.button === 0) press()
      }}
      onPointerUp={(event) => {
        onPointerUp?.(event)
        release()
      }}
      onPointerLeave={(event) => {
        onPointerLeave?.(event)
        release()
      }}
      onPointerCancel={(event) => {
        onPointerCancel?.(event)
        release()
      }}
      onKeyDown={(event) => {
        onKeyDown?.(event)
        if (event.key === ' ' || event.key === 'Enter') press()
      }}
      onKeyUp={(event) => {
        onKeyUp?.(event)
        if (event.key === ' ' || event.key === 'Enter') release()
      }}
    >
      {iconOnly ? iconNode : <>
        {iconPosition === 'start' ? iconNode : null}
        <span className={styles.label}>{children}</span>
        {iconPosition === 'end' ? iconNode : null}
      </>}
    </button>
  )
})
