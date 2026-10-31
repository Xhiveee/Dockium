import { useQueryClient } from '@tanstack/react-query'
import { Link, useNavigate } from 'react-router-dom'
import {
  AlertTriangle,
  ArrowUpRight,
  Boxes,
  CalendarClock,
  Cpu,
  MemoryStick,
  Play,
  Server,
  ShieldCheck,
  Square,
  WifiOff,
} from 'lucide-react'
import { useMemo, useState } from 'react'
import { api, errorMessage } from '../api/client'
import { useLive } from '../data/live'
import { keys, useAgentsQuery, useDashboardQuery, useInstances } from '../data/queries'
import { useSession } from '../data/session'
import { useSeries } from '../data/useSeries'
import type { Agent, Instance } from '../gen/panel/v1/panel_pb'
import { InstanceStatus } from '../gen/panel/v1/panel_pb'
import {
  formatBytes,
  formatDate,
  formatPercent,
  formatRelative,
  formatUptime,
  instanceStatusInfo,
  isRunning,
  isTransition,
} from '../lib/format'
import { cx } from '../lib/cx'
import { Badge, EmptyState, InlineNotice, Meter, PageHeader, Sparkline } from '../ui/Display'
import { Button } from '../ui/Button'
import { Panel } from '../ui/Panel'
import { useToast } from '../ui/Toast'
import styles from './OverviewPage.module.css'

export function OverviewPage() {
  const { user, isAdmin } = useSession()
  const instances = useInstances()
  const agentsQuery = useAgentsQuery(isAdmin)
  const dashboard = useDashboardQuery(Boolean(user))
  const live = useLive()
  const toast = useToast()
  const queryClient = useQueryClient()
  const navigate = useNavigate()
  const [pending, setPending] = useState<Record<string, boolean>>({})

  const agents = agentsQuery.data?.items ?? []
  const onlineAgents = agents.filter((agent) => agent.online)
  const running = instances.filter((instance) => instance.status === InstanceStatus.RUNNING)
  const attention = instances.filter(
    (instance) =>
      instance.status === InstanceStatus.FAILED ||
      (instance.status === InstanceStatus.EXITED && instance.exitCode !== 0),
  )
  const offlineAgents = agents.filter((agent) => !agent.online)

  const aggregate = useMemo(() => {
    if (isAdmin) {
      const cpu = onlineAgents.length
        ? onlineAgents.reduce((sum, agent) => sum + agent.cpuPercent, 0) / onlineAgents.length
        : 0
      return {
        cpu,
        memoryUsed: onlineAgents.reduce((sum, agent) => sum + Number(agent.memoryUsedBytes), 0),
        memoryTotal: onlineAgents.reduce((sum, agent) => sum + Number(agent.memoryTotalBytes), 0),
        caption: `средняя по ${onlineAgents.length} ${onlineAgents.length === 1 ? 'агенту' : 'агентам'}`,
      }
    }
    return {
      cpu: instances.reduce((sum, instance) => sum + (instance.usage?.cpuPercent ?? 0), 0),
      memoryUsed: instances.reduce((sum, instance) => sum + Number(instance.usage?.memoryBytes ?? 0n), 0),
      memoryTotal: instances.reduce(
        (sum, instance) => sum + Number(instance.usage?.memoryLimitBytes ?? 0n),
        0,
      ),
      caption: 'суммарно по экземплярам',
    }
  }, [isAdmin, instances, onlineAgents])

  const cpuHistory = useSeries(Math.round(aggregate.cpu), 48)
  const memoryPercent = aggregate.memoryTotal > 0 ? (aggregate.memoryUsed / aggregate.memoryTotal) * 100 : 0

  const runAction = async (instance: Instance, action: 'start' | 'stop') => {
    const key = instance.id.toString()
    setPending((current) => ({ ...current, [key]: true }))
    try {
      if (action === 'start') {
        await api.instances.start({ id: instance.id })
        toast.success('Экземпляр запускается', instance.name)
      } else {
        await api.instances.stop({ id: instance.id })
        toast.success('Экземпляр останавливается', instance.name)
      }
      await queryClient.invalidateQueries({ queryKey: keys.instances })
    } catch (error) {
      toast.error('Не удалось выполнить действие', errorMessage(error))
    } finally {
      setPending((current) => ({ ...current, [key]: false }))
    }
  }

  const tasks = dashboard.data?.upcomingTasks ?? []
  const logins = dashboard.data?.recentLogins ?? []

  const problems: string[] = []
  if (offlineAgents.length) {
    problems.push(
      `${offlineAgents.length} ${offlineAgents.length === 1 ? 'агент' : 'агентов'} не на связи`,
    )
  }
  const dockerIssues = agents.filter((agent) => agent.online && !agent.dockerOk)
  if (dockerIssues.length) {
    problems.push(`Docker недоступен на ${dockerIssues.map((agent) => agent.name).join(', ')}`)
  }
  if (attention.length) {
    problems.push(`${attention.length} ${attention.length === 1 ? 'экземпляр требует' : 'экземпляра требуют'} внимания`)
  }

  const stripCells = [
    ...(isAdmin
      ? [{
          id: 'agents',
          icon: <Server size={13} />,
          label: 'Агенты',
          value: `${onlineAgents.length} / ${agents.length}`,
          caption: offlineAgents.length ? `${offlineAgents.length} офлайн` : 'все на связи',
          ratio: agents.length ? (onlineAgents.length / agents.length) * 100 : 0,
          tone: 'success' as const,
        }]
      : []),
    {
      id: 'instances',
      icon: <Boxes size={13} />,
      label: 'Экземпляры',
      value: `${running.length} / ${instances.length}`,
      caption: attention.length ? `${attention.length} требуют внимания` : 'работают',
      ratio: instances.length ? (running.length / instances.length) * 100 : 0,
      tone: attention.length ? ('warning' as const) : ('accent' as const),
    },
    {
      id: 'cpu',
      icon: <Cpu size={13} />,
      label: 'Нагрузка CPU',
      value: formatPercent(aggregate.cpu),
      caption: aggregate.caption,
      ratio: Math.min(100, aggregate.cpu),
      tone: aggregate.cpu > 85 ? ('danger' as const) : aggregate.cpu > 60 ? ('warning' as const) : ('accent' as const),
    },
    {
      id: 'memory',
      icon: <MemoryStick size={13} />,
      label: 'Память',
      value: formatBytes(aggregate.memoryUsed),
      caption: aggregate.memoryTotal > 0 ? `из ${formatBytes(aggregate.memoryTotal)}` : 'лимит не задан',
      ratio: memoryPercent,
      tone: memoryPercent > 85 ? ('danger' as const) : memoryPercent > 70 ? ('warning' as const) : ('accent' as const),
    },
  ]

  return (
    <div className={styles.page}>
      <PageHeader
        title="Обзор"
        badge={live.status === 'reconnecting' ? <Badge tone="warning" dot>переподключение</Badge> : null}
        actions={
          <>
            <Button variant="ghost" onClick={() => navigate('/instances')}>Все экземпляры</Button>
            {isAdmin ? (
              <Button
                variant="primary"
                onClick={() => navigate(agents.length ? '/instances?new=1' : '/agents')}
              >
                {agents.length ? 'Создать экземпляр' : 'Подключить агента'}
              </Button>
            ) : null}
          </>
        }
      />

      {problems.length ? (
        <div className={styles.noticeWrap}>
          <InlineNotice
            tone={offlineAgents.length ? 'danger' : 'warning'}
            icon={offlineAgents.length ? WifiOff : AlertTriangle}
            title={problems[0]}
            action={
              <Button size="sm" variant="ghost" onClick={() => navigate(offlineAgents.length ? '/agents' : '/instances')}>
                Открыть
              </Button>
            }
          >
            {problems.slice(1).join(' · ')}
          </InlineNotice>
        </div>
      ) : null}

      <div className={styles.strip}>
        {stripCells.map((cell) => (
          <div key={cell.id} className={styles.cell}>
            <div className={styles.cellHead}>
              <span className={styles.cellIcon} aria-hidden>{cell.icon}</span>
              <span className={styles.cellLabel}>{cell.label}</span>
              {cell.id === 'cpu' ? (
                <span className={styles.cellSpark} aria-hidden>
                  <Sparkline values={cpuHistory} height={20} tone={cell.tone === 'danger' ? 'danger' : 'accent'} />
                </span>
              ) : null}
            </div>
            <p className={styles.cellValue}>{cell.value}</p>
            <p className={styles.cellCaption}>{cell.caption}</p>
            <span className={styles.cellTrack} aria-hidden>
              <span
                className={cx(styles.cellFill, styles[cell.tone])}
                style={{ width: `${Math.max(0, Math.min(100, cell.ratio))}%` }}
              />
            </span>
          </div>
        ))}
      </div>

      <div className={cx(styles.grid, !isAdmin && styles.singleColumn)}>
        <Panel
          title="Экземпляры"
          icon={<Boxes size={14} />}
          action={<Link className={styles.panelLink} to="/instances">Все <ArrowUpRight size={13} /></Link>}
          flush
        >
          {instances.length === 0 ? (
            <EmptyState
              icon={Boxes}
              title={isAdmin ? 'Экземпляров пока нет' : 'Нет доступных экземпляров'}
              description={
                isAdmin
                  ? agents.length
                    ? 'Создайте первый контейнер из Docker-образа и запустите его.'
                    : 'Сначала подключите агента — он запустит Docker-контейнеры.'
                  : 'Администратор панели ещё не выдал доступ ни к одному экземпляру.'
              }
              action={
                isAdmin ? (
                  <Button variant="primary" onClick={() => navigate(agents.length ? '/instances?new=1' : '/agents')}>
                    {agents.length ? 'Создать экземпляр' : 'Подключить агента'}
                  </Button>
                ) : undefined
              }
            />
          ) : (
            <ul className={styles.instanceList}>
              {instances.slice(0, 8).map((instance) => {
                const status = instanceStatusInfo(instance.status, instance.exitCode, instance.lastError)
                const busy = isTransition(instance.status) || pending[instance.id.toString()]
                const canControl = instance.permissions?.control && instance.agentOnline
                return (
                  <li key={instance.id.toString()} className={styles.instanceRow}>
                    <span className={cx(styles.rail, styles[`rail-${status.tone}`])} aria-hidden />
                    <Link to={`/instances/${instance.id}`} className={styles.instanceMain}>
                      <span className={styles.instanceNameRow}>
                        <span className={styles.instanceName}>{instance.name}</span>
                        {!instance.agentOnline ? <WifiOff size={12} className={styles.offlineIcon} aria-label="Агент офлайн" /> : null}
                      </span>
                      <span className={styles.instanceMeta}>
                        {instance.agentName} · {instance.image}
                        {isRunning(instance.status) && instance.lastStartedAt
                          ? ` · ${formatUptime(instance.lastStartedAt)}`
                          : ''}
                      </span>
                    </Link>
                    <div className={styles.instanceMetrics}>
                      <span className={styles.metricInline}>
                        <span className={styles.metricKey}>CPU</span>
                        <span className={styles.metricValue}>{formatPercent(instance.usage?.cpuPercent ?? 0)}</span>
                      </span>
                      <span className={styles.metricInline}>
                        <span className={styles.metricKey}>RAM</span>
                        <span className={styles.metricValue}>{formatBytes(instance.usage?.memoryBytes ?? 0n)}</span>
                      </span>
                    </div>
                    <Badge tone={status.tone} dot>{status.label}</Badge>
                    <div className={styles.instanceAction}>
                      {isRunning(instance.status) ? (
                        <Button
                          size="sm"
                          variant="ghost"
                          icon={<Square size={13} />}
                          aria-label={`Остановить ${instance.name}`}
                          disabled={!canControl || busy}
                          loading={Boolean(pending[instance.id.toString()])}
                          onClick={() => runAction(instance, 'stop')}
                        >
                          Стоп
                        </Button>
                      ) : (
                        <Button
                          size="sm"
                          variant="ghost"
                          icon={<Play size={13} />}
                          aria-label={`Запустить ${instance.name}`}
                          disabled={!canControl || busy}
                          loading={Boolean(pending[instance.id.toString()])}
                          onClick={() => runAction(instance, 'start')}
                        >
                          Старт
                        </Button>
                      )}
                    </div>
                  </li>
                )
              })}
            </ul>
          )}
        </Panel>

        <div className={styles.side}>
          {isAdmin ? (
            <Panel
              title="Агенты"
              icon={<Server size={14} />}
              action={<Link className={styles.panelLink} to="/agents">Все <ArrowUpRight size={13} /></Link>}
              flush
            >
              {agents.length === 0 ? (
                <EmptyState
                  compact
                  icon={Server}
                  title="Агентов нет"
                  description="Подключите агент, чтобы управлять Docker-контейнерами."
                  action={<Button size="sm" onClick={() => navigate('/agents')}>Подключить</Button>}
                />
              ) : (
                <ul className={styles.agentList}>
                  {agents.slice(0, 4).map((agent) => (
                    <AgentMini key={agent.id.toString()} agent={agent} />
                  ))}
                </ul>
              )}
            </Panel>
          ) : null}

          {isAdmin ? (
            <Panel
              title="Ближайшие задачи"
              icon={<CalendarClock size={14} />}
              flush
            >
              {tasks.length === 0 ? (
                <EmptyState compact icon={CalendarClock} title="Расписаний нет" description="Задачи по расписанию настраиваются в экземпляре." />
              ) : (
                <ul className={styles.simpleList}>
                  {tasks.map((task) => (
                    <li key={task.id.toString()} className={styles.simpleRow}>
                      <span className={styles.taskTime}>{formatDate(task.nextRunAt, true)}</span>
                      <span className={styles.simpleMain}>
                        <span className={styles.simpleTitle}>{task.name}</span>
                        <span className={styles.simpleMeta}>{task.instanceName}</span>
                      </span>
                    </li>
                  ))}
                </ul>
              )}
            </Panel>
          ) : null}

          {isAdmin ? (
            <Panel
              title="Журнал входов"
              icon={<ShieldCheck size={14} />}
              action={<Link className={styles.panelLink} to="/security">Все <ArrowUpRight size={13} /></Link>}
              flush
            >
              {logins.length === 0 ? (
                <EmptyState compact icon={ShieldCheck} title="Входов пока не было" />
              ) : (
                <ul className={styles.simpleList}>
                  {logins.map((entry) => (
                    <li key={entry.id.toString()} className={styles.simpleRow}>
                      <span className={cx(styles.loginDot, entry.success ? styles.loginOk : styles.loginFail)} aria-hidden />
                      <span className={styles.simpleMain}>
                        <span className={styles.simpleTitle}>{entry.login || '—'}</span>
                        <span className={styles.simpleMeta}>
                          {entry.success ? 'успешно' : 'отклонён'} · {formatRelative(entry.createdAt)}
                        </span>
                      </span>
                      <span className={styles.ip}>{entry.ip}</span>
                    </li>
                  ))}
                </ul>
              )}
            </Panel>
          ) : null}
        </div>
      </div>
    </div>
  )
}

function AgentMini({ agent }: { agent: Agent }) {
  const history = useSeries(agent.cpuPercent, 36)
  const memoryPercent =
    agent.memoryTotalBytes > 0n ? (Number(agent.memoryUsedBytes) / Number(agent.memoryTotalBytes)) * 100 : 0
  return (
    <li className={styles.agentRow}>
      <div className={styles.agentTop}>
        <span className={cx(styles.agentDot, agent.online ? styles.agentOnline : styles.agentOffline)} aria-hidden />
        <span className={styles.agentName}>{agent.name}</span>
        {!agent.dockerOk && agent.online ? (
          <span title={agent.dockerError || 'Docker недоступен'}>
            <AlertTriangle size={12} className={styles.dockerWarn} />
          </span>
        ) : null}
        <span className={styles.agentCpu}>{formatPercent(agent.cpuPercent)}</span>
      </div>
      <div className={styles.agentMeta}>
        <span className={styles.truncate}>{agent.hostname || 'агент'}</span>
        <span>·</span>
        <span>{agent.latencyMs > 0 ? `${agent.latencyMs} мс` : '—'}</span>
        <span>·</span>
        <span>{agent.instancesRunning}/{agent.instancesTotal} экз.</span>
      </div>
      <div className={styles.agentChart}>
        <Sparkline values={history} height={22} tone={agent.online ? 'success' : 'neutral'} label={`CPU ${agent.name}`} />
      </div>
      <div className={styles.agentMemory}>
        <Meter compact percent={memoryPercent} tone={memoryPercent > 85 ? 'danger' : 'info'} />
        <span className={styles.agentMemoryText}>
          {formatBytes(agent.memoryUsedBytes)} / {formatBytes(agent.memoryTotalBytes)}
        </span>
      </div>
    </li>
  )
}
