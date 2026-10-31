import { useMemo, useState } from 'react'
import { useQueryClient } from '@tanstack/react-query'
import {
  AlertTriangle,
  Cpu,
  HardDrive,
  MemoryStick,
  RefreshCw,
  Server,
  ShieldCheck,
  Trash2,
  Wifi,
} from 'lucide-react'
import { api, errorMessage } from '../api/client'
import { keys, useAgentsQuery } from '../data/queries'
import type { Agent, AgentCreated, AgentToken } from '../gen/panel/v1/panel_pb'
import {
  formatBytes,
  formatDate,
  formatPercent,
  formatRate,
  formatRelative,
} from '../lib/format'
import { cx } from '../lib/cx'
import {
  Badge,
  CopyField,
  CountBadge,
  EmptyState,
  InlineNotice,
  KeyValue,
  Meter,
  PageHeader,
  Sparkline,
  Skeleton,
} from '../ui/Display'
import { Button } from '../ui/Button'
import { Dialog, Drawer } from '../ui/Dialog'
import { Field, Input } from '../ui/Field'
import { Menu, type MenuItem } from '../ui/Menu'
import { Panel } from '../ui/Panel'
import { useToast } from '../ui/Toast'
import { useSeries } from '../data/useSeries'
import styles from './AgentsPage.module.css'

type TokenState = {
  title: string
  token: string
  endpoint?: string
}

export function AgentsPage() {
  const agentsQuery = useAgentsQuery(true)
  const queryClient = useQueryClient()
  const toast = useToast()
  const agents = agentsQuery.data?.items ?? []
  const [createOpen, setCreateOpen] = useState(false)
  const [editTarget, setEditTarget] = useState<Agent | null>(null)
  const [deleteTarget, setDeleteTarget] = useState<Agent | null>(null)
  const [rotateTarget, setRotateTarget] = useState<Agent | null>(null)
  const [detailsTarget, setDetailsTarget] = useState<Agent | null>(null)
  const [tokenState, setTokenState] = useState<TokenState | null>(null)
  const [name, setName] = useState('')
  const [country, setCountry] = useState('')
  const [busy, setBusy] = useState(false)
  const [formError, setFormError] = useState('')

  const refresh = async () => {
    await queryClient.invalidateQueries({ queryKey: keys.agents })
    await queryClient.invalidateQueries({ queryKey: keys.instances })
    await queryClient.invalidateQueries({ queryKey: keys.dashboard })
  }

  const openCreate = () => {
    setName('')
    setCountry('')
    setFormError('')
    setCreateOpen(true)
  }

  const openEdit = (agent: Agent) => {
    setEditTarget(agent)
    setName(agent.name)
    setCountry(agent.country)
    setFormError('')
  }

  const submitCreate = async () => {
    if (name.trim().length < 2) {
      setFormError('Название: не менее 2 символов')
      return
    }
    setBusy(true)
    setFormError('')
    try {
      const created: AgentCreated = await api.agents.create({ name: name.trim(), country: country.trim() })
      setCreateOpen(false)
      setTokenState({
        title: 'Агент создан',
        token: created.token,
        endpoint: created.endpoint,
      })
      toast.success('Агент создан', 'Сохраните токен — он показывается один раз.')
      await refresh()
    } catch (error) {
      setFormError(errorMessage(error))
    } finally {
      setBusy(false)
    }
  }

  const submitEdit = async () => {
    if (!editTarget) return
    if (name.trim().length < 2) {
      setFormError('Название: не менее 2 символов')
      return
    }
    setBusy(true)
    setFormError('')
    try {
      await api.agents.update({ id: editTarget.id, name: name.trim(), country: country.trim() })
      toast.success('Агент обновлён', name.trim())
      setEditTarget(null)
      await refresh()
    } catch (error) {
      setFormError(errorMessage(error))
    } finally {
      setBusy(false)
    }
  }

  const rotate = async () => {
    if (!rotateTarget) return
    setBusy(true)
    try {
      const result: AgentToken = await api.agents.rotateToken({ id: rotateTarget.id })
      const agent = rotateTarget
      setRotateTarget(null)
      setTokenState({ title: `Новый токен: ${agent.name}`, token: result.token })
      toast.success('Токен перевыпущен', 'Агент будет переподключён с новым токеном.')
      await refresh()
    } catch (error) {
      toast.error('Не удалось перевыпустить токен', errorMessage(error))
    } finally {
      setBusy(false)
    }
  }

  const remove = async () => {
    if (!deleteTarget) return
    setBusy(true)
    try {
      await api.agents.delete({ id: deleteTarget.id })
      toast.success('Агент удалён', deleteTarget.name)
      setDeleteTarget(null)
      await refresh()
    } catch (error) {
      toast.error('Не удалось удалить агента', errorMessage(error))
    } finally {
      setBusy(false)
    }
  }

  const menuFor = (agent: Agent): MenuItem[] => [
    {
      id: 'details',
      label: 'Подробности',
      icon: <ShieldCheck size={14} />,
      onSelect: () => setDetailsTarget(agent),
    },
    {
      id: 'edit',
      label: 'Изменить',
      onSelect: () => openEdit(agent),
    },
    {
      id: 'rotate',
      label: 'Перевыпустить токен',
      icon: <RefreshCw size={14} />,
      onSelect: () => setRotateTarget(agent),
    },
    {
      id: 'delete',
      label: 'Удалить',
      icon: <Trash2 size={14} />,
      danger: true,
      separator: true,
      disabled: agent.instancesTotal > 0,
      onSelect: () => setDeleteTarget(agent),
    },
  ]

  const online = agents.filter((agent) => agent.online).length

  return (
    <div className={styles.page}>
      <PageHeader
        title="Агенты"
        badge={<CountBadge value={agents.length} />}
        description="Агент — исходящее gRPC-соединение с Docker-хостом. Он выполняет команды контейнеров, консоли и файлов."
        actions={<Button variant="primary" onClick={openCreate}>Подключить агента</Button>}
      />

      {agentsQuery.isError ? (
        <InlineNotice tone="danger" title="Не удалось загрузить агентов">
          {errorMessage(agentsQuery.error)}
        </InlineNotice>
      ) : null}

      {agents.length > 0 ? (
        <div className={styles.summary}>
          <span className={styles.summaryItem}>
            <Wifi size={13} />
            {online} из {agents.length} на связи
          </span>
          <span className={styles.summarySep} />
          <span className={styles.summaryItem}>
            <Server size={13} />
            {agents.reduce((sum, agent) => sum + agent.instancesTotal, 0)} экземпляров
          </span>
          <span className={styles.summarySep} />
          <span className={styles.summaryItem}>
            <Cpu size={13} />
            {formatPercent(
              online
                ? agents.filter((agent) => agent.online).reduce((sum, agent) => sum + agent.cpuPercent, 0) / online
                : 0,
            )}{' '}
            средняя нагрузка
          </span>
        </div>
      ) : null}

      {agentsQuery.isPending ? (
        <div className={styles.list}>
          {Array.from({ length: 3 }, (_, index) => (
            <div className={styles.row} key={index}>
              <div className={styles.nameCell}>
                <Skeleton width={140} height={13} />
                <Skeleton width={180} height={10} />
              </div>
              <Skeleton width={110} height={10} />
              <Skeleton width={160} height={10} />
              <Skeleton width={70} height={20} />
            </div>
          ))}
        </div>
      ) : agents.length === 0 ? (
        <div className={styles.emptyPanel}>
          <EmptyState
            icon={Server}
            title="Нет подключённых агентов"
            description="Создайте токен, запустите агент на Docker-хосте — и он появится здесь с живыми метриками."
            action={<Button variant="primary" onClick={openCreate}>Подключить агента</Button>}
          />
        </div>
      ) : (
        <div className={styles.list} role="list">
          {agents.map((agent) => (
            <AgentRow key={agent.id.toString()} agent={agent} menu={menuFor(agent)} onOpen={() => setDetailsTarget(agent)} />
          ))}
        </div>
      )}

      <Dialog
        open={createOpen}
        onClose={() => setCreateOpen(false)}
        title="Подключить агента"
        description="Токен создаётся один раз. Передайте его агенту через переменную окружения DOCKIUM_AGENT_TOKEN."
        size="md"
        dismissible={!busy}
        footer={
          <>
            <Button variant="ghost" onClick={() => setCreateOpen(false)} disabled={busy}>Отмена</Button>
            <Button variant="primary" onClick={() => void submitCreate()} loading={busy}>Создать токен</Button>
          </>
        }
      >
        <div className={styles.form}>
          <Field label="Название агента" required error={formError} htmlFor="agent-name" hint="Например, «EU-1» или «Домашний сервер»">
            <Input
              id="agent-name"
              value={name}
              onChange={(event) => setName(event.target.value)}
              placeholder="EU-1"
              maxLength={64}
              autoFocus
              invalid={Boolean(formError)}
            />
          </Field>
          <Field label="Страна или регион" htmlFor="agent-country" optional>
            <Input
              id="agent-country"
              value={country}
              onChange={(event) => setCountry(event.target.value)}
              placeholder="Россия"
              maxLength={64}
            />
          </Field>
        </div>
      </Dialog>

      <Dialog
        open={Boolean(editTarget)}
        onClose={() => setEditTarget(null)}
        title="Изменить агента"
        size="sm"
        dismissible={!busy}
        footer={
          <>
            <Button variant="ghost" onClick={() => setEditTarget(null)} disabled={busy}>Отмена</Button>
            <Button variant="primary" onClick={() => void submitEdit()} loading={busy}>Сохранить</Button>
          </>
        }
      >
        <div className={styles.form}>
          <Field label="Название" required error={formError} htmlFor="agent-edit-name">
            <Input
              id="agent-edit-name"
              value={name}
              onChange={(event) => setName(event.target.value)}
              maxLength={64}
              autoFocus
              invalid={Boolean(formError)}
            />
          </Field>
          <Field label="Страна или регион" htmlFor="agent-edit-country" optional>
            <Input
              id="agent-edit-country"
              value={country}
              onChange={(event) => setCountry(event.target.value)}
              maxLength={64}
            />
          </Field>
        </div>
      </Dialog>

      <Dialog
        open={Boolean(rotateTarget)}
        onClose={() => setRotateTarget(null)}
        title="Перевыпустить токен?"
        description={rotateTarget ? `Текущее соединение агента ${rotateTarget.name} будет закрыто.` : undefined}
        size="sm"
        dismissible={!busy}
        footer={
          <>
            <Button variant="ghost" onClick={() => setRotateTarget(null)} disabled={busy}>Отмена</Button>
            <Button variant="primary" onClick={() => void rotate()} loading={busy}>Перевыпустить</Button>
          </>
        }
      >
        <p className={styles.dialogText}>
          Агенту понадобится новый токен: обновите DOCKIUM_AGENT_TOKEN и перезапустите агент.
          Старый токен перестанет работать.
        </p>
      </Dialog>

      <Dialog
        open={Boolean(deleteTarget)}
        onClose={() => setDeleteTarget(null)}
        title="Удалить агента?"
        description={deleteTarget?.name}
        size="sm"
        dismissible={!busy}
        footer={
          <>
            <Button variant="ghost" onClick={() => setDeleteTarget(null)} disabled={busy}>Отмена</Button>
            <Button variant="dangerSolid" onClick={() => void remove()} loading={busy} disabled={Boolean(deleteTarget?.instancesTotal)}>
              Удалить
            </Button>
          </>
        }
      >
        {deleteTarget && deleteTarget.instancesTotal > 0 ? (
          <InlineNotice tone="warning" title="Сначала удалите экземпляры" icon={AlertTriangle}>
            На агенте {deleteTarget.instancesTotal} экземпляров. Удаление агента удалит и их
            конфигурации.
          </InlineNotice>
        ) : (
          <p className={styles.dialogText}>
            Агент и его метрики будут удалены из панели. Контейнеры на хосте не удаляются, но
            перестанут быть управляемыми.
          </p>
        )}
      </Dialog>

      <TokenDialog state={tokenState} onClose={() => setTokenState(null)} />

      <Drawer
        open={Boolean(detailsTarget)}
        onClose={() => setDetailsTarget(null)}
        title={detailsTarget?.name}
        description={detailsTarget ? `${detailsTarget.hostname || 'агент'} · ${detailsTarget.platform}/${detailsTarget.arch}` : undefined}
        width={560}
      >
        {detailsTarget ? <AgentDetails agent={detailsTarget} /> : null}
      </Drawer>
    </div>
  )
}

function AgentRow({ agent, menu, onOpen }: { agent: Agent; menu: MenuItem[]; onOpen: () => void }) {
  const history = useSeries(agent.cpuPercent, 40)
  const memoryPercent =
    agent.memoryTotalBytes > 0n ? (Number(agent.memoryUsedBytes) / Number(agent.memoryTotalBytes)) * 100 : 0
  const diskPercent =
    agent.diskTotalBytes > 0n ? (Number(agent.diskUsedBytes) / Number(agent.diskTotalBytes)) * 100 : 0
  return (
    <div className={styles.row} role="listitem">
      <button type="button" className={styles.nameCell} onClick={onOpen}>
        <span className={cx(styles.dot, agent.online ? styles.dotOn : styles.dotOff)} aria-hidden />
        <span className={styles.nameBlock}>
          <span className={styles.name}>
            {agent.name}
            {agent.country ? <span className={styles.country}> · {agent.country}</span> : null}
          </span>
          <span className={styles.meta}>
            {agent.hostname || 'хост неизвестен'}
            {agent.online && agent.latencyMs > 0 ? ` · ${agent.latencyMs} мс` : ''}
            {!agent.online && agent.lastSeenAt ? ` · был ${formatRelative(agent.lastSeenAt)}` : ''}
          </span>
        </span>
        <span className={styles.agentBadge}>
          <Badge tone={agent.online ? 'success' : 'neutral'} dot>
            {agent.online ? 'онлайн' : 'офлайн'}
          </Badge>
        </span>
      </button>

      <div className={styles.versionCell}>
        <span className={styles.cellLabel}>Docker</span>
        <span className={styles.cellValue}>
          {agent.dockerVersion || '—'}
          {!agent.dockerOk ? <AlertTriangle size={12} className={styles.warn} aria-label={agent.dockerError || 'Docker недоступен'} /> : null}
        </span>
      </div>

      <div className={styles.resources}>
        <div className={styles.resource}>
          <span className={styles.resourceHead}>
            <Cpu size={11} />
            {formatPercent(agent.cpuPercent)}
          </span>
          <Sparkline values={history} height={16} tone={agent.online ? 'success' : 'neutral'} />
        </div>
        <div className={styles.resource}>
          <span className={styles.resourceHead}>
            <MemoryStick size={11} />
            {formatBytes(agent.memoryUsedBytes)}
          </span>
          <Meter compact percent={memoryPercent} tone={memoryPercent > 85 ? 'danger' : 'info'} />
        </div>
        <div className={styles.resource}>
          <span className={styles.resourceHead}>
            <HardDrive size={11} />
            {formatBytes(agent.diskUsedBytes)}
          </span>
          <Meter compact percent={diskPercent} tone={diskPercent > 90 ? 'danger' : 'accent'} />
        </div>
      </div>

      <div className={styles.instancesCell}>
        <span className={styles.cellLabel}>Экземпляры</span>
        <span className={styles.cellValue}>
          {agent.instancesRunning}/{agent.instancesTotal}
        </span>
      </div>

      <div className={styles.actionsCell}>
        <Menu items={menu} label={`Действия: ${agent.name}`} />
      </div>
    </div>
  )
}

function AgentDetails({ agent }: { agent: Agent }) {
  const history = useSeries(agent.cpuPercent, 48)
  const memoryPercent =
    agent.memoryTotalBytes > 0n ? (Number(agent.memoryUsedBytes) / Number(agent.memoryTotalBytes)) * 100 : 0
  const diskPercent =
    agent.diskTotalBytes > 0n ? (Number(agent.diskUsedBytes) / Number(agent.diskTotalBytes)) * 100 : 0
  return (
    <div className={styles.details}>
      {!agent.dockerOk ? (
        <InlineNotice tone="warning" title="Docker недоступен" icon={AlertTriangle}>
          {agent.dockerError || 'Агент не может связаться с Docker. Экземпляры не запустятся.'}
        </InlineNotice>
      ) : null}
      <Panel title="Нагрузка">
        <div className={styles.detailChart}>
          <Sparkline values={history} height={46} tone={agent.online ? 'success' : 'neutral'} label="Загрузка CPU" />
        </div>
        <div className={styles.detailMeters}>
          <Meter label="CPU" value={formatPercent(agent.cpuPercent)} percent={agent.cpuPercent} tone="success" />
          <Meter
            label="Память"
            value={`${formatBytes(agent.memoryUsedBytes)} / ${formatBytes(agent.memoryTotalBytes)}`}
            percent={memoryPercent}
            tone="info"
          />
          <Meter
            label="Диск"
            value={`${formatBytes(agent.diskUsedBytes)} / ${formatBytes(agent.diskTotalBytes)}`}
            percent={diskPercent}
            tone="accent"
          />
          <div className={styles.networkRow}>
            <span>Сеть</span>
            <span className={styles.mono}>↓ {formatRate(agent.networkInBps)} ↑ {formatRate(agent.networkOutBps)}</span>
          </div>
        </div>
      </Panel>
      <Panel title="Система">
        <KeyValue
          columns={2}
          items={[
            { label: 'Идентификатор', value: <code className={styles.mono}>#{agent.id}</code> },
            { label: 'Платформа', value: `${agent.platform || '—'} / ${agent.arch || '—'}` },
            { label: 'CPU', value: agent.cpuModel || '—' },
            { label: 'Ядра', value: agent.cpuCores ? String(agent.cpuCores) : '—' },
            { label: 'Версия Docker', value: agent.dockerVersion || '—' },
            { label: 'Docker', value: agent.dockerOk ? 'доступен' : 'недоступен' },
            { label: 'Версия агента', value: agent.version || '—' },
            { label: 'Задержка', value: agent.latencyMs ? `${agent.latencyMs} мс` : '—' },
            { label: 'Подключён', value: formatDate(agent.connectedAt) },
            { label: 'Последняя связь', value: formatDate(agent.lastSeenAt) },
            { label: 'Создан', value: formatDate(agent.createdAt) },
            { label: 'Экземпляры', value: `${agent.instancesRunning} из ${agent.instancesTotal} работают` },
          ]}
        />
      </Panel>
    </div>
  )
}

function TokenDialog({ state, onClose }: { state: TokenState | null; onClose: () => void }) {
  const commands = useMemo(() => {
    if (!state) return null
    const endpoint = state.endpoint || 'ПАНЕЛЬ:3443'
    const docker = `docker run -d --name dockium-agent --restart unless-stopped -e DOCKIUM_PANEL_URL=${endpoint} -e DOCKIUM_AGENT_TOKEN=${state.token} -v /var/run/docker.sock:/var/run/docker.sock -v dockium-agent-data:/data dockium-agent:latest`
    const binary = `dockium-agent -panel ${endpoint} -token ${state.token}`
    return { docker, binary }
  }, [state])

  return (
    <Dialog
      open={Boolean(state)}
      onClose={onClose}
      title={state?.title}
      description="Токен показывается один раз — сохраните его в защищённом месте."
      size="md"
      footer={<Button variant="primary" onClick={onClose}>Готово</Button>}
    >
      {state ? (
        <div className={styles.tokenBody}>
          <InlineNotice tone="warning" title="Секрет" icon={AlertTriangle}>
            Передавайте токен только агенту. При утечке перевыпустите его в меню агента.
          </InlineNotice>
          <CopyField label="Токен агента" value={state.token} />
          {state.endpoint ? <CopyField label="Адрес панели для агентов" value={state.endpoint} /> : null}
          {commands ? (
            <>
              <CopyField label="Docker (после сборки образа install.sh)" value={commands.docker} />
              <CopyField label="Бинарный агент" value={commands.binary} />
            </>
          ) : null}
        </div>
      ) : null}
    </Dialog>
  )
}
