import {
  forwardRef,
  type InputHTMLAttributes,
  type ReactNode,
  type SelectHTMLAttributes,
  type TextareaHTMLAttributes,
} from 'react'
import { ChevronDown } from 'lucide-react'
import { cx } from '../lib/cx'
import styles from './Field.module.css'

type FieldProps = {
  label?: ReactNode
  htmlFor?: string
  hint?: ReactNode
  error?: ReactNode
  required?: boolean
  optional?: boolean
  action?: ReactNode
  children: ReactNode
  className?: string
}

export function Field({ label, htmlFor, hint, error, required, optional, action, children, className }: FieldProps) {
  return (
    <div className={cx(styles.field, className)}>
      {label || action ? (
        <div className={styles.labelRow}>
          {label ? (
            <label className={styles.label} htmlFor={htmlFor}>
              {label}
              {required ? <span className={styles.required} aria-hidden> *</span> : null}
              {optional ? <span className={styles.optional}>необязательно</span> : null}
            </label>
          ) : <span />}
          {action}
        </div>
      ) : null}
      {children}
      {error ? (
        <p className={styles.error} role="alert">{error}</p>
      ) : hint ? (
        <p className={styles.hint}>{hint}</p>
      ) : null}
    </div>
  )
}

type InputProps = InputHTMLAttributes<HTMLInputElement> & {
  mono?: boolean
  invalid?: boolean
  leading?: ReactNode
  trailing?: ReactNode
}

export const Input = forwardRef<HTMLInputElement, InputProps>(function Input(
  { className, mono, invalid, leading, trailing, ...rest },
  ref,
) {
  if (!leading && !trailing) {
    return (
      <input
        {...rest}
        ref={ref}
        className={cx(styles.control, mono && styles.mono, invalid && styles.invalid, className)}
        aria-invalid={invalid || undefined}
      />
    )
  }
  return (
    <span className={cx(styles.wrap, invalid && styles.invalid, className)}>
      {leading ? <span className={styles.adornment}>{leading}</span> : null}
      <input
        {...rest}
        ref={ref}
        className={cx(styles.controlInner, mono && styles.mono)}
        aria-invalid={invalid || undefined}
      />
      {trailing ? <span className={styles.adornment}>{trailing}</span> : null}
    </span>
  )
})

type TextareaProps = TextareaHTMLAttributes<HTMLTextAreaElement> & {
  mono?: boolean
  invalid?: boolean
}

export const Textarea = forwardRef<HTMLTextAreaElement, TextareaProps>(function Textarea(
  { className, mono, invalid, ...rest },
  ref,
) {
  return (
    <textarea
      {...rest}
      ref={ref}
      className={cx(styles.control, styles.textarea, mono && styles.mono, invalid && styles.invalid, className)}
      aria-invalid={invalid || undefined}
    />
  )
})

type SelectProps = SelectHTMLAttributes<HTMLSelectElement> & {
  invalid?: boolean
}

export const Select = forwardRef<HTMLSelectElement, SelectProps>(function Select(
  { className, invalid, children, ...rest },
  ref,
) {
  return (
    <span className={cx(styles.selectWrap, invalid && styles.invalid, className)}>
      <select {...rest} ref={ref} className={styles.select} aria-invalid={invalid || undefined}>
        {children}
      </select>
      <ChevronDown size={14} className={styles.selectIcon} aria-hidden />
    </span>
  )
})
