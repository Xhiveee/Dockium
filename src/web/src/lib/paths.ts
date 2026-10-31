export function joinPath(directory: string, name: string): string {
  const base = directory.replaceAll('\\', '/').replace(/^\/+|\/+$/g, '')
  const cleanName = name.replaceAll('\\', '/').replace(/^\/+|\/+$/g, '')
  if (!base) return cleanName
  if (!cleanName) return base
  return `${base}/${cleanName}`
}

export function parentPath(path: string): string {
  const normalized = path.replaceAll('\\', '/').replace(/\/+$/, '')
  const index = normalized.lastIndexOf('/')
  return index <= 0 ? '' : normalized.slice(0, index)
}

export function baseName(path: string): string {
  const normalized = path.replaceAll('\\', '/').replace(/\/+$/, '')
  const index = normalized.lastIndexOf('/')
  return index < 0 ? normalized : normalized.slice(index + 1)
}

export function extensionOf(name: string): string {
  const index = name.lastIndexOf('.')
  return index > 0 ? name.slice(index + 1).toLowerCase() : ''
}
