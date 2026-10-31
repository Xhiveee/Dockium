import { api, isUnauthenticated } from '../api/client'
import type { BuildOutput } from '../gen/panel/v1/panel_pb'
import { joinPath } from '../lib/paths'

export async function streamBuild(
  agentId: bigint,
  tag: string,
  dockerfile: string,
  onOutput: (output: BuildOutput) => void,
  signal?: AbortSignal,
): Promise<void> {
  const stream = api.docker.buildImage({ agentId, tag, dockerfile }, { signal })
  for await (const output of stream) {
    onOutput(output)
  }
}

export async function streamPull(
  agentId: bigint,
  image: string,
  onOutput: (output: BuildOutput) => void,
  signal?: AbortSignal,
): Promise<void> {
  const stream = api.docker.pullImage({ agentId, image }, { signal })
  for await (const output of stream) {
    onOutput(output)
  }
}

export async function downloadToBrowser(
  instanceId: bigint,
  path: string,
  filename: string,
): Promise<void> {
  const chunks: Uint8Array[] = []
  let total = 0
  const limit = 256 * 1024 * 1024
  const stream = api.files.downloadFile({ instanceId, path })
  for await (const chunk of stream) {
    if (chunk.data.length) {
      total += chunk.data.length
      if (total > limit) throw new Error('Файл больше 256 МБ — скачайте его через агент')
      chunks.push(chunk.data)
    }
  }
  const blob = new Blob(chunks as BlobPart[], { type: 'application/octet-stream' })
  const url = URL.createObjectURL(blob)
  const anchor = document.createElement('a')
  anchor.href = url
  anchor.download = filename
  document.body.appendChild(anchor)
  anchor.click()
  anchor.remove()
  window.setTimeout(() => URL.revokeObjectURL(url), 4000)
}

export async function uploadFile(
  instanceId: bigint,
  path: string,
  file: File,
  overwrite: boolean,
): Promise<void> {
  const maximum = 32 * 1024 * 1024
  if (file.size > maximum) {
    throw new Error('Файл больше 32 МБ: используйте агент или архив')
  }
  const content = new Uint8Array(await file.arrayBuffer())
  await api.files.uploadFile({ instanceId, path, content, overwrite })
}

export async function uploadMany(
  instanceId: bigint,
  directory: string,
  files: File[],
  onProgress?: (done: number, total: number, current: string) => void,
): Promise<void> {
  for (let index = 0; index < files.length; index += 1) {
    const file = files[index]
    if (!file) continue
    onProgress?.(index, files.length, file.name)
    await uploadFile(instanceId, joinPath(directory, file.name), file, false)
  }
  onProgress?.(files.length, files.length, '')
}

export function decodeUtf8(content: Uint8Array): string {
  return new TextDecoder('utf-8', { fatal: false }).decode(content)
}

export function encodeUtf8(value: string): Uint8Array {
  return new TextEncoder().encode(value)
}

export function isUnauthorized(error: unknown): boolean {
  return isUnauthenticated(error)
}
