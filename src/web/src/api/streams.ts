import { api, isUnauthenticated } from '../api/client'
import type { BuildOutput } from '../gen/panel/v1/panel_pb'

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

export const UPLOAD_CHUNK_SIZE = 4 * 1024 * 1024

export type UploadProgress = (uploadedBytes: number, totalBytes: number) => void

export async function uploadFile(
  instanceId: bigint,
  path: string,
  file: File,
  overwrite: boolean,
  maxBytes: number,
  onProgress?: UploadProgress,
): Promise<void> {
  if (file.size > maxBytes) {
    const maximumMB = Math.max(1, Math.floor(maxBytes / (1024 * 1024)))
    throw new Error('Файл ' + file.name + ' больше ' + maximumMB + ' МБ')
  }
  const uploadId = crypto.randomUUID()
  let offset = 0
  do {
    const end = Math.min(offset + UPLOAD_CHUNK_SIZE, file.size)
    const content = new Uint8Array(await file.slice(offset, end).arrayBuffer())
    await api.files.uploadFile({
      instanceId,
      path,
      content,
      overwrite,
      uploadId,
      offset: BigInt(offset),
      final: end >= file.size,
    })
    offset = end
    onProgress?.(offset, file.size)
  } while (offset < file.size)
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
