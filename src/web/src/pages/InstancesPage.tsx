import { useEffect, useMemo, useState } from 'react'
import { useQueryClient } from '@tanstack/react-query'
import { Link, useNavigate, useSearchParams } from 'react-router-dom'
import {
  Boxes,
  CircleStop,
  FolderOpen,
  Play,
  RefreshCw,
  Settings,
  Terminal,
  Trash2,
  WifiOff,
} from 'lucide-react'
import { api, errorMessage } from '../api/client'
import { keys, useAgentsQuery, useInstancesQuery } from '../data/queries'
import { useSession } from '../data/session'
import type { Instance } from '../gen/panel/v1/panel_pb'
import { InstanceStatus } from '../gen/panel/v1/panel_pb'
import {
  describePorts,
  formatBytes,
  formatPercent,
  formatUptime,
  instanceStatusInfo,
  isTransition,
} from '../lib/format'
import { cx } from '../lib/cx'
import { Badge, CountBadge, EmptyState, InlineNotice, PageHeader, Skeleton } from '../ui/Display'
import { Button } from '../ui/Button'
import { Dialog } from '../ui/Dialog'
import { Menu, type MenuItem } from '../ui/Menu'
import { SearchField } from '../ui/SearchField'
import { Select } from '../ui/Field'
import { Tabs } from '../ui/Tabs'
import { Switch } from '../ui/Toggle'
import { useToast } from '../ui/Toast'
import { InstanceFormDialog } from '../features/instance/InstanceFormDialog'
import styles from './InstancesPage.module.css'

type StatusFilter = 'all' | 'running' | 'stopped' | 'issues'
type ActionKey = 'start' | 'stop' | 'restart'

export function InstancesPage() {
  const { isAdmin } = useSession()
  const instancesQuery = useInstancesQuery()
  const agentsQuery = useAgentsQuery(isAdmin)
  const instances = instancesQuery.data?.items ?? []
  const agents = agentsQuery.data?.items ?? []
  const toast = useToast()
  const queryClient = useQueryClient()
  const navigate = useNavigate()
  const [searchParams, setSearchParams] = useSearchParams()

  const [search, setSearch] = useState('')
  const [statusFilter, setStatusFilter] = useState<StatusFilter>('all')
  const [agentFilter, setAgentFilter] = useState('all')
  const [pending, setPending] = useState<Record<string, ActionKey | null>>({})
  const [createOpen, setCreateOpen] = useState(false)
  const [deleteTarget, setDeleteTarget] = useState<Instance | null>(null)
  const [removeData, setRemoveData] = useState(false)
  const [deleting, setDeleting] = useState(false)

  useEffect(() => {
    if (searchParams.get('new') === '1' && isAdmin) {
      setCreateOpen(true)
      const next = new URLSearchParams(searchParams)
      next.delete('new')
      setSearchParams(next, { replace: true })
    }
  }, [searchParams, isAdmin, setSearchParams])

  const agentOptions = useMemo(() => {
    if (isAdmin) return agents.map((agent) => ({ id: agent.id.toString(), name: agent.name }))
    const unique = new Map<string, string>()
    for (const instance of instances) {
      unique.set(instance.agentId.toString(), instance.agentName)
    }
    return Array.from(unique, ([id, name]) => ({ id, name }))
  }, [agents, instances, isAdmin])

  const filtered = useMemo(() => {
    const query = search.trim().toLowerCase()
    return instances.filter((instance) => {
      if (agentFilter !== 'all' && instance.agentId.toString() !== agentFilter) return false
      if (query) {
        const haystack = `${instance.name} ${instance.image} ${instance.agentName} ${instance.containerName}`.toLowerCase()
        if (!haystack.includes(query)) return false
      }
      const status = instanceStatusInfo(instance.status, instance.exitCode, instance.lastError)
      if (statusFilter === 'running') return instance.status === InstanceStatus.RUNNING
      if (statusFilter === 'stopped') {
        return instance.status === InstanceStatus.STOPPED || instance.status === InstanceStatus.EXITED
      }
      if (statusFilter === 'issues') {
        return instance.status === InstanceStatus.FAILED || status.tone === 'danger' || !instance.agentOnline
      }
      return true
    })
  }, [instances, search, agentFilter, statusFilter])

  const counts = useMemo(
    () => ({
      all: instances.length,
      running: instances.filter((instance) => instance.status === InstanceStatus.RUNNING).length,
      stopped: instances.filter(
        (instance) => instance.status === InstanceStatus.STOPPED || instance.status === InstanceStatus.EXITED,
      ).length,
      issues: instances.filter(
        (instance) => instance.status === InstanceStatus.FAILED || !instance.agentOnline,
      ).length,
    }),
    [instances],
  )

  const runAction = async (instance: Instance, action: ActionKey) => {
    const id = instance.id.toString()
    setPending((current) => ({ ...current, [id]: action }))
    try {
      if (action === 'start') await api.instances.start({ id: instance.id })
      else if (action === 'stop') await api.instances.stop({ id: instance.id })
      else await api.instances.restart({ id: instance.id })
      const messages: Record<ActionKey, string> = {
        start: 'Экземпляр запускается',
        stop: 'Экземпляр останавливается',
        restart: 'Экземпляр перезапускается',
      }
      toast.success(messages[action], instance.name)
      await queryClient.invalidateQueries({ queryKey: keys.instances })
      await queryClient.invalidateQueries({ queryKey: keys.dashboard })
    } catch (error) {
      toast.error('Не удалось выполнить действие', errorMessage(error))
    } finally {
      setPending((current) => ({ ...current, [id]: null }))
    }
  }

  const confirmDelete = async () => {
    if (!deleteTarget) return
    setDeleting(true)
    try {
      await api.instances.delete({ id: deleteTarget.id, removeData })
      toast.success('Экземпляр удалён', deleteTarget.name)
      await queryClient.invalidateQueries({ queryKey: keys.instances })
      await queryClient.invalidateQueries({ queryKey: keys.dashboard })
      setDeleteTarget(null)
      setRemoveData(false)
    } catch (error) {
      toast.error('Не удалось удалить экземпляр', errorMessage(error))
    } finally {
      setDeleting(false)
    }
  }

  const menuFor = (instance: Instance): MenuItem[] => {
    const items: MenuItem[] = []
    if (instance.permissions?.control && instance.agentOnline) {
      items.push({
        id: 'restart',
        label: 'Перезапустить',
        icon: <RefreshCw size={14} />,
        onSelect: () => void runAction(instance, 'restart'),
      })
    }
    if (instance.permissions?.console) {
      items.push({
        id: 'console',
        label: 'Консоль',
        icon: <Terminal size={14} />,
        onSelect: () => navigate(`/instances/${instance.id}?tab=console`),
      })
    }
    if (instance.permissions?.files) {
      items.push({
        id: 'files',
        label: 'Файлы',
        icon: <FolderOpen size={14} />,
        onSelect: () => navigate(`/instances/${instance.id}?tab=files`),
      })
    }
    if (isAdmin) {
      items.push({
        id: 'settings',
        label: 'Настройки',
        icon: <Settings size={14} />,
        separator: items.length > 0,
        onSelect: () => navigate(`/instances/${instance.id}?tab=settings`),
      })
      items.push({
        id: 'delete',
        label: 'Удалить',
        icon: <Trash2 size={14} />,
        danger: true,
        onSelect: () => {
          setDeleteTarget(instance)
          setRemoveData(false)
        },
      })
    }
    return items
  }

  const loading = instancesQuery.isPending
  const error = instancesQuery.error

  return (
    <div className={styles.page}>
      <PageHeader
        title="Экземпляры"
        badge={<CountBadge value={instances.length} />}
        actions={
          isAdmin ? (
            <Button
              variant="primary"
              onClick={() => {
                if (agents.length) setCreateOpen(true)
                else navigate('/agents')
              }}
            >
              {agents.length || loading ? 'Создать экземпляр' : 'Подключить агента'}
            </Button>
          ) : null
        }
      />

      <div className={styles.toolbar}>
        <SearchField
          value={search}
          onChange={setSearch}
          placeholder="Имя, образ или агент"
          className={styles.search}
        />
        <Tabs<StatusFilter>
          variant="pill"
          size="sm"
          ariaLabel="Фильтр по статусу"
          value={statusFilter}
          onChange={setStatusFilter}
          items={[
            { id: 'all', label: 'Все', count: counts.all },
            { id: 'running', label: 'Работают', count: counts.running },
            { id: 'stopped', label: 'Остановлены', count: counts.stopped },
            { id: 'issues', label: 'Проблемы', count: counts.issues },
          ]}
        />
        <div className={styles.agentFilter}>
          <Select
            aria-label="Фильтр по агенту"
            value={agentFilter}
            onChange={(event) => setAgentFilter(event.target.value)}
          >
            <option value="all">Все агенты</option>
            {agentOptions.map((agent) => (
              <option key={agent.id} value={agent.id}>{agent.name}</option>
            ))}
          </Select>
        </div>
      </div>

      {error ? (
        <InlineNotice
          tone="danger"
          title="Не удалось загрузить экземпляры"
          action={<Button size="sm" onClick={() => void instancesQuery.refetch()}>Повторить</Button>}
        >
          {errorMessage(error)}
        </InlineNotice>
      ) : null}

      {loading ? (
        <div className={styles.table}>
          <div className={styles.head}>
            <span>Экземпляр</span><span>Агент</span><span>Образ</span><span>Порты</span><span>Нагрузка</span><span>Статус</span><span />
          </div>
          {Array.from({ length: 5 }, (_, index) => (
            <div className={styles.row} key={index}>
              <div className={styles.cellName}><Skeleton width={160} height={13} /><Skeleton width={110} height={10} /></div>
              <Skeleton width={90} height={11} />
              <Skeleton width={120} height={11} />
              <Skeleton width={64} height={11} />
              <Skeleton width={80} height={11} />
              <Skeleton width={64} height={18} />
              <Skeleton width={60} height={26} />
            </div>
          ))}
        </div>
      ) : filtered.length === 0 ? (
        instances.length === 0 ? (
          <div className={styles.emptyPanel}>
            <EmptyState
              icon={Boxes}
              title={isAdmin ? 'Экземпляров пока нет' : 'Нет доступных экземпляров'}
              description={
                isAdmin
                  ? agents.length
                    ? 'Создайте контейнер из Docker-образа: укажите порты, тома и лимиты.'
                    : 'Чтобы запускать контейнеры, подключите агента на Docker-хосте.'
                  : 'Администратор панели ещё не выдал доступ ни к одному экземпляру.'
              }
              action={
                isAdmin ? (
                  <Button variant="primary" onClick={() => (agents.length ? setCreateOpen(true) : navigate('/agents'))}>
                    {agents.length ? 'Создать экземпляр' : 'Подключить агента'}
                  </Button>
                ) : undefined
              }
            />
          </div>
        ) : (
          <div className={styles.emptyPanel}>
            <EmptyState
              icon={Boxes}
              title="Ничего не найдено"
              description="Измените запрос или сбросьте фильтры."
              action={
                <Button
                  onClick={() => {
                    setSearch('')
                    setStatusFilter('all')
                    setAgentFilter('all')
                  }}
                >
                  Сбросить фильтры
                </Button>
              }
            />
          </div>
        )
      ) : (
        <>
          <div className={styles.table}>
            <div className={styles.head}>
              <span>Экземпляр</span>
              <span>Агент</span>
              <span>Образ</span>
              <span>Порты</span>
              <span>Нагрузка</span>
              <span>Статус</span>
              <span />
            </div>
            {filtered.map((instance) => {
              const status = instanceStatusInfo(instance.status, instance.exitCode, instance.lastError)
              const action = pending[instance.id.toString()]
              const busy = Boolean(action) || isTransition(instance.status)
              const canControl = Boolean(instance.permissions?.control && instance.agentOnline)
              return (
                <div className={styles.row} key={instance.id.toString()}>
                  <div className={styles.cellName}>
                    <span className={cx(styles.statusDot, styles[`dot-${status.tone}`])} aria-hidden />
                    <div className={styles.nameBlock}>
                      <Link to={`/instances/${instance.id}`} className={styles.name}>
                        {instance.name}
                      </Link>
                      <span className={styles.meta}>
                        {instance.lastStartedAt && instance.status === InstanceStatus.RUNNING
                          ? `работает ${formatUptime(instance.lastStartedAt)}`
                          : instance.exitCode !== 0
                            ? `код выхода ${instance.exitCode}`
                            : instance.containerName || 'контейнер не создан'}
                      </span>
                    </div>
                  </div>
                  <div className={styles.cellAgent}>
                    <span className={instance.agentOnline ? styles.onlineMark : styles.offlineMark} aria-hidden />
                    <span className={styles.truncate} title={instance.agentName}>{instance.agentName}</span>
                    {!instance.agentOnline ? <WifiOff size={12} className={styles.warnIcon} aria-label="Агент офлайн" /> : null}
                  </div>
                  <span className={cx(styles.cellImage, styles.truncate)} title={instance.image}>{instance.image}</span>
                  <span className={cx(styles.cellPorts, styles.mono)}>{describePorts(instance.ports)}</span>
                  <div className={styles.cellUsage}>
                    <span className={styles.usageLine}>
                      <span className={styles.usageKey}>CPU</span>
                      <span className={styles.usageValue}>{formatPercent(instance.usage?.cpuPercent ?? 0)}</span>
                    </span>
                    <span className={styles.usageLine}>
                      <span className={styles.usageKey}>RAM</span>
                      <span className={styles.usageValue}>{formatBytes(instance.usage?.memoryBytes ?? 0n)}</span>
                    </span>
                  </div>
                  <div className={styles.cellStatus}>
                    <Badge tone={status.tone} dot>{status.label}</Badge>
                  </div>
                  <div className={styles.cellActions}>
                    {instance.status === InstanceStatus.RUNNING || instance.status === InstanceStatus.STARTING ? (
                      <Button
                        variant="ghost"
                        size="sm"
                        iconOnly
                        aria-label={`Остановить ${instance.name}`}
                        title={`Остановить ${instance.name}`}
                        icon={<CircleStop size={14} />}
                        loading={action === 'stop'}
                        disabled={!canControl || busy}
                        onClick={() => void runAction(instance, 'stop')}
                      />
                    ) : (
                      <Button
                        variant="ghost"
                        size="sm"
                        iconOnly
                        aria-label={`Запустить ${instance.name}`}
                        title={`Запустить ${instance.name}`}
                        icon={<Play size={14} />}
                        loading={action === 'start'}
                        disabled={!canControl || busy}
                        onClick={() => void runAction(instance, 'start')}
                      />
                    )}
                    <Menu items={menuFor(instance)} label={`Действия: ${instance.name}`} />
                  </div>
                </div>
              )
            })}
          </div>
          <p className={styles.footerNote}>
            Показано {filtered.length} из {instances.length}
          </p>
        </>
      )}

      <InstanceFormDialog
        open={createOpen}
        onClose={() => setCreateOpen(false)}
        agents={agents}
      />

      <Dialog
        open={Boolean(deleteTarget)}
        onClose={() => {
          if (!deleting) {
            setDeleteTarget(null)
            setRemoveData(false)
          }
        }}
        title="Удалить экземпляр?"
        description={deleteTarget ? `Контейнер «${deleteTarget.containerName}» будет удалён с агента ${deleteTarget.agentName}.` : undefined}
        size="sm"
        dismissible={!deleting}
        footer={
          <>
            <Button variant="ghost" onClick={() => setDeleteTarget(null)} disabled={deleting}>Отмена</Button>
            <Button variant="dangerSolid" onClick={confirmDelete} loading={deleting}>Удалить</Button>
          </>
        }
      >
        <div className={styles.deleteBody}>
          <Switch
            checked={removeData}
            onChange={setRemoveData}
            label="Удалить каталог данных"
            description="Файлы экземпляра на агенте будут удалены без возможности восстановления."
          />
          {deleteTarget && !deleteTarget.agentOnline ? (
            <InlineNotice tone="warning" title="Агент офлайн">
              Контейнер будет удалён при подключении агента. Каталог данных удалить сейчас нельзя.
            </InlineNotice>
          ) : null}
        </div>
      </Dialog>
    </div>
  )
}
