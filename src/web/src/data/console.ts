import { useCallback, useEffect, useRef, useState } from 'react'
import { api, isAbort, isUnauthenticated, errorMessage } from '../api/client'
import type { ConsoleLine } from '../gen/panel/v1/panel_pb'

export type ConsoleState = 'idle' | 'connecting' | 'live' | 'reconnecting' | 'ended' | 'error'

const MAX_LINES = 4000

export function useConsole(options: {
  instanceId: bigint | undefined
  enabled: boolean
  agentOnline: boolean
  shouldReconnect: boolean
}) {
  const { instanceId, enabled, agentOnline, shouldReconnect } = options
  const [lines, setLines] = useState<ConsoleLine[]>([])
  const [status, setStatus] = useState<ConsoleState>('idle')
  const [error, setError] = useState('')
  const [retryTick, setRetryTick] = useState(0)
  const reconnectRef = useRef(shouldReconnect)
  reconnectRef.current = shouldReconnect

  useEffect(() => {
    if (!enabled || !instanceId) {
      setStatus('idle')
      return
    }
    if (!agentOnline) {
      setStatus('error')
      setError('Агент офлайн: консоль недоступна')
      return
    }
    let stopped = false
    const controller = new AbortController()
    let backoff = 1200

    const run = async () => {
      while (!stopped) {
        setStatus((current) => (current === 'live' ? 'reconnecting' : 'connecting'))
        setError('')
        setLines([])
        let received = false
        try {
          for await (const line of api.console.attach(
            { instanceId, tailLines: 300 },
            { signal: controller.signal },
          )) {
            if (stopped) return
            if (!received) {
              received = true
              setStatus('live')
              backoff = 1200
            }
            setLines((current) => {
              const next = [...current, line]
              return next.length > MAX_LINES ? next.slice(next.length - MAX_LINES) : next
            })
          }
          if (stopped) return
          if (!reconnectRef.current) {
            setStatus('ended')
            return
          }
          setStatus('reconnecting')
        } catch (streamError) {
          if (stopped || isAbort(streamError)) return
          if (isUnauthenticated(streamError)) {
            window.dispatchEvent(new Event('dockium:unauthorized'))
            return
          }
          setError(errorMessage(streamError))
          setStatus('error')
        }
        await new Promise((resolve) => window.setTimeout(resolve, backoff))
        backoff = Math.min(backoff * 2, 10_000)
      }
    }

    void run()
    return () => {
      stopped = true
      controller.abort()
    }
  }, [instanceId, enabled, agentOnline, retryTick])

  const clear = useCallback(() => setLines([]), [])

  const retry = useCallback(() => setRetryTick((current) => current + 1), [])

  const send = useCallback(
    async (command: string) => {
      if (!instanceId) throw new Error('Экземпляр не выбран')
      await api.console.send({ instanceId, command })
    },
    [instanceId],
  )

  return { lines, status, error, clear, send, retry }
}
