import { useQueryClient } from '@tanstack/react-query'
import {
  createContext,
  useCallback,
  useContext,
  useEffect,
  useMemo,
  useRef,
  useState,
  type ReactNode,
} from 'react'
import { isAbort, isUnauthenticated } from '../api/client'
import { api } from '../api/client'
import type {
  AgentList,
  ApiRequestList,
  BanList,
  InstanceList,
  LoginLogList,
  SecurityEvent,
} from '../gen/panel/v1/panel_pb'
import { useToast } from '../ui/Toast'
import { keys } from './queries'
import { useSession } from './session'

export type LiveStatus = 'connecting' | 'live' | 'reconnecting' | 'offline'

type LiveContextValue = {
  status: LiveStatus
  instanceStream: LiveStatus
  agentStream: LiveStatus
  securityStream: LiveStatus
}

const LiveContext = createContext<LiveContextValue>({
  status: 'offline',
  instanceStream: 'offline',
  agentStream: 'offline',
  securityStream: 'offline',
})

function sleep(milliseconds: number): Promise<void> {
  return new Promise((resolve) => window.setTimeout(resolve, milliseconds))
}

type StreamOptions<T> = {
  enabled: boolean
  label: 'instances' | 'agents' | 'security'
  start: (signal: AbortSignal) => AsyncIterable<T>
  onItem: (item: T) => void
  onRecover: () => void
  setStatus: (label: 'instances' | 'agents' | 'security', status: LiveStatus) => void
}

function useLiveStream<T>({ enabled, label, start, onItem, onRecover, setStatus }: StreamOptions<T>) {
  const onItemRef = useRef(onItem)
  const onRecoverRef = useRef(onRecover)
  const startRef = useRef(start)
  onItemRef.current = onItem
  onRecoverRef.current = onRecover
  startRef.current = start

  useEffect(() => {
    if (!enabled) {
      setStatus(label, 'offline')
      return
    }
    let stopped = false
    const controller = new AbortController()
    let backoff = 900
    let live = false

    const run = async () => {
      while (!stopped) {
        setStatus(label, 'connecting')
        try {
          for await (const item of startRef.current(controller.signal)) {
            if (stopped) return
            if (!live) {
              live = true
              setStatus(label, 'live')
              onRecoverRef.current()
            }
            onItemRef.current(item)
          }
          if (stopped) return
        } catch (error) {
          if (stopped || isAbort(error)) return
          if (isUnauthenticated(error)) {
            window.dispatchEvent(new Event('dockium:unauthorized'))
            return
          }
        }
        if (stopped) return
        live = false
        setStatus(label, 'reconnecting')
        await sleep(backoff + Math.random() * 500)
        backoff = Math.min(backoff * 2, 15_000)
      }
    }

    void run()
    return () => {
      stopped = true
      controller.abort()
      setStatus(label, 'offline')
    }
  }, [enabled, label, setStatus])
}

export function LiveProvider({ children }: { children: ReactNode }) {
  const queryClient = useQueryClient()
  const toast = useToast()
  const { user, isAdmin } = useSession()
  const [statuses, setStatuses] = useState<Record<'instances' | 'agents' | 'security', LiveStatus>>({
    instances: 'offline',
    agents: 'offline',
    security: 'offline',
  })

  const setStatus = useCallback(
    (label: 'instances' | 'agents' | 'security', status: LiveStatus) => {
      setStatuses((current) => (current[label] === status ? current : { ...current, [label]: status }))
    },
    [],
  )

  const handleInstanceEvent = useCallback(
    (event: import('../gen/panel/v1/panel_pb').InstanceEvent) => {
      queryClient.setQueryData<InstanceList>(keys.instances, (current) => {
        if (!current) return current
        const payload = event.payload
        if (payload.case === 'snapshot') {
          return { ...current, items: payload.value.items }
        }
        if (payload.case === 'instance') {
          const instance = payload.value
          const items = current.items.slice()
          const index = items.findIndex((item) => item.id === instance.id)
          if (index >= 0) items[index] = instance
          else items.push(instance)
          items.sort((left, right) => left.name.localeCompare(right.name, 'ru'))
          return { ...current, items }
        }
        if (payload.case === 'removed') {
          return { ...current, items: current.items.filter((item) => item.id !== payload.value.id) }
        }
        return current
      })
    },
    [queryClient],
  )

  const handleAgentEvent = useCallback(
    (event: import('../gen/panel/v1/panel_pb').AgentEvent) => {
      queryClient.setQueryData<AgentList>(keys.agents, (current) => {
        if (!current) return current
        const payload = event.payload
        if (payload.case === 'snapshot') {
          return { ...current, items: payload.value.items }
        }
        if (payload.case === 'agent') {
          const agent = payload.value
          const items = current.items.slice()
          const index = items.findIndex((item) => item.id === agent.id)
          if (index >= 0) items[index] = agent
          else items.push(agent)
          items.sort((left, right) => left.name.localeCompare(right.name, 'ru'))
          return { ...current, items }
        }
        if (payload.case === 'removed') {
          return { ...current, items: current.items.filter((item) => item.id !== payload.value.id) }
        }
        return current
      })
    },
    [queryClient],
  )

  const handleSecurityEvent = useCallback(
    (event: SecurityEvent) => {
      const payload = event.payload
      if (payload.case === 'loginLog') {
        queryClient.setQueryData<LoginLogList>(keys.logins, (current) => {
          if (!current) return current
          const index = current.items.findIndex((item) => item.id === payload.value.id)
          if (index >= 0) {
            const items = current.items.slice()
            items[index] = payload.value
            return { ...current, items }
          }
          return { ...current, total: current.total + 1, items: [payload.value, ...current.items].slice(0, 100) }
        })
      } else if (payload.case === 'apiRequest') {
        queryClient.setQueryData<ApiRequestList>(keys.api, (current) => {
          if (!current) return current
          if (current.items.some((item) => item.id === payload.value.id)) return current
          return { ...current, total: current.total + 1, items: [payload.value, ...current.items].slice(0, 200) }
        })
      } else if (payload.case === 'ban') {
        queryClient.setQueryData<BanList>(keys.bans, (current) => {
          if (!current) return current
          const items = current.items.filter((item) => item.ip !== payload.value.ip)
          items.unshift(payload.value)
          return { ...current, total: items.length, items }
        })
      } else if (payload.case === 'banRemoved') {
        queryClient.setQueryData<BanList>(keys.bans, (current) => {
          if (!current) return current
          const items = current.items.filter((item) => item.id !== payload.value.id)
          return { ...current, total: items.length, items }
        })
      } else if (payload.case === 'sessionRevoked') {
        void queryClient.invalidateQueries({ queryKey: keys.sessions })
      }
    },
    [queryClient],
  )

  useLiveStream({
    enabled: Boolean(user),
    label: 'instances',
    start: (signal) => api.instances.watch({}, { signal }),
    onItem: handleInstanceEvent,
    onRecover: () => {
      void queryClient.invalidateQueries({ queryKey: keys.instances })
    },
    setStatus,
  })

  useLiveStream({
    enabled: Boolean(user) && isAdmin,
    label: 'agents',
    start: (signal) => api.agents.watch({}, { signal }),
    onItem: handleAgentEvent,
    onRecover: () => {
      void queryClient.invalidateQueries({ queryKey: keys.agents })
    },
    setStatus,
  })

  useLiveStream({
    enabled: Boolean(user) && isAdmin,
    label: 'security',
    start: (signal) => api.securityEvents.watch({}, { signal }),
    onItem: handleSecurityEvent,
    onRecover: () => {
      void queryClient.invalidateQueries({ queryKey: keys.sessions })
    },
    setStatus,
  })

  const activeStatuses = useMemo(() => {
    const values: LiveStatus[] = [statuses.instances]
    if (isAdmin) {
      values.push(statuses.agents, statuses.security)
    }
    return values
  }, [statuses, isAdmin])

  const status = useMemo<LiveStatus>(() => {
    if (!user) return 'offline'
    if (activeStatuses.includes('reconnecting')) return 'reconnecting'
    if (activeStatuses.every((value) => value === 'live')) return 'live'
    if (activeStatuses.some((value) => value === 'live')) return 'connecting'
    if (activeStatuses.every((value) => value === 'offline')) return 'offline'
    return 'connecting'
  }, [activeStatuses, user])

  const previousStatus = useRef<LiveStatus>('offline')
  useEffect(() => {
    const previous = previousStatus.current
    if (previous === 'live' && status === 'reconnecting') {
      toast.warning(
        'Соединение с панелью потеряно',
        'Пробуем восстановить поток событий автоматически.',
      )
    }
    if (previous === 'reconnecting' && status === 'live') {
      toast.success(
        'Соединение восстановлено',
        'Состояние экземпляров и агентов обновлено.',
      )
    }
    previousStatus.current = status
  }, [status])

  const value = useMemo<LiveContextValue>(
    () => ({
      status,
      instanceStream: statuses.instances,
      agentStream: statuses.agents,
      securityStream: statuses.security,
    }),
    [status, statuses],
  )

  return <LiveContext.Provider value={value}>{children}</LiveContext.Provider>
}

export function useLive(): LiveContextValue {
  return useContext(LiveContext)
}
