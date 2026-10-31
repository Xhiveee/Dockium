export type AnsiSegment = {
  text: string
  className?: string
  style?: { color?: string; fontWeight?: number; opacity?: number; textDecoration?: string }
}

const BASIC_COLORS = ['#8b8982', '#e28f8b', '#8fc9a8', '#e0b568', '#8fb2e2', '#c3a3cf', '#8ec5c9', '#d9d6cc']
const BRIGHT_COLORS = ['#a8a59d', '#f0a8a3', '#a8dcc0', '#efcb8a', '#a9c6ed', '#d8bce2', '#a8dadd', '#f2efe6']

function colorFor256(index: number): string {
  if (index < 8) return BASIC_COLORS[index] ?? '#d9d6cc'
  if (index < 16) return BRIGHT_COLORS[index - 8] ?? '#f2efe6'
  if (index >= 16 && index <= 231) {
    const adjusted = index - 16
    const red = Math.floor(adjusted / 36)
    const green = Math.floor((adjusted % 36) / 6)
    const blue = adjusted % 6
    const channel = (value: number) => (value === 0 ? 0 : 55 + value * 40)
    return `rgb(${channel(red)}, ${channel(green)}, ${channel(blue)})`
  }
  const grey = 8 + (index - 232) * 10
  return `rgb(${grey}, ${grey}, ${grey})`
}

const ANSI_PATTERN = /\u001b\[([0-9;]*)([A-Za-z])/g

export function ansiSegments(input: string): AnsiSegment[] {
  const segments: AnsiSegment[] = []
  let current: AnsiSegment = { text: '' }
  let color: string | undefined
  let bold = false
  let dim = false
  let underline = false
  let lastIndex = 0

  const flush = () => {
    if (!current.text) return
    const style = current.style ?? {}
    if (color) style.color = color
    if (bold) style.fontWeight = 600
    if (dim) style.opacity = 0.7
    if (underline) style.textDecoration = 'underline'
    current.style = Object.keys(style).length ? style : undefined
    segments.push(current)
    current = { text: '' }
  }

  ANSI_PATTERN.lastIndex = 0
  for (let match = ANSI_PATTERN.exec(input); match; match = ANSI_PATTERN.exec(input)) {
    current.text += input.slice(lastIndex, match.index)
    flush()
    lastIndex = match.index + match[0].length
    if (match[2] !== 'm') continue
    const codes = (match[1] || '0').split(';').map((value) => (value === '' ? 0 : Number(value)))
    for (let index = 0; index < codes.length; index += 1) {
      const code = codes[index]
      if (code === 0) {
        color = undefined
        bold = false
        dim = false
        underline = false
      } else if (code === 1) {
        bold = true
      } else if (code === 2) {
        dim = true
      } else if (code === 4) {
        underline = true
      } else if (code === 22) {
        bold = false
        dim = false
      } else if (code === 24) {
        underline = false
      } else if (code === 39) {
        color = undefined
      } else if (code >= 30 && code <= 37) {
        color = BASIC_COLORS[code - 30]
      } else if (code >= 90 && code <= 97) {
        color = BRIGHT_COLORS[code - 90]
      } else if (code === 38 || code === 48) {
        const mode = codes[index + 1]
        if (mode === 5) {
          const paletteIndex = codes[index + 2] ?? 0
          if (code === 38) color = colorFor256(paletteIndex)
          index += 2
        } else if (mode === 2) {
          const red = codes[index + 2] ?? 0
          const green = codes[index + 3] ?? 0
          const blue = codes[index + 4] ?? 0
          if (code === 38) color = `rgb(${red}, ${green}, ${blue})`
          index += 4
        }
      }
    }
  }
  current.text += input.slice(lastIndex)
  flush()
  return segments.length ? segments : [{ text: input }]
}

export function stripAnsi(input: string): string {
  return input.replace(ANSI_PATTERN, '')
}
