import { MutationCache, QueryCache, QueryClient } from '@tanstack/react-query'
import { isUnauthenticated } from '../api/client'

export function createAppQueryClient(): QueryClient {
  const client: QueryClient = new QueryClient({
    defaultOptions: {
      queries: {
        staleTime: 20_000,
        refetchOnWindowFocus: false,
        retry: (failureCount, error) => !isUnauthenticated(error) && failureCount < 2,
      },
      mutations: {
        retry: false,
      },
    },
    queryCache: new QueryCache({
      onError: (error) => {
        if (isUnauthenticated(error)) {
          window.dispatchEvent(new Event('dockium:unauthorized'))
        }
      },
    }),
    mutationCache: new MutationCache({}),
  })
  return client
}
