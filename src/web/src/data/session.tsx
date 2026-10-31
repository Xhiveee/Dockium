import { useQuery, useQueryClient } from '@tanstack/react-query'
import {
  createContext,
  useCallback,
  useContext,
  useEffect,
  useMemo,
  useState,
  type ReactNode,
} from 'react'
import { api, isUnauthenticated } from '../api/client'
import type { MeResponse, SetupStatus, User } from '../gen/panel/v1/panel_pb'

type SessionContextValue = {
  setup: SetupStatus | undefined
  initialized: boolean
  user: User | null
  isAdmin: boolean
  securityAccess: boolean
  agentsAccess: boolean
  loading: boolean
  version: string
  signIn: (login: string, password: string) => Promise<User>
  createAdmin: (login: string, password: string) => Promise<User>
  signOut: () => Promise<void>
  refresh: () => Promise<void>
}

const SessionContext = createContext<SessionContextValue | null>(null)

function meFromUser(user: User): MeResponse {
  return {
    $typeName: 'dockium.panel.v1.MeResponse',
    user,
    securityAccess: user.role === 'admin',
    agentsAccess: user.role === 'admin',
  }
}

export function SessionProvider({ children }: { children: ReactNode }) {
  const queryClient = useQueryClient()
  const [signedOut, setSignedOut] = useState(false)

  const setupQuery = useQuery({
    queryKey: ['setup'],
    queryFn: () => api.setup.getStatus({}),
    staleTime: 60_000,
    retry: 1,
  })

  const initialized = setupQuery.data?.initialized ?? false

  const meQuery = useQuery({
    queryKey: ['me'],
    queryFn: async (): Promise<MeResponse | null> => {
      try {
        return await api.auth.me({})
      } catch (error) {
        if (isUnauthenticated(error)) return null
        throw error
      }
    },
    enabled: initialized && !signedOut,
    retry: 0,
    staleTime: 30_000,
  })

  useEffect(() => {
    const onUnauthorized = () => {
      queryClient.setQueryData(['me'], null)
      queryClient.removeQueries({ queryKey: ['instances'] })
      queryClient.removeQueries({ queryKey: ['agents'] })
    }
    window.addEventListener('dockium:unauthorized', onUnauthorized)
    return () => window.removeEventListener('dockium:unauthorized', onUnauthorized)
  }, [queryClient])

  const signIn = useCallback(
    async (login: string, password: string) => {
      const response = await api.auth.login({ login, password })
      if (!response.user) throw new Error('Панель не вернула пользователя')
      setSignedOut(false)
      queryClient.setQueryData<MeResponse>(['me'], meFromUser(response.user))
      queryClient.setQueryData<SetupStatus | undefined>(['setup'], (current) =>
        current ? { ...current, initialized: true } : current,
      )
      await queryClient.invalidateQueries()
      return response.user
    },
    [queryClient],
  )

  const createAdmin = useCallback(
    async (login: string, password: string) => {
      const user = await api.setup.createAdmin({ credentials: { login, password } })
      setSignedOut(false)
      queryClient.setQueryData<MeResponse>(['me'], meFromUser(user))
      queryClient.setQueryData<SetupStatus | undefined>(['setup'], (current) =>
        current ? { ...current, initialized: true } : current,
      )
      return user
    },
    [queryClient],
  )

  const signOut = useCallback(async () => {
    try {
      await api.auth.logout({})
    } finally {
      setSignedOut(true)
      queryClient.removeQueries({
        predicate: (query) => !['setup', 'me'].includes(String(query.queryKey[0] ?? '')),
      })
      queryClient.setQueryData(['me'], null)
    }
  }, [queryClient])

  const refresh = useCallback(async () => {
    await queryClient.invalidateQueries({ queryKey: ['me'] })
  }, [queryClient])

  const me = signedOut ? null : (meQuery.data ?? null)
  const user = me?.user ?? null

  const value = useMemo<SessionContextValue>(
    () => ({
      setup: setupQuery.data,
      initialized,
      user,
      isAdmin: user?.role === 'admin',
      securityAccess: me?.securityAccess ?? false,
      agentsAccess: me?.agentsAccess ?? false,
      loading: setupQuery.isPending || (initialized && !signedOut && meQuery.isPending),
      version: setupQuery.data?.version ?? '',
      signIn,
      createAdmin,
      signOut,
      refresh,
    }),
    [setupQuery.data, initialized, user, me, setupQuery.isPending, meQuery.isPending, signedOut, signIn, createAdmin, signOut, refresh],
  )

  return <SessionContext.Provider value={value}>{children}</SessionContext.Provider>
}

export function useSession(): SessionContextValue {
  const context = useContext(SessionContext)
  if (!context) throw new Error('useSession must be used inside SessionProvider')
  return context
}
