import { useEffect, useMemo, useState } from 'react'
import { Plus, Trash2 } from 'lucide-react'
import { api, errorMessage } from '../../api/client'
import { useQueryClient } from '@tanstack/react-query'
import type { Agent, Instance } from '../../gen/panel/v1/panel_pb'
import { InstanceStatus } from '../../gen/panel/v1/panel_pb'
import { keys } from '../../data/queries'
import { joinCommandLine, splitCommandLine } from '../../lib/format'
import { cx } from '../../lib/cx'
import { Button } from '../../ui/Button'
import { Dialog } from '../../ui/Dialog'
import { Field, Input, Select } from '../../ui/Field'
import { InlineNotice } from '../../ui/Display'
import { Switch } from '../../ui/Toggle'
import { useToast } from '../../ui/Toast'
import styles from './InstanceFormDialog.module.css'

type PortRow = { host: string; container: string; protocol: string }
type EnvRow = { key: string; value: string }
type VolumeRow = { host: string; container: string; readOnly: boolean }

type FormState = {
  name: string
  agentId: string
  image: string
  containerName: string
  command: string
  networkMode: string
  restartPolicy: string
  memoryMb: string
  cpus: string
  tty: boolean
  dataMount: string
  encoding: string
  autoStart: boolean
  autoRestart: boolean
  ports: PortRow[]
  environment: EnvRow[]
  volumes: VolumeRow[]
}

const imageSuggestions = [
  'nginx:alpine',
  'redis:7-alpine',
  'postgres:16-alpine',
  'eclipse-temurin:21-jre',
  'itzg/minecraft-server',
  'ghcr.io/pterodactyl/yolks:java_21',
]

function emptyForm(agents: Agent[]): FormState {
  const firstOnline = agents.find((agent) => agent.online) ?? agents[0]
  return {
    name: '',
    agentId: firstOnline ? firstOnline.id.toString() : '',
    image: '',
    containerName: '',
    command: '',
    networkMode: 'bridge',
    restartPolicy: 'no',
    memoryMb: '',
    cpus: '',
    tty: true,
    dataMount: '/data',
    encoding: 'utf-8',
    autoStart: false,
    autoRestart: true,
    ports: [],
    environment: [],
    volumes: [],
  }
}

function formFromInstance(instance: Instance): FormState {
  return {
    name: instance.name,
    agentId: instance.agentId.toString(),
    image: instance.image,
    containerName: instance.containerName,
    command: joinCommandLine(instance.command),
    networkMode: instance.networkMode || 'bridge',
    restartPolicy: instance.restartPolicy || 'no',
    memoryMb: instance.memoryMb ? String(instance.memoryMb) : '',
    cpus: instance.cpus ? String(instance.cpus) : '',
    tty: instance.tty,
    dataMount: instance.dataMount || '/data',
    encoding: instance.consoleEncoding || 'utf-8',
    autoStart: instance.autoStart,
    autoRestart: instance.autoRestart,
    ports: instance.ports.map((port) => ({
      host: String(port.hostPort),
      container: String(port.containerPort),
      protocol: port.protocol || 'tcp',
    })),
    environment: instance.environment.map((item) => ({ key: item.key, value: item.value })),
    volumes: instance.volumes.map((volume) => ({
      host: volume.hostPath,
      container: volume.containerPath,
      readOnly: volume.readOnly,
    })),
  }
}

export function InstanceFormDialog({
  open,
  onClose,
  instance,
  agents,
  onCreated,
}: {
  open: boolean
  onClose: () => void
  instance?: Instance
  agents: Agent[]
  onCreated?: (instance: Instance) => void
}) {
  const toast = useToast()
  const queryClient = useQueryClient()
  const [form, setForm] = useState<FormState>(() => emptyForm(agents))
  const [errors, setErrors] = useState<Record<string, string>>({})
  const [generalError, setGeneralError] = useState('')
  const [busy, setBusy] = useState(false)
  const editing = Boolean(instance)

  useEffect(() => {
    if (!open) return
    setForm(instance ? formFromInstance(instance) : emptyForm(agents))
    setErrors({})
    setGeneralError('')
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [open, instance])

  useEffect(() => {
    if (!open || editing) return
    setForm((current) => {
      if (current.agentId) return current
      const preferred = agents.find((agent) => agent.online) ?? agents[0]
      return preferred ? { ...current, agentId: preferred.id.toString() } : current
    })
  }, [agents, open, editing])

  const wasRunning = useMemo(
    () =>
      Boolean(
        instance &&
          (instance.status === InstanceStatus.RUNNING ||
            instance.status === InstanceStatus.STARTING ||
            instance.status === InstanceStatus.STOPPING),
      ),
    [instance],
  )

  const update = <K extends keyof FormState>(key: K, value: FormState[K]) => {
    setForm((current) => ({ ...current, [key]: value }))
  }

  const submit = async () => {
    const nextErrors: Record<string, string> = {}
    if (form.name.trim().length < 2) nextErrors.name = 'Не менее 2 символов'
    if (form.name.trim().length > 48) nextErrors.name = 'Не более 48 символов'
    if (!form.agentId && !editing) nextErrors.agentId = 'Выберите агента'
    if (!form.image.trim()) nextErrors.image = 'Укажите Docker-образ'
    if (form.dataMount && !form.dataMount.startsWith('/')) {
      nextErrors.dataMount = 'Путь в контейнере должен начинаться с /'
    }
    for (const port of form.ports) {
      if (!port.host || !port.container) continue
      const host = Number(port.host)
      const container = Number(port.container)
      if (!Number.isInteger(host) || host < 1 || host > 65535) {
        nextErrors.ports = 'Порты должны быть от 1 до 65535'
      }
      if (!Number.isInteger(container) || container < 1 || container > 65535) {
        nextErrors.ports = 'Порты должны быть от 1 до 65535'
      }
    }
    for (const item of form.environment) {
      if (item.key && !/^[A-Za-z_][A-Za-z0-9_]*$/.test(item.key)) {
        nextErrors.env = `Некорректное имя переменной: ${item.key}`
      }
    }
    setErrors(nextErrors)
    if (Object.keys(nextErrors).length) return
    setBusy(true)
    setGeneralError('')
    try {
      const ports = form.ports
        .filter((port) => port.host && port.container)
        .map((port) => ({
          hostPort: Number(port.host),
          containerPort: Number(port.container),
          protocol: port.protocol || 'tcp',
        }))
      const environment = form.environment
        .filter((item) => item.key.trim())
        .map((item) => ({ key: item.key.trim(), value: item.value }))
      const volumes = form.volumes
        .filter((volume) => volume.host.trim() && volume.container.trim())
        .map((volume) => ({
          hostPath: volume.host.trim(),
          containerPath: volume.container.trim(),
          readOnly: volume.readOnly,
        }))
      const command = splitCommandLine(form.command)
      const payload = {
        name: form.name.trim(),
        image: form.image.trim(),
        containerName: form.containerName.trim(),
        command,
        environment,
        ports,
        volumes,
        networkMode: form.networkMode,
        restartPolicy: form.restartPolicy,
        memoryMb: form.memoryMb ? Number(form.memoryMb) : 0,
        cpus: form.cpus ? Number(form.cpus) : 0,
        tty: form.tty,
        dataMount: form.dataMount.trim() || '/data',
        consoleEncoding: form.encoding,
        autoStart: form.autoStart,
        autoRestart: form.autoRestart,
      }
      if (editing && instance) {
        const updated = await api.instances.update({ ...payload, id: instance.id })
        toast.success('Настройки сохранены', wasRunning ? 'Контейнер будет перезапущен с новой конфигурацией.' : undefined)
        await queryClient.invalidateQueries({ queryKey: keys.instances })
        await queryClient.invalidateQueries({ queryKey: keys.dashboard })
        onCreated?.(updated)
        onClose()
      } else {
        const created = await api.instances.create({ ...payload, agentId: BigInt(form.agentId) })
        toast.success('Экземпляр создан', 'Подготовка контейнера уже началась.')
        await queryClient.invalidateQueries({ queryKey: keys.instances })
        await queryClient.invalidateQueries({ queryKey: keys.dashboard })
        onCreated?.(created)
        onClose()
      }
    } catch (error) {
      const message = errorMessage(error)
      setGeneralError(message)
      toast.error(editing ? 'Не удалось сохранить' : 'Не удалось создать экземпляр', message)
    } finally {
      setBusy(false)
    }
  }

  const title = editing ? `Настройки «${instance?.name}»` : 'Новый экземпляр'
  const description = editing
    ? 'После сохранения контейнер будет пересоздан из нового образа и конфигурации.'
    : 'Экземпляр — это Docker-контейнер на агенте, его данные хранятся в отдельном каталоге.'

  return (
    <Dialog
      open={open}
      onClose={busy ? () => undefined : onClose}
      title={title}
      description={description}
      size="lg"
      dismissible={!busy}
      footer={
        <>
          <Button variant="ghost" onClick={onClose} disabled={busy}>Отмена</Button>
          <Button variant="primary" onClick={submit} loading={busy}>
            {editing ? 'Сохранить' : 'Создать экземпляр'}
          </Button>
        </>
      }
    >
      <div className={styles.form}>
        {wasRunning && editing ? (
          <InlineNotice tone="warning" title="Экземпляр работает">
            Сохранение остановит контейнер, пересоздаст его с новой конфигурацией и запустит снова.
          </InlineNotice>
        ) : null}
        {generalError ? <InlineNotice tone="danger" title={generalError} /> : null}

        <section className={styles.section}>
          <h3 className={styles.sectionTitle}>Основное</h3>
          <div className={styles.grid}>
            <Field label="Название" required error={errors.name} htmlFor="instance-name">
              <Input
                id="instance-name"
                value={form.name}
                onChange={(event) => update('name', event.target.value)}
                placeholder="Мой сервер"
                maxLength={48}
                invalid={Boolean(errors.name)}
                autoFocus
              />
            </Field>
            <Field
              label="Образ"
              required
              error={errors.image}
              htmlFor="instance-image"
              hint="Можно указать тег: nginx:alpine"
            >
              <Input
                id="instance-image"
                value={form.image}
                onChange={(event) => update('image', event.target.value)}
                placeholder="nginx:alpine"
                list="dockium-image-suggestions"
                mono
                invalid={Boolean(errors.image)}
              />
              <datalist id="dockium-image-suggestions">
                {imageSuggestions.map((image) => <option key={image} value={image} />)}
              </datalist>
            </Field>
            {editing ? (
              <Field label="Агент" htmlFor="instance-agent-readonly">
                <Input id="instance-agent-readonly" value={instance?.agentName ?? ''} readOnly disabled />
              </Field>
            ) : (
              <Field label="Агент" required error={errors.agentId} htmlFor="instance-agent">
                <Select
                  id="instance-agent"
                  value={form.agentId}
                  onChange={(event) => update('agentId', event.target.value)}
                  invalid={Boolean(errors.agentId)}
                >
                  <option value="">Выберите агента</option>
                  {agents.map((agent) => (
                    <option key={agent.id.toString()} value={agent.id.toString()}>
                      {agent.name}{agent.online ? '' : ' — офлайн'}
                    </option>
                  ))}
                </Select>
              </Field>
            )}
            <Field
              label="Имя контейнера"
              htmlFor="instance-container"
              hint="Оставьте пустым — Dockium сгенерирует имя."
            >
              <Input
                id="instance-container"
                value={form.containerName}
                onChange={(event) => update('containerName', event.target.value)}
                placeholder={editing ? instance?.containerName : 'автоматически'}
                mono
                maxLength={63}
              />
            </Field>
            <Field
              label="Команда"
              htmlFor="instance-command"
              hint="Переопределяет CMD образа. Кавычки поддерживаются."
              className={styles.full}
            >
              <Input
                id="instance-command"
                value={form.command}
                onChange={(event) => update('command', event.target.value)}
                placeholder="java -Xmx2G -jar server.jar nogui"
                mono
              />
            </Field>
          </div>
        </section>

        <section className={styles.section}>
          <div className={styles.sectionHead}>
            <h3 className={styles.sectionTitle}>Порты</h3>
            <Button size="sm" variant="ghost" icon={<Plus size={13} />} onClick={() => update('ports', [...form.ports, { host: '', container: '', protocol: 'tcp' }])}>
              Добавить порт
            </Button>
          </div>
          {form.ports.length ? (
            <div className={styles.array}>
              <div className={cx(styles.arrayRow, styles.arrayHeader)}>
                <span>Порт агента</span>
                <span>Порт контейнера</span>
                <span>Протокол</span>
                <span />
              </div>
              {form.ports.map((port, index) => (
                <div className={styles.arrayRow} key={index}>
                  <Input
                    aria-label={`Порт агента ${index + 1}`}
                    value={port.host}
                    inputMode="numeric"
                    placeholder="8080"
                    mono
                    onChange={(event) => {
                      const next = [...form.ports]
                      next[index] = { ...port, host: event.target.value }
                      update('ports', next)
                    }}
                  />
                  <Input
                    aria-label={`Порт контейнера ${index + 1}`}
                    value={port.container}
                    inputMode="numeric"
                    placeholder="80"
                    mono
                    onChange={(event) => {
                      const next = [...form.ports]
                      next[index] = { ...port, container: event.target.value }
                      update('ports', next)
                    }}
                  />
                  <Select
                    aria-label={`Протокол ${index + 1}`}
                    value={port.protocol}
                    onChange={(event) => {
                      const next = [...form.ports]
                      next[index] = { ...port, protocol: event.target.value }
                      update('ports', next)
                    }}
                  >
                    <option value="tcp">TCP</option>
                    <option value="udp">UDP</option>
                  </Select>
                  <Button
                    size="sm"
                    variant="ghost"
                    iconOnly
                    aria-label="Удалить порт"
                    icon={<Trash2 size={14} />}
                    onClick={() => update('ports', form.ports.filter((_, position) => position !== index))}
                  />
                </div>
              ))}
            </div>
          ) : (
            <p className={styles.emptyHint}>Порты не проброшены. Добавьте, если контейнер должен быть доступен снаружи.</p>
          )}
          {errors.ports ? <p className={styles.error}>{errors.ports}</p> : null}
        </section>

        <section className={styles.section}>
          <div className={styles.sectionHead}>
            <h3 className={styles.sectionTitle}>Переменные окружения</h3>
            <Button size="sm" variant="ghost" icon={<Plus size={13} />} onClick={() => update('environment', [...form.environment, { key: '', value: '' }])}>
              Добавить переменную
            </Button>
          </div>
          {form.environment.length ? (
            <div className={styles.array}>
              {form.environment.map((item, index) => (
                <div className={cx(styles.arrayRow, styles.envRow)} key={index}>
                  <Input
                    aria-label={`Имя переменной ${index + 1}`}
                    value={item.key}
                    placeholder="JAVA_OPTS"
                    mono
                    onChange={(event) => {
                      const next = [...form.environment]
                      next[index] = { ...item, key: event.target.value }
                      update('environment', next)
                    }}
                  />
                  <Input
                    aria-label={`Значение переменной ${index + 1}`}
                    value={item.value}
                    placeholder="-Xmx2G"
                    mono
                    onChange={(event) => {
                      const next = [...form.environment]
                      next[index] = { ...item, value: event.target.value }
                      update('environment', next)
                    }}
                  />
                  <Button
                    size="sm"
                    variant="ghost"
                    iconOnly
                    aria-label="Удалить переменную"
                    icon={<Trash2 size={14} />}
                    onClick={() => update('environment', form.environment.filter((_, position) => position !== index))}
                  />
                </div>
              ))}
            </div>
          ) : (
            <p className={styles.emptyHint}>Переменные не заданы — используются значения образа.</p>
          )}
          {errors.env ? <p className={styles.error}>{errors.env}</p> : null}
        </section>

        <section className={styles.section}>
          <div className={styles.sectionHead}>
            <h3 className={styles.sectionTitle}>Хранение и лимиты</h3>
            <Button size="sm" variant="ghost" icon={<Plus size={13} />} onClick={() => update('volumes', [...form.volumes, { host: '', container: '', readOnly: false }])}>
              Добавить том
            </Button>
          </div>
          <div className={styles.grid}>
            <Field
              label="Каталог данных в контейнере"
              htmlFor="instance-datamount"
              error={errors.dataMount}
              hint="Сюда монтируется каталог экземпляра на агенте."
            >
              <Input
                id="instance-datamount"
                value={form.dataMount}
                onChange={(event) => update('dataMount', event.target.value)}
                mono
                placeholder="/data"
                invalid={Boolean(errors.dataMount)}
              />
            </Field>
            <Field label="Сеть" htmlFor="instance-network">
              <Select id="instance-network" value={form.networkMode} onChange={(event) => update('networkMode', event.target.value)}>
                <option value="bridge">bridge</option>
                <option value="host">host</option>
                <option value="none">none</option>
              </Select>
            </Field>
            <Field label="Политика перезапуска Docker" htmlFor="instance-restart">
              <Select id="instance-restart" value={form.restartPolicy} onChange={(event) => update('restartPolicy', event.target.value)}>
                <option value="no">Не перезапускать</option>
                <option value="unless-stopped">Пока не остановлен</option>
                <option value="always">Всегда</option>
                <option value="on-failure">При ошибке</option>
              </Select>
            </Field>
            <Field label="Память, МБ" htmlFor="instance-memory" hint="0 — без лимита">
              <Input
                id="instance-memory"
                value={form.memoryMb}
                inputMode="numeric"
                placeholder="0"
                mono
                onChange={(event) => update('memoryMb', event.target.value.replace(/[^0-9]/g, ''))}
              />
            </Field>
            <Field label="Лимит CPU" htmlFor="instance-cpus" hint="Например, 1.5. 0 — без лимита">
              <Input
                id="instance-cpus"
                value={form.cpus}
                inputMode="decimal"
                placeholder="0"
                mono
                onChange={(event) => update('cpus', event.target.value.replace(/[^0-9.]/g, ''))}
              />
            </Field>
            <Field label="Кодировка консоли" htmlFor="instance-encoding">
              <Select id="instance-encoding" value={form.encoding} onChange={(event) => update('encoding', event.target.value)}>
                <option value="utf-8">UTF-8</option>
                <option value="windows-1251">Windows-1251</option>
                <option value="cp866">CP866</option>
                <option value="koi8-r">KOI8-R</option>
              </Select>
            </Field>
            <div className={styles.full}>
              <Switch
                checked={form.tty}
                onChange={(checked) => update('tty', checked)}
                label="TTY"
                description="Нужен приложениям, которые требуют терминал. Ввод команд работает и без TTY."
              />
            </div>
          </div>
          {form.volumes.length ? (
            <div className={styles.array}>
              <div className={cx(styles.arrayRow, styles.volumeRow, styles.arrayHeader)}>
                <span>Путь на агенте</span>
                <span>Путь в контейнере</span>
                <span>Только чтение</span>
                <span />
              </div>
              {form.volumes.map((volume, index) => (
                <div className={cx(styles.arrayRow, styles.volumeRow)} key={index}>
                  <Input
                    aria-label={`Путь на агенте ${index + 1}`}
                    value={volume.host}
                    onChange={(event) => {
                      const next = [...form.volumes]
                      next[index] = { ...volume, host: event.target.value }
                      update('volumes', next)
                    }}
                    placeholder="/srv/data"
                    mono
                  />
                  <Input
                    aria-label={`Путь в контейнере ${index + 1}`}
                    value={volume.container}
                    onChange={(event) => {
                      const next = [...form.volumes]
                      next[index] = { ...volume, container: event.target.value }
                      update('volumes', next)
                    }}
                    placeholder="/mnt/data"
                    mono
                  />
                  <Switch
                    compact
                    checked={volume.readOnly}
                    onChange={(checked) => {
                      const next = [...form.volumes]
                      next[index] = { ...volume, readOnly: checked }
                      update('volumes', next)
                    }}
                    label="RO"
                  />
                  <Button
                    size="sm"
                    variant="ghost"
                    iconOnly
                    aria-label="Удалить том"
                    icon={<Trash2 size={14} />}
                    onClick={() => update('volumes', form.volumes.filter((_, position) => position !== index))}
                  />
                </div>
              ))}
            </div>
          ) : null}
        </section>

        <section className={styles.section}>
          <h3 className={styles.sectionTitle}>Автоматизация</h3>
          <div className={styles.switches}>
            <Switch
              checked={form.autoStart}
              onChange={(checked) => update('autoStart', checked)}
              label="Запускать при подключении агента"
              description="Экземпляр стартует автоматически после запуска агента или панели."
            />
            <Switch
              checked={form.autoRestart}
              onChange={(checked) => update('autoRestart', checked)}
              label="Перезапускать при аварийном завершении"
              description="Dockium перезапустит контейнер, если он завершился не по команде пользователя."
            />
          </div>
        </section>

        {!editing ? (
          <p className={styles.footnote}>
            Создание экземпляра не скачивает образ мгновенно: первый старт может занять время,
            пока агент загружает образ.
          </p>
        ) : null}
      </div>
    </Dialog>
  )
}
