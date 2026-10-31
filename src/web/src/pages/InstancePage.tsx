import { useMemo, useState } from 'react'
import type { ReactNode } from 'react'
import { useQueryClient } from '@tanstack/react-query'
import { Link, useNavigate, useParams, useSearchParams } from 'react-router-dom'
import {
  ArrowLeft,
  Boxes,
  CalendarClock,
  FolderOpen,
  Pencil,
  Play,
  RotateCw,
  Settings2,
  Square,
  Terminal,
  Trash2,
  WifiOff,
  Zap,
} from 'lucide-react'
import { api, errorMessage } from '../api/client'
import { keys, useInstance, useInstancesQuery } from '../data/queries'
import { useSession } from '../data/session'
import { InstanceStatus } from '../gen/panel/v1/panel_pb'
import {
  describePorts,
  formatBytes,
  formatDate,
  formatRate,
  formatUptime,
  instanceStatusInfo,
  isTransition,
} from '../lib/format'
import { cx } from '../lib/cx'
import { Badge, EmptyState, InlineNotice, PageHeader, Skeleton } from '../ui/Display'
import { Button } from '../ui/Button'
import { Dialog } from '../ui/Dialog'
import { Menu, type MenuItem } from '../ui/Menu'
import { Tabs } from '../ui/Tabs'
import { Switch } from '../ui/Toggle'
import { useToast } from '../ui/Toast'
import { InstanceFormDialog } from '../features/instance/InstanceFormDialog'
import { ConsoleTab } from './instance/ConsoleTab'
import { FilesTab } from './instance/FilesTab'
import { TasksTab } from './instance/TasksTab'
import { SettingsTab } from './instance/SettingsTab'
import styles from './InstancePage.module.css'

type TabId = 'console' | 'files' | 'tasks' | 'settings'

export function InstancePage() {
  const { id } = useParams<{ id: string }>()
  const navigate = useNavigate()
  const toast = useToast()
  const queryClient = useQueryClient()
  const { isAdmin } = useSession()
  const instancesQuery = useInstancesQuery()
  const instance = useInstance(id)
  const [searchParams, setSearchParams] = useSearchParams()
  const [pending, setPending] = useState<'start' | 'stop' | 'restart' | 'kill' | null>(null)
  const [deleteOpen, setDeleteOpen] = useState(false)
  const [removeData, setRemoveData] = useState(false)
  const [deleting, setDeleting] = useState(false)
  const [editOpen, setEditOpen] = useState(false)

  const tabs = useMemo(() => {
    if (!instance) return []
    const items: { id: TabId; label: string; icon: ReactNode }[] = []
    if (instance.permissions?.console || isAdmin) {
      items.push({ id: 'console', label: 'Консоль', icon: <Terminal size={14} /> })
    }
    if (instance.permissions?.files || isAdmin) {
      items.push({ id: 'files', label: 'Файлы', icon: <FolderOpen size={14} /> })
    }
    if (instance.permissions?.control || isAdmin) {
      items.push({ id: 'tasks', label: 'Расписание', icon: <CalendarClock size={14} /> })
    }
    if (isAdmin) {
      items.push({ id: 'settings', label: 'Настройки', icon: <Settings2 size={14} /> })
    }
    return items
  }, [instance, isAdmin])

  const requested = searchParams.get('tab') as TabId | null
  const activeTab: TabId | null =
    requested && tabs.some((tab) => tab.id === requested) ? requested : (tabs[0]?.id ?? null)

  const changeTab = (next: TabId) => {
    const params = new URLSearchParams(searchParams)
    params.set('tab', next)
    setSearchParams(params, { replace: true })
  }

  const refresh = async () => {
    await queryClient.invalidateQueries({ queryKey: keys.instances })
    await queryClient.invalidateQueries({ queryKey: keys.dashboard })
  }

  const run = async (action: 'start' | 'stop' | 'restart' | 'kill') => {
    if (!instance) return
    setPending(action)
    try {
      if (action === 'start') await api.instances.start({ id: instance.id })
      else if (action === 'stop') await api.instances.stop({ id: instance.id })
      else if (action === 'restart') await api.instances.restart({ id: instance.id })
      else await api.instances.kill({ id: instance.id })
      const messages = {
        start: 'Экземпляр запускается',
        stop: 'Экземпляр останавливается',
        restart: 'Экземпляр перезапускается',
        kill: 'Экземпляр остановлен принудительно',
      }
      toast.success(messages[action], instance.name)
      await refresh()
    } catch (error) {
      toast.error('Не удалось выполнить действие', errorMessage(error))
    } finally {
      setPending(null)
    }
  }

  const remove = async () => {
    if (!instance) return
    setDeleting(true)
    try {
      await api.instances.delete({ id: instance.id, removeData })
      toast.success('Экземпляр удалён', instance.name)
      await refresh()
      navigate('/instances')
    } catch (error) {
      toast.error('Не удалось удалить экземпляр', errorMessage(error))
      setDeleting(false)
    }
  }

  if (!instance) {
    if (instancesQuery.isPending) {
      return (
        <div className={styles.page}>
          <Skeleton width={180} height={14} />
          <div className={styles.skeletonTitle}>
            <Skeleton width={240} height={26} />
            <Skeleton width={80} height={20} />
          </div>
          <div className={styles.facts}>
            {Array.from({ length: 5 }, (_, index) => (
              <div className={styles.fact} key={index}>
                <Skeleton width={60} height={10} />
                <Skeleton width="70%" height={13} />
              </div>
            ))}
          </div>
          <Skeleton width="100%" height={300} radius={12} />
        </div>
      )
    }
    return (
      <div className={styles.missing}>
        <EmptyState
          icon={Boxes}
          title="Экземпляр не найден"
          description={
            instancesQuery.isError
              ? errorMessage(instancesQuery.error)
              : 'Возможно, он был удалён или доступ к нему не выдан.'
          }
          action={<Button onClick={() => navigate('/instances')}>К экземплярам</Button>}
        />
      </div>
    )
  }

  const status = instanceStatusInfo(instance.status, instance.exitCode, instance.lastError)
  const running = instance.status === InstanceStatus.RUNNING
  const transitional = isTransition(instance.status)
  const canControl = Boolean(instance.permissions?.control && instance.agentOnline)
  const busy = transitional || pending !== null
  const usage = instance.usage

  const menuItems: MenuItem[] = []
  if (isAdmin) {
    menuItems.push({
      id: 'edit',
      label: 'Изменить конфигурацию',
      icon: <Pencil size={14} />,
      onSelect: () => setEditOpen(true),
    })
  }
  if (instance.permissions?.control && instance.agentOnline && (running || transitional)) {
    menuItems.push({
      id: 'kill',
      label: 'Остановить принудительно',
      icon: <Zap size={14} />,
      onSelect: () => void run('kill'),
    })
  }
  if (isAdmin) {
    menuItems.push({
      id: 'delete',
      label: 'Удалить экземпляр',
      icon: <Trash2 size={14} />,
      danger: true,
      separator: menuItems.length > 0,
      onSelect: () => {
        setRemoveData(false)
        setDeleteOpen(true)
      },
    })
  }

  return (
    <div className={styles.page}>
      <PageHeader
        back={
          <Link to="/instances" className={styles.back}>
            <ArrowLeft size={14} /> Экземпляры
          </Link>
        }
        title={instance.name}
        badge={<Badge tone={status.tone} dot>{status.label}</Badge>}
        description={
          <span className={styles.subline}>
            {instance.agentName}
            {!instance.agentOnline ? <span className={styles.offline}> · агент офлайн</span> : null}
            {' · '}
            <code className={styles.code}>{instance.image}</code>
          </span>
        }
        actions={
          <>
            {running || transitional ? (
              <>
                <Button
                  variant="secondary"
                  icon={<RotateCw size={14} />}
                  onClick={() => void run('restart')}
                  loading={pending === 'restart'}
                  disabled={!canControl || busy}
                >
                  Перезапустить
                </Button>
                <Button
                  variant="secondary"
                  icon={<Square size={13} />}
                  onClick={() => void run('stop')}
                  loading={pending === 'stop'}
                  disabled={!canControl || busy}
                >
                  Остановить
                </Button>
              </>
            ) : (
              <Button
                variant="primary"
                icon={<Play size={14} />}
                onClick={() => void run('start')}
                loading={pending === 'start'}
                disabled={!canControl || busy}
              >
                Запустить
              </Button>
            )}
            {menuItems.length ? <Menu items={menuItems} label="Действия экземпляра" size="md" /> : null}
          </>
        }
      />

      {!instance.agentOnline ? (
        <InlineNotice tone="warning" title="Агент офлайн" icon={WifiOff}>
          Действия станут доступны после восстановления соединения. Последнее известное состояние
          сохранено.
        </InlineNotice>
      ) : null}
      {instance.status === InstanceStatus.FAILED && instance.lastError ? (
        <InlineNotice tone="danger" title="Экземпляр не запущен">
          {instance.lastError}
        </InlineNotice>
      ) : null}
      {!tabs.length ? (
        <InlineNotice tone="info" title="Нет доступных действий">
          У вашей учётной записи есть доступ к экземпляру, но без прав на консоль, файлы или управление.
        </InlineNotice>
      ) : null}

      <div className={styles.facts}>
        <div className={styles.fact}>
          <span className={styles.factLabel}>Агент</span>
          <span className={styles.factValue}>
            <span className={cx(styles.dot, instance.agentOnline ? styles.dotOn : styles.dotOff)} aria-hidden />
            {instance.agentName}
          </span>
        </div>
        <div className={styles.fact}>
          <span className={styles.factLabel}>Порты</span>
          <span className={cx(styles.factValue, styles.monoValue)}>{describePorts(instance.ports)}</span>
        </div>
        <div className={styles.fact}>
          <span className={styles.factLabel}>Запущен</span>
          <span className={styles.factValue}>
            {running ? formatUptime(instance.lastStartedAt) : instance.lastStartedAt ? formatDate(instance.lastStartedAt) : '—'}
          </span>
        </div>
        <div className={styles.fact}>
          <span className={styles.factLabel}>CPU</span>
          <span className={cx(styles.factValue, styles.monoValue)}>
            {running ? `${(usage?.cpuPercent ?? 0).toFixed(1)}%` : '—'}
          </span>
        </div>
        <div className={styles.fact}>
          <span className={styles.factLabel}>Память</span>
          <span className={cx(styles.factValue, styles.monoValue)}>
            {running ? formatBytes(usage?.memoryBytes ?? 0n) : '—'}
            {usage?.memoryLimitBytes ? <span className={styles.factMuted}> / {formatBytes(usage.memoryLimitBytes)}</span> : null}
          </span>
        </div>
        <div className={styles.fact}>
          <span className={styles.factLabel}>Сеть</span>
          <span className={cx(styles.factValue, styles.monoValue)}>
            {running ? `↓ ${formatRate(usage?.networkInBps ?? 0n)} ↑ ${formatRate(usage?.networkOutBps ?? 0n)}` : '—'}
          </span>
        </div>
      </div>

      {tabs.length ? (
        <>
          <Tabs<TabId>
            items={tabs}
            value={activeTab ?? tabs[0].id}
            onChange={(next) => changeTab(next)}
            ariaLabel="Разделы экземпляра"
            className={styles.tabs}
          />
          <div className={styles.tabContent}>
            {activeTab === 'console' ? <ConsoleTab instance={instance} /> : null}
            {activeTab === 'files' ? <FilesTab instance={instance} /> : null}
            {activeTab === 'tasks' ? <TasksTab instance={instance} /> : null}
            {activeTab === 'settings' ? <SettingsTab instance={instance} /> : null}
          </div>
        </>
      ) : null}

      <InstanceFormDialog
        open={editOpen}
        onClose={() => setEditOpen(false)}
        instance={instance}
        agents={[]}
      />

      <Dialog
        open={deleteOpen}
        onClose={() => {
          if (!deleting) {
            setDeleteOpen(false)
            setRemoveData(false)
          }
        }}
        title="Удалить экземпляр?"
        description={`${instance.name} · ${instance.containerName}`}
        size="sm"
        dismissible={!deleting}
        footer={
          <>
            <Button variant="ghost" onClick={() => setDeleteOpen(false)} disabled={deleting}>Отмена</Button>
            <Button variant="dangerSolid" onClick={() => void remove()} loading={deleting}>Удалить</Button>
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
          {!instance.agentOnline ? (
            <InlineNotice tone="warning" title="Агент офлайн">
              Контейнер будет удалён при подключении агента. Каталог данных сейчас удалить нельзя.
            </InlineNotice>
          ) : null}
        </div>
      </Dialog>
    </div>
  )
}
