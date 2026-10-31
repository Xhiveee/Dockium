import { InstanceStatus } from '../gen/panel/v1/panel_pb'

export type Tone = 'neutral' | 'success' | 'warning' | 'danger' | 'info'

const BYTE_UNITS = ['Б', 'КиБ', 'МиБ', 'ГиБ', 'ТиБ', 'ПиБ']

export function toNumber(value: bigint | number | string | null | undefined): number {
  if (typeof value === 'bigint') return Number(value)
  if (typeof value === 'number') return value
  if (typeof value === 'string') return Number(value)
  return 0
}

export function formatBytes(value: bigint | number | null | undefined, fraction = 1): string {
  const bytes = toNumber(value)
  if (!Number.isFinite(bytes) || bytes <= 0) return '0 Б'
  let size = bytes
  let unit = 0
  while (size >= 1024 && unit < BYTE_UNITS.length - 1) {
    size /= 1024
    unit += 1
  }
  const digits = size >= 100 || unit === 0 ? 0 : fraction
  return `${size.toLocaleString('ru-RU', { maximumFractionDigits: digits, minimumFractionDigits: 0 })} ${BYTE_UNITS[unit]}`
}

export function formatRate(bitsPerSecond: bigint | number | null | undefined): string {
  const value = toNumber(bitsPerSecond)
  if (!Number.isFinite(value) || value <= 0) return '0 Б/с'
  return `${formatBytes(value)}/с`
}

export function formatPercent(value: number | null | undefined, fraction = 0): string {
  const number = Number(value ?? 0)
  if (!Number.isFinite(number)) return '0%'
  return `${number.toLocaleString('ru-RU', { maximumFractionDigits: fraction, minimumFractionDigits: fraction > 0 ? fraction : 0 })}%`
}

export function formatNumber(value: number | bigint | null | undefined): string {
  return toNumber(value).toLocaleString('ru-RU')
}

const dateFormatter = new Intl.DateTimeFormat('ru-RU', {
  day: '2-digit',
  month: 'short',
  year: 'numeric',
  hour: '2-digit',
  minute: '2-digit',
})

const shortDateFormatter = new Intl.DateTimeFormat('ru-RU', {
  day: '2-digit',
  month: 'short',
  hour: '2-digit',
  minute: '2-digit',
})

export function formatDate(value: bigint | number | null | undefined, short = false): string {
  const millis = toNumber(value)
  if (!millis || !Number.isFinite(millis)) return '—'
  const date = new Date(millis)
  if (Number.isNaN(date.getTime())) return '—'
  return (short ? shortDateFormatter : dateFormatter).format(date)
}

export function formatRelative(value: bigint | number | null | undefined): string {
  const millis = toNumber(value)
  if (!millis) return '—'
  const difference = Date.now() - millis
  if (difference < 45_000) return 'только что'
  const seconds = Math.floor(difference / 1000)
  const minutes = Math.floor(seconds / 60)
  if (minutes < 60) return `${minutes} ${plural(minutes, ['минуту', 'минуты', 'минут'])} назад`
  const hours = Math.floor(minutes / 60)
  if (hours < 24) return `${hours} ${plural(hours, ['час', 'часа', 'часов'])} назад`
  const days = Math.floor(hours / 24)
  if (days < 30) return `${days} ${plural(days, ['день', 'дня', 'дней'])} назад`
  return formatDate(millis, true)
}

export function formatUptime(startedAt: bigint | number | null | undefined): string {
  const millis = toNumber(startedAt)
  if (!millis) return '—'
  return formatElapsed(Date.now() - millis)
}

export function formatElapsed(milliseconds: number): string {
  if (milliseconds < 0 || !Number.isFinite(milliseconds)) return '—'
  const totalSeconds = Math.floor(milliseconds / 1000)
  if (totalSeconds < 60) return `${totalSeconds} с`
  const minutes = Math.floor(totalSeconds / 60)
  if (minutes < 60) return `${minutes} мин`
  const hours = Math.floor(minutes / 60)
  const restMinutes = minutes % 60
  if (hours < 24) return restMinutes > 0 ? `${hours} ч ${restMinutes} мин` : `${hours} ч`
  const days = Math.floor(hours / 24)
  const restHours = hours % 24
  return restHours > 0 ? `${days} д ${restHours} ч` : `${days} д`
}

export function plural(count: number, forms: [string, string, string]): string {
  const abs = Math.abs(count) % 100
  const last = abs % 10
  if (abs > 10 && abs < 20) return forms[2]
  if (last > 1 && last < 5) return forms[1]
  if (last === 1) return forms[0]
  return forms[2]
}

export function formatDateTimeLocalInput(millis: number): string {
  const date = new Date(millis)
  const pad = (part: number) => String(part).padStart(2, '0')
  return `${date.getFullYear()}-${pad(date.getMonth() + 1)}-${pad(date.getDate())}T${pad(date.getHours())}:${pad(date.getMinutes())}`
}

export type StatusInfo = {
  label: string
  tone: Tone
  busy: boolean
  hint?: string
}

export function instanceStatusInfo(status: InstanceStatus, exitCode = 0, lastError = ''): StatusInfo {
  switch (status) {
    case InstanceStatus.STARTING:
      return { label: 'Запускается', tone: 'info', busy: true }
    case InstanceStatus.RUNNING:
      return { label: 'Работает', tone: 'success', busy: false }
    case InstanceStatus.STOPPING:
      return { label: 'Останавливается', tone: 'warning', busy: true }
    case InstanceStatus.EXITED:
      return exitCode !== 0
        ? { label: 'Завершён', tone: 'warning', busy: false, hint: `код ${exitCode}` }
        : { label: 'Остановлен', tone: 'neutral', busy: false }
    case InstanceStatus.FAILED:
      return { label: 'Ошибка', tone: 'danger', busy: false, hint: lastError || undefined }
    case InstanceStatus.STOPPED:
      return { label: 'Остановлен', tone: 'neutral', busy: false }
    default:
      return { label: 'Неизвестно', tone: 'neutral', busy: false }
  }
}

export function isRunning(status: InstanceStatus): boolean {
  return status === InstanceStatus.RUNNING
}

export function isTransition(status: InstanceStatus): boolean {
  return status === InstanceStatus.STARTING || status === InstanceStatus.STOPPING
}

export function truncateMiddle(value: string, max = 24): string {
  if (value.length <= max) return value
  const half = Math.floor((max - 1) / 2)
  return `${value.slice(0, half)}…${value.slice(-half)}`
}

export function roleLabel(role: string): string {
  return role === 'admin' ? 'Администратор' : 'Пользователь'
}

export function splitCommandLine(input: string): string[] {
  const result: string[] = []
  let current = ''
  let quote: '"' | "'" | null = null
  let escaped = false
  for (const character of input) {
    if (escaped) {
      current += character
      escaped = false
      continue
    }
    if (character === '\\' && quote !== "'") {
      escaped = true
      continue
    }
    if (quote) {
      if (character === quote) {
        quote = null
      } else {
        current += character
      }
      continue
    }
    if (character === '"' || character === "'") {
      quote = character
      continue
    }
    if (/\s/.test(character)) {
      if (current) {
        result.push(current)
        current = ''
      }
      continue
    }
    current += character
  }
  if (current) result.push(current)
  return result
}

export function joinCommandLine(argumentsList: string[]): string {
  return argumentsList
    .map((argument) => (/\s/.test(argument) ? `"${argument.replaceAll('"', '\\"')}"` : argument))
    .join(' ')
}

export function describePorts(
  ports: { hostPort: number; containerPort: number; protocol: string }[],
): string {
  if (!ports.length) return '—'
  return ports.map((port) => `${port.hostPort}→${port.containerPort}/${port.protocol || 'tcp'}`).join(', ')
}

export function formatMode(mode: number): string {
  const padded = (mode & 0o777).toString(8).padStart(3, '0')
  return padded
}

export function isImageReference(value: string): boolean {
  return /^[a-zA-Z0-9][a-zA-Z0-9._/:@-]{0,254}$/.test(value.trim())
}

export async function copyText(value: string): Promise<boolean> {
  try {
    await navigator.clipboard.writeText(value)
    return true
  } catch {
    return false
  }
}
