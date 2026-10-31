import { describe, expect, it } from 'vitest'
import { baseName, extensionOf, joinPath, parentPath } from './paths'

describe('path helpers', () => {
  it('joins directories and names', () => {
    expect(joinPath('', 'file.txt')).toBe('file.txt')
    expect(joinPath('plugins', 'config.yml')).toBe('plugins/config.yml')
    expect(joinPath('/plugins/', '/config.yml')).toBe('plugins/config.yml')
    expect(joinPath('a/b', '')).toBe('a/b')
  })

  it('normalizes backslashes', () => {
    expect(joinPath('plugins\\cfg', 'a.txt')).toBe('plugins/cfg/a.txt')
  })

  it('returns a parent path', () => {
    expect(parentPath('plugins/config.yml')).toBe('plugins')
    expect(parentPath('file.txt')).toBe('')
    expect(parentPath('a/b/c.txt')).toBe('a/b')
  })

  it('returns a base name', () => {
    expect(baseName('plugins/config.yml')).toBe('config.yml')
    expect(baseName('file.txt')).toBe('file.txt')
  })

  it('detects extensions', () => {
    expect(extensionOf('server.jar')).toBe('jar')
    expect(extensionOf('archive.tar.gz')).toBe('gz')
    expect(extensionOf('README')).toBe('')
  })
})
