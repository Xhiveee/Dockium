import { describe, expect, it } from 'vitest'
import {
  describePorts,
  formatBytes,
  formatMode,
  formatPercent,
  joinCommandLine,
  plural,
  splitCommandLine,
  toNumber,
  truncateMiddle,
} from './format'

describe('formatBytes', () => {
  it('formats zero and small values', () => {
    expect(formatBytes(0)).toBe('0 Б')
    expect(formatBytes(512)).toBe('512 Б')
  })

  it('formats binary units', () => {
    expect(formatBytes(1024)).toBe('1 КиБ')
    expect(formatBytes(1536)).toBe('1,5 КиБ')
    expect(formatBytes(1024 * 1024 * 1024)).toBe('1 ГиБ')
    expect(formatBytes(1_500_000_000)).toContain('ГиБ')
  })

  it('accepts bigint values', () => {
    expect(formatBytes(2048n)).toBe('2 КиБ')
  })
})

describe('formatting helpers', () => {
  it('formats percentages', () => {
    expect(formatPercent(12.345)).toBe('12%')
    expect(formatPercent(12.345, 1)).toBe('12,3%')
    expect(formatPercent(null)).toBe('0%')
  })

  it('converts bigint and string numbers', () => {
    expect(toNumber(42n)).toBe(42)
    expect(toNumber('7')).toBe(7)
    expect(toNumber(undefined)).toBe(0)
  })

  it('describes ports', () => {
    expect(describePorts([])).toBe('—')
    expect(describePorts([{ hostPort: 8080, containerPort: 80, protocol: 'tcp' }])).toBe('8080→80/tcp')
  })

  it('pluralizes russian nouns', () => {
    expect(plural(1, ['минута', 'минуты', 'минут'])).toBe('минута')
    expect(plural(2, ['минута', 'минуты', 'минут'])).toBe('минуты')
    expect(plural(5, ['минута', 'минуты', 'минут'])).toBe('минут')
    expect(plural(11, ['минута', 'минуты', 'минут'])).toBe('минут')
  })

  it('formats file modes', () => {
    expect(formatMode(0o644)).toBe('644')
    expect(formatMode(0o755)).toBe('755')
  })

  it('truncates middle', () => {
    expect(truncateMiddle('1234567890', 5)).toBe('12…90')
    expect(truncateMiddle('short', 10)).toBe('short')
  })
})

describe('command line parsing', () => {
  it('splits plain arguments', () => {
    expect(splitCommandLine('java -jar server.jar')).toEqual(['java', '-jar', 'server.jar'])
  })

  it('keeps quoted arguments together', () => {
    expect(splitCommandLine('sh -c "echo hello world"')).toEqual(['sh', '-c', 'echo hello world'])
    expect(splitCommandLine("python -c 'print(1)'")).toEqual(['python', '-c', 'print(1)'])
  })

  it('supports escaped quotes and spaces', () => {
    expect(splitCommandLine('echo "a \\"b\\" c"')).toEqual(['echo', 'a "b" c'])
  })

  it('round-trips through join', () => {
    const args = ['sh', '-c', 'echo hello']
    expect(splitCommandLine(joinCommandLine(args))).toEqual(args)
  })

  it('ignores extra whitespace', () => {
    expect(splitCommandLine('  java   -jar  app.jar ')).toEqual(['java', '-jar', 'app.jar'])
  })
})
