import { Check } from 'lucide-react'
import type { ReactNode } from 'react'
import { cx } from '../lib/cx'
import styles from './Controls.module.css'

type SwitchProps = {
  checked: boolean
  onChange: (checked: boolean) => void
  label: ReactNode
  description?: ReactNode
  disabled?: boolean
  compact?: boolean
  id?: string
  ariaLabel?: string
}

export function Switch({ checked, onChange, label, description, disabled, compact = false, id, ariaLabel }: SwitchProps) {
  return (
    <button
      type="button"
      id={id}
      role="switch"
      aria-checked={checked}
      aria-label={ariaLabel}
      disabled={disabled}
      className={cx(styles.switchRow, compact && styles.compact, disabled && styles.disabled)}
      onClick={() => onChange(!checked)}
    >
      {compact ? null : (
        <span className={styles.switchText}>
          <span className={styles.switchLabel}>{label}</span>
          {description ? <span className={styles.switchDescription}>{description}</span> : null}
        </span>
      )}
      <span className={cx(styles.track, checked && styles.trackOn)} aria-hidden>
        <span className={styles.thumb} />
      </span>
      {compact ? <span className={styles.compactLabel}>{label}</span> : null}
    </button>
  )
}

type CheckboxProps = {
  checked: boolean
  onChange: (checked: boolean) => void
  label: ReactNode
  disabled?: boolean
  id?: string
  ariaLabel?: string
}

export function Checkbox({ checked, onChange, label, disabled, id, ariaLabel }: CheckboxProps) {
  return (
    <button
      type="button"
      id={id}
      role="checkbox"
      aria-checked={checked}
      aria-label={ariaLabel}
      disabled={disabled}
      className={cx(styles.checkboxRow, disabled && styles.disabled)}
      onClick={() => onChange(!checked)}
    >
      <span className={cx(styles.checkbox, checked && styles.checkboxOn)} aria-hidden>
        {checked ? <Check size={12} strokeWidth={2.6} /> : null}
      </span>
      <span className={styles.checkboxLabel}>{label}</span>
    </button>
  )
}
