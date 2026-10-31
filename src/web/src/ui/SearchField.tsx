import { Search, X } from 'lucide-react'
import { Input } from './Field'
import styles from './SearchField.module.css'

export function SearchField({
  value,
  onChange,
  placeholder = 'Поиск',
  ariaLabel = 'Поиск',
  className,
}: {
  value: string
  onChange: (value: string) => void
  placeholder?: string
  ariaLabel?: string
  className?: string
}) {
  return (
    <div className={className}>
      <Input
        aria-label={ariaLabel}
        value={value}
        onChange={(event) => onChange(event.target.value)}
        placeholder={placeholder}
        leading={<Search size={14} />}
        trailing={
          value ? (
            <button type="button" className={styles.clear} aria-label="Очистить поиск" onClick={() => onChange('')}>
              <X size={13} />
            </button>
          ) : null
        }
      />
    </div>
  )
}
