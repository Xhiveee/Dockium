import { useEffect, useMemo, useState } from 'react'
import { useQueryClient } from '@tanstack/react-query'
import { CalendarClock, Pencil, Plus, Trash2 } from 'lucide-react'
import { api, errorMessage } from '../../api/client'
import { keys, useTasksQuery } from '../../data/queries'
import type { Instance, ScheduledTask } from '../../gen/panel/v1/panel_pb'
import { ScheduleAction, ScheduleKind } from '../../gen/panel/v1/panel_pb'
import {
  formatDate,
  formatRelative,
  plural,
} from '../../lib/format'
import { Badge, EmptyState, InlineNotice, Skeleton } from '../../ui/Display'
import { Button } from '../../ui/Button'
import { Dialog } from '../../ui/Dialog'
import { Field, Input, Select } from '../../ui/Field'
import { Menu } from '../../ui/Menu'
import { Switch } from '../../ui/Toggle'
import { useToast } from '../../ui/Toast'
import styles from './TasksTab.module.css'

type TaskForm = {
  name: string
  action: string
  payload: string
  kind: string
  interval: string
  timeOfDay: string
  enabled: boolean
}

const emptyTask: TaskForm = {
  name: '',
  action: 'restart',
  payload: '',
  kind: 'interval',
  interval: '60',
  timeOfDay: '04:00',
  enabled: true,
}

function actionLabel(action: ScheduleAction): string {
  switch (action) {
    case ScheduleAction.START:
      return 'Запуск'
    case ScheduleAction.STOP:
      return 'Остановка'
    case ScheduleAction.RESTART:
      return 'Перезапуск'
    case ScheduleAction.COMMAND:
      return 'Команда'
    default:
      return '—'
  }
}

function actionValue(action: ScheduleAction): string {
  switch (action) {
    case ScheduleAction.START:
      return 'start'
    case ScheduleAction.STOP:
      return 'stop'
    case ScheduleAction.COMMAND:
      return 'command'
    case ScheduleAction.RESTART:
    default:
      return 'restart'
  }
}

function kindValue(kind: ScheduleKind): string {
  return kind === ScheduleKind.DAILY ? 'daily' : 'interval'
}

export function TasksTab({ instance }: { instance: Instance }) {
  const tasksQuery = useTasksQuery(instance.id.toString())
  const queryClient = useQueryClient()
  const toast = useToast()
  const [formOpen, setFormOpen] = useState(false)
  const [editing, setEditing] = useState<ScheduledTask | null>(null)
  const [form, setForm] = useState<TaskForm>(emptyTask)
  const [errors, setErrors] = useState<Record<string, string>>({})
  const [saving, setSaving] = useState(false)
  const [deleteTarget, setDeleteTarget] = useState<ScheduledTask | null>(null)
  const [deleting, setDeleting] = useState(false)

  const tasks = useMemo(() => tasksQuery.data?.items ?? [], [tasksQuery.data])

  useEffect(() => {
    if (!formOpen) return
    if (editing) {
      setForm({
        name: editing.name,
        action: actionValue(editing.action),
        payload: editing.payload,
        kind: kindValue(editing.kind),
        interval: String(editing.intervalMinutes || 60),
        timeOfDay: editing.timeOfDay || '04:00',
        enabled: editing.enabled,
      })
    } else {
      setForm(emptyTask)
    }
    setErrors({})
  }, [formOpen, editing])

  const refresh = async () => {
    await queryClient.invalidateQueries({ queryKey: keys.tasks(instance.id.toString()) })
    await queryClient.invalidateQueries({ queryKey: keys.dashboard })
  }

  const submit = async () => {
    const nextErrors: Record<string, string> = {}
    if (form.name.trim().length < 2) nextErrors.name = 'Не менее 2 символов'
    if (form.name.trim().length > 64) nextErrors.name = 'Не более 64 символов'
    if (form.kind === 'interval') {
      const minutes = Number(form.interval)
      if (!Number.isInteger(minutes) || minutes < 1 || minutes > 10080) {
        nextErrors.interval = 'От 1 до 10080 минут'
      }
    } else if (!/^([01]\d|2[0-3]):[0-5]\d$/.test(form.timeOfDay)) {
      nextErrors.time = 'Время в формате ЧЧ:ММ'
    }
    if (form.action === 'command' && !form.payload.trim()) {
      nextErrors.payload = 'Укажите команду'
    }
    setErrors(nextErrors)
    if (Object.keys(nextErrors).length) return
    setSaving(true)
    try {
      const payload = {
        name: form.name.trim(),
        action:
          form.action === 'start'
            ? ScheduleAction.START
            : form.action === 'stop'
              ? ScheduleAction.STOP
              : form.action === 'command'
                ? ScheduleAction.COMMAND
                : ScheduleAction.RESTART,
        payload: form.payload,
        kind: form.kind === 'daily' ? ScheduleKind.DAILY : ScheduleKind.INTERVAL,
        intervalMinutes: form.kind === 'interval' ? Number(form.interval) : 0,
        timeOfDay: form.kind === 'daily' ? form.timeOfDay : '',
        enabled: form.enabled,
      }
      if (editing) {
        await api.schedules.update({ ...payload, id: editing.id })
        toast.success('Задача обновлена', form.name.trim())
      } else {
        await api.schedules.create({ ...payload, instanceId: instance.id })
        toast.success('Задача создана', form.name.trim())
      }
      setFormOpen(false)
      await refresh()
    } catch (error) {
      toast.error('Не удалось сохранить задачу', errorMessage(error))
    } finally {
      setSaving(false)
    }
  }

  const toggle = async (task: ScheduledTask, enabled: boolean) => {
    try {
      await api.schedules.update({
        id: task.id,
        name: task.name,
        action: task.action,
        payload: task.payload,
        kind: task.kind,
        intervalMinutes: task.intervalMinutes,
        timeOfDay: task.timeOfDay,
        enabled,
      })
      await refresh()
    } catch (error) {
      toast.error('Не удалось изменить задачу', errorMessage(error))
    }
  }

  const remove = async () => {
    if (!deleteTarget) return
    setDeleting(true)
    try {
      await api.schedules.delete({ id: deleteTarget.id })
      toast.success('Задача удалена', deleteTarget.name)
      setDeleteTarget(null)
      await refresh()
    } catch (error) {
      toast.error('Не удалось удалить задачу', errorMessage(error))
    } finally {
      setDeleting(false)
    }
  }

  return (
    <div className={styles.wrap}>
      {tasksQuery.isError ? (
        <InlineNotice tone="danger" title="Не удалось загрузить задачи">
          {errorMessage(tasksQuery.error)}
        </InlineNotice>
      ) : null}
      <div className={styles.panel}>
        <div className={styles.toolbar}>
          <div className={styles.toolbarTitle}>
            <CalendarClock size={14} aria-hidden />
            <span>Расписания</span>
            <span className={styles.count}>{tasks.length}</span>
          </div>
          <Button
            size="sm"
            variant="secondary"
            icon={<Plus size={14} />}
            onClick={() => {
              setEditing(null)
              setFormOpen(true)
            }}
          >
            Новая задача
          </Button>
        </div>

        {tasksQuery.isPending ? (
          <div className={styles.rows}>
            {Array.from({ length: 3 }, (_, index) => (
              <div className={styles.row} key={index}>
                <Skeleton width="40%" height={12} />
                <Skeleton width={70} height={11} />
                <Skeleton width={100} height={11} />
                <Skeleton width={60} height={20} />
              </div>
            ))}
          </div>
        ) : tasks.length === 0 ? (
          <EmptyState
            icon={CalendarClock}
            title="Задач по расписанию нет"
            description="Настройте автоматический запуск, остановку или перезапуск в заданное время."
            action={<Button variant="primary" onClick={() => setFormOpen(true)}>Создать задачу</Button>}
          />
        ) : (
          <div className={styles.rows}>
            <div className={styles.rowsHead}>
              <span>Задача</span>
              <span>Действие</span>
              <span>Расписание</span>
              <span>Следующий запуск</span>
              <span>Вкл.</span>
              <span />
            </div>
            {tasks.map((task) => (
              <div className={styles.row} key={task.id.toString()}>
                <div className={styles.nameCell}>
                  <span className={styles.name}>{task.name}</span>
                  {task.lastError ? <span className={styles.error}>{task.lastError}</span> : null}
                  {task.lastRunAt ? (
                    <span className={styles.meta}>запуск {formatRelative(task.lastRunAt)}</span>
                  ) : null}
                </div>
                <div>
                  <Badge tone={task.action === ScheduleAction.STOP ? 'warning' : 'neutral'}>
                    {actionLabel(task.action)}
                  </Badge>
                </div>
                <span className={styles.schedule}>
                  {task.kind === ScheduleKind.DAILY
                    ? `Ежедневно в ${task.timeOfDay}`
                    : `Каждые ${task.intervalMinutes} ${plural(task.intervalMinutes, ['минуту', 'минуты', 'минут'])}`}
                  {task.action === ScheduleAction.COMMAND && task.payload ? (
                    <span className={styles.command} title={task.payload}>· {task.payload}</span>
                  ) : null}
                </span>
                <span className={styles.next}>{task.nextRunAt ? formatDate(task.nextRunAt, true) : '—'}</span>
                <div className={styles.toggle}>
                  <Switch
                    compact
                    checked={task.enabled}
                    onChange={(checked) => void toggle(task, checked)}
                    label=""
                    ariaLabel={`Задача «${task.name}» включена`}
                    disabled={!instance.permissions?.control}
                  />
                </div>
                <div className={styles.rowActions}>
                  <Menu
                    label={`Действия: ${task.name}`}
                    items={[
                      {
                        id: 'edit',
                        label: 'Изменить',
                        icon: <Pencil size={14} />,
                        onSelect: () => {
                          setEditing(task)
                          setFormOpen(true)
                        },
                      },
                      {
                        id: 'delete',
                        label: 'Удалить',
                        icon: <Trash2 size={14} />,
                        danger: true,
                        separator: true,
                        onSelect: () => setDeleteTarget(task),
                      },
                    ]}
                  />
                </div>
              </div>
            ))}
          </div>
        )}
        <div className={styles.footer}>
          Время расписания считается по часовому поясу панели.
        </div>
      </div>

      <Dialog
        open={formOpen}
        onClose={() => setFormOpen(false)}
        title={editing ? 'Изменить задачу' : 'Новая задача'}
        size="md"
        dismissible={!saving}
        footer={
          <>
            <Button variant="ghost" onClick={() => setFormOpen(false)} disabled={saving}>Отмена</Button>
            <Button variant="primary" onClick={() => void submit()} loading={saving}>
              {editing ? 'Сохранить' : 'Создать'}
            </Button>
          </>
        }
      >
        <div className={styles.form}>
          <Field label="Название" required error={errors.name} htmlFor="task-name">
            <Input
              id="task-name"
              value={form.name}
              onChange={(event) => setForm({ ...form, name: event.target.value })}
              placeholder="Ночной перезапуск"
              maxLength={64}
              autoFocus
              invalid={Boolean(errors.name)}
            />
          </Field>
          <Field label="Действие" htmlFor="task-action">
            <Select
              id="task-action"
              value={form.action}
              onChange={(event) => setForm({ ...form, action: event.target.value })}
            >
              <option value="start">Запустить</option>
              <option value="stop">Остановить</option>
              <option value="restart">Перезапустить</option>
              <option value="command">Отправить команду</option>
            </Select>
          </Field>
          {form.action === 'command' ? (
            <Field
              label="Команда"
              required
              error={errors.payload}
              htmlFor="task-payload"
              hint="Отправляется в контейнер через Docker attach"
            >
              <Input
                id="task-payload"
                value={form.payload}
                onChange={(event) => setForm({ ...form, payload: event.target.value })}
                placeholder="say Сервер перезапустится через 5 минут"
                mono
                invalid={Boolean(errors.payload)}
              />
            </Field>
          ) : null}
          <Field label="Тип расписания" htmlFor="task-kind">
            <Select
              id="task-kind"
              value={form.kind}
              onChange={(event) => setForm({ ...form, kind: event.target.value })}
            >
              <option value="interval">Интервал</option>
              <option value="daily">Ежедневно</option>
            </Select>
          </Field>
          {form.kind === 'interval' ? (
            <Field label="Интервал, минут" error={errors.interval} htmlFor="task-interval">
              <Input
                id="task-interval"
                value={form.interval}
                onChange={(event) => setForm({ ...form, interval: event.target.value.replace(/[^0-9]/g, '') })}
                inputMode="numeric"
                mono
                invalid={Boolean(errors.interval)}
              />
            </Field>
          ) : (
            <Field label="Время" error={errors.time} htmlFor="task-time" hint="ЧЧ:ММ по часовому поясу панели">
              <Input
                id="task-time"
                value={form.timeOfDay}
                onChange={(event) => setForm({ ...form, timeOfDay: event.target.value })}
                placeholder="04:00"
                mono
                maxLength={5}
                invalid={Boolean(errors.time)}
              />
            </Field>
          )}
          <Switch
            checked={form.enabled}
            onChange={(checked) => setForm({ ...form, enabled: checked })}
            label="Задача включена"
            description="Выключенную задачу можно сохранить, чтобы запустить позже."
          />
        </div>
      </Dialog>

      <Dialog
        open={Boolean(deleteTarget)}
        onClose={() => setDeleteTarget(null)}
        title="Удалить задачу?"
        description={deleteTarget?.name}
        size="sm"
        dismissible={!deleting}
        footer={
          <>
            <Button variant="ghost" onClick={() => setDeleteTarget(null)} disabled={deleting}>Отмена</Button>
            <Button variant="dangerSolid" onClick={() => void remove()} loading={deleting}>Удалить</Button>
          </>
        }
      >
        <p className={styles.dialogText}>Задача будет удалена из расписания экземпляра.</p>
      </Dialog>
    </div>
  )
}

