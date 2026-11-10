import { Code, ConnectError, createClient } from '@connectrpc/connect'
import { createConnectTransport } from '@connectrpc/connect-web'
import {
  AgentService,
  AuthService,
  ConsoleService,
  DashboardService,
  DockerService,
  FileService,
  InstanceService,
  ScheduleService,
  SecurityEventsService,
  SecurityService,
  SettingsService,
  SetupService,
  UserService,
} from '../gen/panel/v1/panel_pb'

const transport = createConnectTransport({
  baseUrl: window.location.origin,
  useBinaryFormat: true,
})

export const api = {
  setup: createClient(SetupService, transport),
  auth: createClient(AuthService, transport),
  users: createClient(UserService, transport),
  agents: createClient(AgentService, transport),
  instances: createClient(InstanceService, transport),
  console: createClient(ConsoleService, transport),
  files: createClient(FileService, transport),
  docker: createClient(DockerService, transport),
  schedules: createClient(ScheduleService, transport),
  security: createClient(SecurityService, transport),
  securityEvents: createClient(SecurityEventsService, transport),
  dashboard: createClient(DashboardService, transport),
  settings: createClient(SettingsService, transport),
}

export function errorMessage(error: unknown): string {
  if (error instanceof ConnectError) {
    return error.rawMessage || error.message || 'Неизвестная ошибка'
  }
  if (error instanceof Error) {
    if (error.name === 'AbortError') return 'Запрос отменён'
    return error.message
  }
  return 'Неизвестная ошибка'
}

export function isCode(error: unknown, code: Code): boolean {
  return error instanceof ConnectError && error.code === code
}

export function isUnauthenticated(error: unknown): boolean {
  return isCode(error, Code.Unauthenticated)
}

export function isAbort(error: unknown): boolean {
  return error instanceof Error && error.name === 'AbortError'
}
