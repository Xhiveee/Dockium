import { useQuery } from '@tanstack/react-query'
import { api } from '../api/client'
import type {
  AgentList,
  ApiRequestList,
  BanList,
  DashboardOverview,
  DockerContainerList,
  DockerImageList,
  Instance,
  InstanceList,
  LoginLogList,
  ScheduledTaskList,
  SessionList,
  UserAccess,
  UserList,
} from '../gen/panel/v1/panel_pb'
import { useSession } from './session'

export const keys = {
  instances: ['instances'] as const,
  agents: ['agents'] as const,
  dashboard: ['dashboard'] as const,
  users: ['users'] as const,
  access: (userId: string) => ['access', userId] as const,
  tasks: (instanceId: string) => ['tasks', instanceId] as const,
  images: (agentId: string) => ['images', agentId] as const,
  containers: (agentId: string) => ['containers', agentId] as const,
  logins: ['security', 'logins'] as const,
  api: ['security', 'api'] as const,
  bans: ['security', 'bans'] as const,
  sessions: ['security', 'sessions'] as const,
}

export function useInstancesQuery() {
  return useQuery<InstanceList>({
    queryKey: keys.instances,
    queryFn: () => api.instances.list({}),
    staleTime: 60_000,
    refetchOnWindowFocus: false,
  })
}

export function useInstances(): Instance[] {
  return useInstancesQuery().data?.items ?? []
}

export function useInstance(id: string | undefined): Instance | undefined {
  const instances = useInstances()
  if (!id) return undefined
  return instances.find((instance) => instance.id.toString() === id)
}

export function useAgentsQuery(enabled = true) {
  return useQuery<AgentList>({
    queryKey: keys.agents,
    queryFn: () => api.agents.list({}),
    enabled,
    staleTime: 60_000,
    refetchOnWindowFocus: false,
  })
}

export function useDashboardQuery(enabled = true) {
  return useQuery<DashboardOverview>({
    queryKey: keys.dashboard,
    queryFn: () => api.dashboard.getOverview({}),
    enabled,
    refetchInterval: 20_000,
    staleTime: 10_000,
  })
}

export function useUsersQuery() {
  const { isAdmin } = useSession()
  return useQuery<UserList>({
    queryKey: keys.users,
    queryFn: () => api.users.list({ limit: 200 }),
    enabled: isAdmin,
    staleTime: 30_000,
  })
}

export function useAccessQuery(userId: string | undefined) {
  return useQuery<UserAccess>({
    queryKey: keys.access(userId ?? ''),
    queryFn: () => api.users.listAccess({ id: BigInt(userId ?? '0') }),
    enabled: Boolean(userId),
    staleTime: 20_000,
  })
}

export function useTasksQuery(instanceId: string | undefined) {
  return useQuery<ScheduledTaskList>({
    queryKey: keys.tasks(instanceId ?? ''),
    queryFn: () => api.schedules.list({ instanceId: BigInt(instanceId ?? '0') }),
    enabled: Boolean(instanceId),
    staleTime: 10_000,
    refetchInterval: 20_000,
  })
}

export function useImagesQuery(agentId: string | undefined) {
  return useQuery<DockerImageList>({
    queryKey: keys.images(agentId ?? ''),
    queryFn: () => api.docker.listImages({ agentId: BigInt(agentId ?? '0') }),
    enabled: Boolean(agentId),
    staleTime: 20_000,
    retry: 1,
  })
}

export function useContainersQuery(agentId: string | undefined) {
  return useQuery<DockerContainerList>({
    queryKey: keys.containers(agentId ?? ''),
    queryFn: () => api.docker.listContainers({ agentId: BigInt(agentId ?? '0') }),
    enabled: Boolean(agentId),
    staleTime: 20_000,
    retry: 1,
  })
}

export function useLoginLogsQuery(enabled = true) {
  return useQuery<LoginLogList>({
    queryKey: keys.logins,
    queryFn: () => api.security.listLoginLogs({ limit: 100, offset: 0 }),
    enabled,
    staleTime: 15_000,
  })
}

export function useApiRequestsQuery(enabled = true) {
  return useQuery<ApiRequestList>({
    queryKey: keys.api,
    queryFn: () => api.security.listApiRequests({ limit: 200, offset: 0 }),
    enabled,
    staleTime: 10_000,
  })
}

export function useBansQuery(enabled = true) {
  return useQuery<BanList>({
    queryKey: keys.bans,
    queryFn: () => api.security.listBans({ limit: 100 }),
    enabled,
    staleTime: 15_000,
  })
}

export function useSessionsQuery(enabled = true) {
  return useQuery<SessionList>({
    queryKey: keys.sessions,
    queryFn: () => api.security.listSessions({}),
    enabled,
    staleTime: 10_000,
  })
}
