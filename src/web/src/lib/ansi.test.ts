import { describe, expect, it } from 'vitest'
import { ansiSegments, stripAnsi } from './ansi'

describe('ansi', () => {
  it('returns a single segment for plain text', () => {
    expect(ansiSegments('hello')).toEqual([{ text: 'hello' }])
  })

  it('strips escape sequences', () => {
    expect(stripAnsi('\u001b[31mred\u001b[0m text')).toBe('red text')
  })

  it('splits coloured text into segments', () => {
    const segments = ansiSegments('\u001b[32mok\u001b[0m done')
    expect(segments.map((segment) => segment.text)).toEqual(['ok', ' done'])
    expect(segments[0]?.style?.color).toBeTruthy()
  })

  it('applies bold and resets it', () => {
    const segments = ansiSegments('\u001b[1mbold\u001b[22m normal')
    expect(segments[0]?.style?.fontWeight).toBe(600)
    expect(segments[1]?.style?.fontWeight).toBeUndefined()
  })

  it('supports 256 colour escapes', () => {
    const segments = ansiSegments('\u001b[38;5;196mred\u001b[0m')
    expect(segments[0]?.style?.color).toBe('rgb(255, 0, 0)')
  })

  it('keeps unknown codes out of the text', () => {
    expect(stripAnsi('\u001b[2Jclear')).toBe('clear')
  })
})
