import { useEffect, useMemo, useRef, useState } from 'react'
import { Link } from 'react-router-dom'
import {
  AlertTriangle,
  Boxes,
  Copy,
  Hammer,
  Info,
  Layers,
  RefreshCw,
  Trash2,
  UploadCloud,
} from 'lucide-react'
import { api, errorMessage } from '../api/client'
import { streamBuild, streamPull } from '../api/streams'
import { useAgentsQuery, useContainersQuery, useImagesQuery } from '../data/queries'
import type { Agent, BuildOutput, DockerImage } from '../gen/panel/v1/panel_pb'
import { ansiSegments } from '../lib/ansi'
import { formatBytes, formatDate, truncateMiddle, copyText } from '../lib/format'
import { cx } from '../lib/cx'
import {
  Badge,
  EmptyState,
  InlineNotice,
  PageHeader,
  Skeleton,
} from '../ui/Display'
import { Button } from '../ui/Button'
import { Dialog, Drawer } from '../ui/Dialog'
import { Field, Input, Select, Textarea } from '../ui/Field'
import { Menu } from '../ui/Menu'
import { Tabs } from '../ui/Tabs'
import { useToast } from '../ui/Toast'
import { useQueryClient } from '@tanstack/react-query'
import styles from './ImagesPage.module.css'

type View = 'images' | 'containers'
type StreamMode = 'pull' | 'build' | null

const DEFAULT_DOCKERFILE = `FROM alpine:3.20
RUN apk add --no-cache curl
`

export function ImagesPage() {
  const agentsQuery = useAgentsQuery(true)
  const agents = agentsQuery.data?.items ?? []
  const [agentId, setAgentId] = useState('')
  const [view, setView] = useState<View>('images')
  const [streamMode, setStreamMode] = useState<StreamMode>(null)
  const [image, setImage] = useState('')
  const [tag, setTag] = useState('dockium/custom:latest')
  const [dockerfile, setDockerfile] = useState(DEFAULT_DOCKERFILE)
  const [logs, setLogs] = useState<string[]>([])
  const [streaming, setStreaming] = useState(false)
  const [streamError, setStreamError] = useState('')
  const [inspect, setInspect] = useState<{ title: string; json: string } | null>(null)
  const [inspectLoading, setInspectLoading] = useState(false)
  const [removeTarget, setRemoveTarget] = useState<DockerImage | null>(null)
  const [working, setWorking] = useState(false)
  const [tab, setTab] = useState('all')
  const controllerRef = useRef<AbortController | null>(null)
  const logRef = useRef<HTMLDivElement | null>(null)
  const queryClient = useQueryClient()
  const toast = useToast()

  useEffect(() => {
    if (!agents.length) {
      setAgentId('')
      return
    }
    if (!agentId || !agents.some((agent) => agent.id.toString() === agentId)) {
      const preferred = agents.find((agent) => agent.online) ?? agents[0]
      setAgentId(preferred ? preferred.id.toString() : '')
    }
  }, [agents, agentId])

  const selectedAgent: Agent | undefined = agents.find((agent) => agent.id.toString() === agentId)
  const imagesQuery = useImagesQuery(agentId || undefined)
  const containersQuery = useContainersQuery(agentId || undefined)
  const images = imagesQuery.data?.items ?? []
  const containers = containersQuery.data?.items ?? []

  useEffect(() => {
    if (!logRef.current) return
    logRef.current.scrollTop = logRef.current.scrollHeight
  }, [logs])

  const filteredImages = useMemo(() => {
    if (tab === 'all') return images
    if (tab === 'dangling') return images.filter((item) => item.repository === '<none>' || item.tag === '<none>')
    return images.filter((item) => item.repository !== '<none>')
  }, [images, tab])

  const refresh = async () => {
    await queryClient.invalidateQueries({ queryKey: ['images', agentId] })
    await queryClient.invalidateQueries({ queryKey: ['containers', agentId] })
  }

  const openStream = (mode: StreamMode) => {
    setStreamMode(mode)
    setLogs([])
    setStreamError('')
    setStreaming(false)
    if (mode === 'pull') setImage('')
    if (mode === 'build') {
      setTag('dockium/custom:latest')
      setDockerfile(DEFAULT_DOCKERFILE)
    }
  }

  const closeStream = () => {
    controllerRef.current?.abort()
    controllerRef.current = null
    setStreamMode(null)
    setStreaming(false)
  }

  const startStream = async () => {
    if (!streamMode || !agentId) return
    const controller = new AbortController()
    controllerRef.current = controller
    setStreaming(true)
    setStreamError('')
    setLogs([])
    const append = (output: BuildOutput) => {
      if (output.line) {
        setLogs((current) => {
          const next = [...current, output.line]
          return next.length > 2000 ? next.slice(next.length - 2000) : next
        })
      }
      if (output.error) setStreamError(output.error)
      if (output.done && !output.error) {
        toast.success(streamMode === 'pull' ? 'Образ загружен' : 'Образ собран', streamMode === 'pull' ? image : tag)
      }
    }
    try {
      if (streamMode === 'pull') {
        if (!image.trim()) throw new Error('Укажите имя образа')
        await streamPull(BigInt(agentId), image.trim(), append, controller.signal)
      } else {
        if (!tag.trim()) throw new Error('Укажите тег образа')
        if (!dockerfile.trim()) throw new Error('Dockerfile пуст')
        await streamBuild(BigInt(agentId), tag.trim(), dockerfile, append, controller.signal)
      }
      await refresh()
    } catch (error) {
      if (!(error instanceof Error && error.name === 'AbortError')) {
        const message = errorMessage(error)
        setStreamError(message)
        toast.error(streamMode === 'pull' ? 'Не удалось загрузить образ' : 'Не удалось собрать образ', message)
      }
    } finally {
      setStreaming(false)
      controllerRef.current = null
    }
  }

  const inspectObject = async (kind: 'image' | 'container', id: string, title: string) => {
    if (!agentId) return
    setInspectLoading(true)
    try {
      const response = await api.docker.inspect({ agentId: BigInt(agentId), kind, id })
      setInspect({ title, json: response.json })
    } catch (error) {
      toast.error('Не удалось получить информацию', errorMessage(error))
    } finally {
      setInspectLoading(false)
    }
  }

  const removeImage = async () => {
    if (!removeTarget || !agentId) return
    setWorking(true)
    try {
      await api.docker.removeImage({ agentId: BigInt(agentId), image: removeTarget.id })
      toast.success('Образ удалён', `${removeTarget.repository}:${removeTarget.tag}`)
      setRemoveTarget(null)
      await refresh()
    } catch (error) {
      toast.error('Не удалось удалить образ', errorMessage(error))
    } finally {
      setWorking(false)
    }
  }

  const dockerUnavailable = selectedAgent && selectedAgent.online && !selectedAgent.dockerOk
  const agentOffline = selectedAgent && !selectedAgent.online

  return (
    <div className={styles.page}>
      <PageHeader
        title="Образы"
        actions={
          <>
            <div className={styles.agentSelect}>
              <Select
                aria-label="Агент"
                value={agentId}
                onChange={(event) => setAgentId(event.target.value)}
                disabled={!agents.length}
              >
                {agents.length === 0 ? <option value="">Нет агентов</option> : null}
                {agents.map((agent) => (
                  <option key={agent.id.toString()} value={agent.id.toString()}>
                    {agent.name}{agent.online ? '' : ' — офлайн'}
                  </option>
                ))}
              </Select>
            </div>
            <Button
              variant="ghost"
              size="md"
              iconOnly
              aria-label="Обновить"
              icon={<RefreshCw size={15} />}
              onClick={() => void refresh()}
            />
            <Button variant="secondary" icon={<UploadCloud size={14} />} onClick={() => openStream('pull')} disabled={!agentId || Boolean(agentOffline) || Boolean(dockerUnavailable)}>
              Загрузить
            </Button>
            <Button variant="primary" icon={<Hammer size={14} />} onClick={() => openStream('build')} disabled={!agentId || Boolean(agentOffline) || Boolean(dockerUnavailable)}>
              Собрать
            </Button>
          </>
        }
      />

      {agentsQuery.isPending ? null : agents.length === 0 ? (
        <div className={styles.emptyPanel}>
          <EmptyState
            icon={Layers}
            title="Нет агентов"
            description="Образы и контейнеры принадлежат Docker-хосту. Сначала подключите агента."
            action={<Link to="/agents"><Button variant="primary">К агентам</Button></Link>}
          />
        </div>
      ) : dockerUnavailable ? (
        <InlineNotice tone="warning" title="Docker недоступен на агенте" icon={AlertTriangle}>
          {selectedAgent?.dockerError || 'Проверьте, что Docker запущен и доступен агенту.'}
        </InlineNotice>
      ) : agentOffline ? (
        <InlineNotice tone="warning" title="Агент офлайн">
          Показаны последние известные данные. Образы можно смотреть после подключения агента.
        </InlineNotice>
      ) : null}

      <div className={styles.toolbar}>
        <Tabs
          variant="pill"
          size="sm"
          value={view}
          onChange={(next) => setView(next as View)}
          items={[
            { id: 'images', label: 'Образы', count: images.length },
            { id: 'containers', label: 'Контейнеры', count: containers.length },
          ]}
        />
        {view === 'images' ? (
          <Tabs
            variant="pill"
            size="sm"
            value={tab}
            onChange={setTab}
            items={[
              { id: 'all', label: 'Все' },
              { id: 'tagged', label: 'С тегом' },
              { id: 'dangling', label: 'Без тега' },
            ]}
          />
        ) : null}
      </div>

      {view === 'images' ? (
        <div className={styles.panel}>
          <div className={styles.rowsHead}>
            <span>Образ</span>
            <span>ID</span>
            <span>Размер</span>
            <span>Создан</span>
            <span />
          </div>
          {imagesQuery.isPending ? (
            Array.from({ length: 5 }, (_, index) => (
              <div className={styles.row} key={index}>
                <Skeleton width="45%" height={12} />
                <Skeleton width={80} height={10} />
                <Skeleton width={60} height={10} />
                <Skeleton width={90} height={10} />
                <Skeleton width={28} height={24} />
              </div>
            ))
          ) : imagesQuery.isError ? (
            <div className={styles.stateWrap}>
              <InlineNotice tone="danger" title="Не удалось получить образы">
                {errorMessage(imagesQuery.error)}
              </InlineNotice>
            </div>
          ) : filteredImages.length === 0 ? (
            <EmptyState
              icon={Layers}
              title="Образов нет"
              description="Загрузите образ из реестра или соберите собственный из Dockerfile."
              action={<Button onClick={() => openStream('pull')}>Загрузить образ</Button>}
            />
          ) : (
            filteredImages.map((item) => {
              const reference = `${item.repository}:${item.tag}`
              const dangling = item.repository === '<none>'
              return (
                <div className={styles.row} key={item.id}>
                  <div className={styles.repoCell}>
                    <Layers size={14} className={styles.repoIcon} aria-hidden />
                    <span className={styles.repo}>{dangling ? 'без тега' : reference}</span>
                    {item.tag === 'latest' ? <Badge tone="neutral">latest</Badge> : null}
                  </div>
                  <span className={cx(styles.monoCell, styles.idCell)} title={item.id}>
                    {truncateMiddle(item.id.replace('sha256:', ''), 14)}
                  </span>
                  <span className={styles.monoCell}>{formatBytes(item.sizeBytes)}</span>
                  <span className={styles.dateCell}>{formatDate(item.createdAt, true)}</span>
                  <div className={styles.rowActions}>
                    <Menu
                      label={`Действия: ${reference}`}
                      items={[
                        {
                          id: 'inspect',
                          label: 'Информация',
                          icon: <Info size={14} />,
                          onSelect: () => void inspectObject('image', item.id, reference),
                        },
                        {
                          id: 'remove',
                          label: 'Удалить',
                          icon: <Trash2 size={14} />,
                          danger: true,
                          separator: true,
                          onSelect: () => setRemoveTarget(item),
                        },
                      ]}
                    />
                  </div>
                </div>
              )
            })
          )}
        </div>
      ) : (
        <div className={styles.panel}>
          <div className={cx(styles.rowsHead, styles.containerHead)}>
            <span>Контейнер</span>
            <span>Образ</span>
            <span>Состояние</span>
            <span>Экземпляр</span>
            <span />
          </div>
          {containersQuery.isPending ? (
            Array.from({ length: 4 }, (_, index) => (
              <div className={cx(styles.row, styles.containerRow)} key={index}>
                <Skeleton width="35%" height={12} />
                <Skeleton width="30%" height={10} />
                <Skeleton width={70} height={18} />
                <Skeleton width={80} height={10} />
                <Skeleton width={28} height={24} />
              </div>
            ))
          ) : containersQuery.isError ? (
            <div className={styles.stateWrap}>
              <InlineNotice tone="danger" title="Не удалось получить контейнеры">
                {errorMessage(containersQuery.error)}
              </InlineNotice>
            </div>
          ) : containers.length === 0 ? (
            <EmptyState
              icon={Boxes}
              title="Контейнеров нет"
              description="Dockium создаёт контейнер при запуске экземпляра. Здесь появятся и контейнеры, созданные вручную."
            />
          ) : (
            containers.map((container) => (
              <div className={cx(styles.row, styles.containerRow)} key={container.id}>
                <div className={styles.repoCell}>
                  <Boxes size={14} className={styles.repoIcon} aria-hidden />
                  <span className={styles.repo}>{container.name}</span>
                  {container.managed ? <Badge tone="accent">Dockium</Badge> : <Badge>вручную</Badge>}
                </div>
                <span className={styles.monoCell} title={container.image}>{container.image}</span>
                <div>
                  <Badge tone={container.state === 'running' ? 'success' : 'neutral'} dot>
                    {container.state}
                  </Badge>
                  <span className={styles.containerStatus}>{container.status}</span>
                </div>
                <div className={styles.instanceCell}>
                  {container.managed && container.instanceId > 0n ? (
                    <Link to={`/instances/${container.instanceId}`} className={styles.instanceLink}>
                      Открыть
                    </Link>
                  ) : (
                    <span className={styles.dim}>—</span>
                  )}
                </div>
                <div className={styles.rowActions}>
                  <Menu
                    label={`Действия: ${container.name}`}
                    items={[
                      {
                        id: 'inspect',
                        label: 'Информация',
                        icon: <Info size={14} />,
                        onSelect: () => void inspectObject('container', container.id, container.name),
                      },
                    ]}
                  />
                </div>
              </div>
            ))
          )}
        </div>
      )}

      <Dialog
        open={Boolean(streamMode)}
        onClose={streaming ? () => undefined : closeStream}
        title={streamMode === 'pull' ? 'Загрузить образ' : 'Собрать образ'}
        description={
          streamMode === 'build'
            ? 'Сборка выполняется агентом. Доступен только Dockerfile без файлов контекста.'
            : 'Docker скачает образ из реестра на выбранном агенте.'
        }
        size="lg"
        dismissible={!streaming}
        footer={
          <>
            <Button variant="ghost" onClick={closeStream} disabled={streaming}>
              {streaming ? 'Дождитесь завершения' : 'Закрыть'}
            </Button>
            {streaming ? (
              <Button
                variant="ghost"
                onClick={() => {
                  controllerRef.current?.abort()
                  closeStream()
                  toast.info('Вывод сборки закрыт', 'Процесс на агенте может продолжиться.')
                }}
              >
                Отключиться
              </Button>
            ) : (
              <Button variant="primary" onClick={() => void startStream()} disabled={!agentId}>
                {streamMode === 'pull' ? 'Загрузить' : 'Собрать'}
              </Button>
            )}
          </>
        }
      >
        <div className={styles.streamForm}>
          {streamMode === 'pull' ? (
            <Field label="Образ" required htmlFor="pull-image" hint="Например, nginx:alpine или ghcr.io/owner/image:tag">
              <Input
                id="pull-image"
                value={image}
                onChange={(event) => setImage(event.target.value)}
                placeholder="nginx:alpine"
                mono
                disabled={streaming}
                autoFocus
              />
            </Field>
          ) : (
            <>
              <Field label="Тег образа" required htmlFor="build-tag">
                <Input
                  id="build-tag"
                  value={tag}
                  onChange={(event) => setTag(event.target.value)}
                  placeholder="dockium/custom:latest"
                  mono
                  disabled={streaming}
                />
              </Field>
              <Field label="Dockerfile" htmlFor="build-dockerfile" hint="Каталог сборки пуст: используйте только инструкции без COPY из контекста.">
                <Textarea
                  id="build-dockerfile"
                  value={dockerfile}
                  onChange={(event) => setDockerfile(event.target.value)}
                  rows={8}
                  mono
                  disabled={streaming}
                  spellCheck={false}
                />
              </Field>
            </>
          )}
          {streamError ? <InlineNotice tone="danger" title="Ошибка">{streamError}</InlineNotice> : null}
          {logs.length || streaming ? (
            <div className={styles.buildLog} ref={logRef} aria-live="polite">
              {logs.map((line, index) => (
                <div key={index} className={styles.logLine}>
                  {ansiSegments(line).map((segment, position) => (
                    <span key={position} style={segment.style}>{segment.text}</span>
                  ))}
                </div>
              ))}
              {streaming ? <span className={styles.cursor} aria-hidden /> : null}
            </div>
          ) : (
            <p className={styles.streamHint}>Вывод Docker появится здесь.</p>
          )}
        </div>
      </Dialog>

      <Dialog
        open={Boolean(removeTarget)}
        onClose={() => setRemoveTarget(null)}
        title="Удалить образ?"
        description={removeTarget ? `${removeTarget.repository}:${removeTarget.tag}` : undefined}
        size="sm"
        dismissible={!working}
        footer={
          <>
            <Button variant="ghost" onClick={() => setRemoveTarget(null)} disabled={working}>Отмена</Button>
            <Button variant="dangerSolid" onClick={() => void removeImage()} loading={working}>Удалить</Button>
          </>
        }
      >
        <p className={styles.dialogText}>
          Образ будет удалён с агента. Контейнеры, использующие его, останутся, но не смогут быть
          пересозданы без повторной загрузки.
        </p>
      </Dialog>

      <Drawer
        open={Boolean(inspect) || inspectLoading}
        onClose={() => {
          setInspect(null)
          setInspectLoading(false)
        }}
        title={inspect?.title || 'Информация'}
        description="docker inspect"
        width={680}
        footer={
          inspect ? (
            <Button
              variant="secondary"
              icon={<Copy size={14} />}
              onClick={async () => {
                if (await copyText(inspect.json)) toast.success('Скопировано')
              }}
            >
              Скопировать JSON
            </Button>
          ) : null
        }
      >
        {inspectLoading ? (
          <div className={styles.inspectLoading}>
            <Skeleton width="100%" height={12} />
            <Skeleton width="92%" height={12} />
            <Skeleton width="70%" height={12} />
            <Skeleton width="100%" height={12} />
          </div>
        ) : inspect ? (
          <pre className={styles.inspectJson}>{inspect.json}</pre>
        ) : null}
      </Drawer>
    </div>
  )
}
