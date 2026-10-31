import { useState } from 'react'
import { useQueryClient } from '@tanstack/react-query'
import { useNavigate } from 'react-router-dom'
import { AlertTriangle, Settings2, Trash2, Zap } from 'lucide-react'
import { api, errorMessage } from '../../api/client'
import { keys, useAgentsQuery } from '../../data/queries'
import type { Instance } from '../../gen/panel/v1/panel_pb'
import {
  describePorts,
  formatBytes,
  formatDate,
  formatUptime,
  instanceStatusInfo,
  truncateMiddle,
} from '../../lib/format'
import { Badge, InlineNotice, KeyValue } from '../../ui/Display'
import { Button } from '../../ui/Button'
import { Panel } from '../../ui/Panel'
import { Switch } from '../../ui/Toggle'
import { Dialog } from '../../ui/Dialog'
import { useToast } from '../../ui/Toast'
import { InstanceFormDialog } from '../../features/instance/InstanceFormDialog'
import styles from './SettingsTab.module.css'

export function SettingsTab({ instance }: { instance: Instance }) {
  const toast = useToast()
  const queryClient = useQueryClient()
  const navigate = useNavigate()
  const agentsQuery = useAgentsQuery(true)
  const [editOpen, setEditOpen] = useState(false)
  const [deleteOpen, setDeleteOpen] = useState(false)
  const [removeData, setRemoveData] = useState(false)
  const [deleting, setDeleting] = useState(false)
  const [savingAutomation, setSavingAutomation] = useState(false)
  const [autoStart, setAutoStart] = useState(instance.autoStart)
  const [autoRestart, setAutoRestart] = useState(instance.autoRestart)

  const saveAutomation = async (nextAutoStart: boolean, nextAutoRestart: boolean) => {
    const previous = { autoStart, autoRestart }
    setAutoStart(nextAutoStart)
    setAutoRestart(nextAutoRestart)
    setSavingAutomation(true)
    try {
      await api.instances.setAutomation({
        id: instance.id,
        autoStart: nextAutoStart,
        autoRestart: nextAutoRestart,
      })
      toast.success('Автоматизация обновлена')
      await queryClient.invalidateQueries({ queryKey: keys.instances })
    } catch (error) {
      setAutoStart(previous.autoStart)
      setAutoRestart(previous.autoRestart)
      toast.error('Не удалось сохранить автоматизацию', errorMessage(error))
    } finally {
      setSavingAutomation(false)
    }
  }

  const remove = async () => {
    setDeleting(true)
    try {
      await api.instances.delete({ id: instance.id, removeData })
      toast.success('Экземпляр удалён', instance.name)
      await queryClient.invalidateQueries({ queryKey: keys.instances })
      await queryClient.invalidateQueries({ queryKey: keys.dashboard })
      navigate('/instances')
    } catch (error) {
      toast.error('Не удалось удалить экземпляр', errorMessage(error))
      setDeleting(false)
    }
  }

  const limits =
    instance.memoryMb > 0 || instance.cpus > 0
      ? [instance.memoryMb > 0 ? `${instance.memoryMb} МБ RAM` : '', instance.cpus > 0 ? `${instance.cpus} CPU` : '']
          .filter(Boolean)
          .join(' · ')
      : 'Без лимитов'
  const status = instanceStatusInfo(instance.status, instance.exitCode, instance.lastError)

  return (
    <div className={styles.grid}>
      <Panel
        title="Конфигурация"
        icon={<Settings2 size={14} />}
        action={
          <Button size="sm" variant="secondary" onClick={() => setEditOpen(true)}>
            Изменить
          </Button>
        }
      >
        <div className={styles.sections}>
          <div>
            <h3 className={styles.subhead}>Контейнер</h3>
            <KeyValue
              columns={2}
              items={[
                { label: 'Образ', value: <code className={styles.code}>{instance.image}</code> },
                { label: 'Контейнер', value: <code className={styles.code}>{instance.containerName || '—'}</code> },
                { label: 'ID контейнера', value: <code className={styles.code}>{instance.containerId ? truncateMiddle(instance.containerId, 24) : '—'}</code> },
                { label: 'Агент', value: instance.agentName },
                { label: 'Порты', value: <code className={styles.code}>{describePorts(instance.ports)}</code> },
                { label: 'Сеть', value: instance.networkMode || 'bridge' },
                { label: 'Политика Docker', value: instance.restartPolicy || 'no' },
                { label: 'Лимиты', value: limits },
                { label: 'Каталог данных', value: <code className={styles.code}>{instance.dataMount}</code> },
                { label: 'Кодировка консоли', value: instance.consoleEncoding },
                { label: 'TTY', value: instance.tty ? 'включён' : 'выключен' },
                { label: 'Команда', value: instance.command.length ? <code className={styles.code}>{instance.command.join(' ')}</code> : 'из образа' },
              ]}
            />
          </div>

          <div>
            <h3 className={styles.subhead}>Состояние</h3>
            <KeyValue
              columns={2}
              items={[
                { label: 'Статус', value: <Badge tone={status.tone} dot>{status.label}</Badge> },
                { label: 'Проработал', value: formatUptime(instance.lastStartedAt) },
                { label: 'Память', value: `${formatBytes(instance.usage?.memoryBytes ?? 0n)}${instance.usage?.memoryLimitBytes ? ` / ${formatBytes(instance.usage.memoryLimitBytes)}` : ''}` },
                { label: 'Последний запуск', value: formatDate(instance.lastStartedAt) },
                { label: 'Создан', value: formatDate(instance.createdAt) },
                { label: 'Обновлён', value: formatDate(instance.updatedAt) },
              ]}
            />
          </div>
        </div>
        {instance.lastError ? (
          <InlineNotice tone="warning" title="Последняя ошибка" icon={AlertTriangle}>
            {instance.lastError}
          </InlineNotice>
        ) : null}
      </Panel>

      <div className={styles.side}>
        <Panel title="Автоматизация" icon={<Zap size={14} />}>
          <div className={styles.switches}>
            <Switch
              checked={autoStart}
              onChange={(checked) => void saveAutomation(checked, autoRestart)}
              disabled={savingAutomation}
              label="Запускать при подключении агента"
              description="Экземпляр стартует после запуска агента или панели."
            />
            <Switch
              checked={autoRestart}
              onChange={(checked) => void saveAutomation(autoStart, checked)}
              disabled={savingAutomation}
              label="Перезапускать при падении"
              description="Перезапуск после аварийного выхода контейнера."
            />
          </div>
          <p className={styles.hint}>Изменения применяются сразу.</p>
        </Panel>

        <Panel title="Опасная зона" icon={<AlertTriangle size={14} />} className={styles.dangerPanel}>
          <p className={styles.dangerText}>
            Удаление экземпляра останавливает и удаляет контейнер на агенте. Каталог данных можно
            удалить вместе с экземпляром.
          </p>
          <Button
            variant="danger"
            icon={<Trash2 size={14} />}
            onClick={() => setDeleteOpen(true)}
          >
            Удалить экземпляр
          </Button>
        </Panel>
      </div>

      <InstanceFormDialog
        open={editOpen}
        onClose={() => setEditOpen(false)}
        instance={instance}
        agents={agentsQuery.data?.items ?? []}
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
              Контейнер будет удалён при подключении агента. Удаление данных сейчас недоступно.
            </InlineNotice>
          ) : null}
        </div>
      </Dialog>
    </div>
  )
}
