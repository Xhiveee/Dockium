import { memo, useEffect, useMemo, useRef, useState } from 'react'
import { ArrowDown, RefreshCw, Send, Trash2, Download } from 'lucide-react'
import { useConsole } from '../../data/console'
import type { ConsoleLine, Instance } from '../../gen/panel/v1/panel_pb'
import { ConsoleLineKind, InstanceStatus } from '../../gen/panel/v1/panel_pb'
import { ansiSegments, stripAnsi } from '../../lib/ansi'
import { formatUptime, instanceStatusInfo } from '../../lib/format'
import { cx } from '../../lib/cx'
import { Badge, EmptyState, InlineNotice } from '../../ui/Display'
import { Button } from '../../ui/Button'
import { useToast } from '../../ui/Toast'
import { errorMessage } from '../../api/client'
import styles from './ConsoleTab.module.css'

const MAX_RENDERED = 2000

function timeLabel(timestamp: bigint): string {
  const value = Number(timestamp)
  if (!value) return '--:--:--'
  return new Date(value).toLocaleTimeString('ru-RU', { hour12: false })
}

const ConsoleLineRow = memo(function ConsoleLineRow({ line }: { line: ConsoleLine }) {
  const segments = useMemo(() => ansiSegments(line.text), [line.text])
  const system = line.kind === ConsoleLineKind.SYSTEM
  const stderr = line.kind === ConsoleLineKind.STDERR
  return (
    <div className={cx(styles.line, system && styles.systemLine, stderr && styles.stderrLine)}>
      <span className={styles.time}>{timeLabel(line.timestamp)}</span>
      <span className={styles.text}>
        {system ? <span className={styles.sysMark} aria-hidden>·</span> : null}
        {segments.map((segment, index) => (
          <span key={index} className={segment.className} style={segment.style}>
            {segment.text}
          </span>
        ))}
      </span>
    </div>
  )
})

export function ConsoleTab({ instance }: { instance: Instance }) {
  const status = instanceStatusInfo(instance.status, instance.exitCode, instance.lastError)
  const console = useConsole({
    instanceId: instance.id,
    enabled: Boolean(instance.permissions?.console),
    agentOnline: instance.agentOnline,
    shouldReconnect:
      instance.status === InstanceStatus.RUNNING || instance.status === InstanceStatus.STARTING,
  })
  const toast = useToast()
  const [command, setCommand] = useState('')
  const [sending, setSending] = useState(false)
  const [history, setHistory] = useState<string[]>([])
  const [historyIndex, setHistoryIndex] = useState(-1)
  const [autoScroll, setAutoScroll] = useState(true)
  const [atBottom, setAtBottom] = useState(true)
  const scrollRef = useRef<HTMLDivElement | null>(null)
  const inputRef = useRef<HTMLInputElement | null>(null)

  const visibleLines = useMemo(
    () => (console.lines.length > MAX_RENDERED ? console.lines.slice(-MAX_RENDERED) : console.lines),
    [console.lines],
  )

  useEffect(() => {
    if (!autoScroll) return
    const element = scrollRef.current
    if (element) element.scrollTop = element.scrollHeight
  }, [visibleLines, autoScroll])

  const handleScroll = () => {
    const element = scrollRef.current
    if (!element) return
    const distance = element.scrollHeight - element.scrollTop - element.clientHeight
    const nearBottom = distance < 48
    setAtBottom(nearBottom)
    if (!nearBottom) setAutoScroll(false)
  }

  const scrollToBottom = () => {
    const element = scrollRef.current
    if (element) element.scrollTop = element.scrollHeight
    setAutoScroll(true)
    setAtBottom(true)
  }

  const submit = async () => {
    const value = command.trim()
    if (!value || sending) return
    setSending(true)
    try {
      await console.send(value)
      setHistory((current) => [...current.filter((item) => item !== value), value].slice(-30))
      setHistoryIndex(-1)
      setCommand('')
      inputRef.current?.focus()
    } catch (error) {
      toast.error('Команда не отправлена', errorMessage(error))
    } finally {
      setSending(false)
    }
  }

  const download = () => {
    const text = console.lines
      .map((line) => `[${timeLabel(line.timestamp)}] ${stripAnsi(line.text)}`)
      .join('\n')
    const blob = new Blob([text], { type: 'text/plain;charset=utf-8' })
    const url = URL.createObjectURL(blob)
    const anchor = document.createElement('a')
    anchor.href = url
    anchor.download = `${instance.name}-console.txt`
    anchor.click()
    window.setTimeout(() => URL.revokeObjectURL(url), 3000)
  }

  const statusBadge =
    console.status === 'live' ? (
      <Badge tone="success" dot>В эфире</Badge>
    ) : console.status === 'connecting' ? (
      <Badge tone="info" dot>Подключение</Badge>
    ) : console.status === 'reconnecting' ? (
      <Badge tone="warning" dot>Переподключение</Badge>
    ) : console.status === 'error' ? (
      <Badge tone="danger" dot>Ошибка</Badge>
    ) : console.status === 'ended' ? (
      <Badge tone="neutral" dot>Поток завершён</Badge>
    ) : (
      <Badge tone="neutral">Ожидание</Badge>
    )

  return (
    <div className={styles.wrap}>
      {!instance.agentOnline ? (
        <InlineNotice tone="warning" title="Агент офлайн">
          Консоль восстановится автоматически после подключения агента.
        </InlineNotice>
      ) : null}
      <section className={styles.panel} aria-label="Консоль экземпляра">
        <header className={styles.toolbar}>
          <div className={styles.toolbarLeft}>
            {statusBadge}
            <span className={styles.statusMeta}>
              {status.label}
              {instance.status === InstanceStatus.RUNNING && instance.lastStartedAt
                ? ` · ${formatUptime(instance.lastStartedAt)}`
                : ''}
            </span>
            {instance.tty ? <span className={styles.tty}>TTY</span> : null}
          </div>
          <div className={styles.toolbarActions}>
            <Button
              variant="ghost"
              size="sm"
              icon={<ArrowDown size={14} />}
              onClick={scrollToBottom}
              aria-pressed={autoScroll}
              title="Следовать за новыми строками"
            >
              Автопрокрутка
            </Button>
            <Button variant="ghost" size="sm" icon={<Trash2 size={14} />} onClick={console.clear} disabled={!console.lines.length}>
              Очистить
            </Button>
            <Button variant="ghost" size="sm" icon={<Download size={14} />} onClick={download} disabled={!console.lines.length}>
              Скачать
            </Button>
            {console.status === 'ended' || console.status === 'error' ? (
              <Button variant="ghost" size="sm" icon={<RefreshCw size={14} />} onClick={console.retry}>
                Подключиться
              </Button>
            ) : null}
          </div>
        </header>

        {console.error && console.status === 'error' ? (
          <div className={styles.errorBar}>
            <InlineNotice
              tone="danger"
              title="Консоль прервана"
              action={<Button size="sm" onClick={console.retry}>Повторить</Button>}
            >
              {console.error}
            </InlineNotice>
          </div>
        ) : null}

        <div className={styles.screen} ref={scrollRef} onScroll={handleScroll} tabIndex={0}>
          {visibleLines.length === 0 ? (
            <EmptyState
              compact
              title={console.status === 'connecting' ? 'Ожидание вывода…' : 'Вывод пуст'}
              description={
                console.status === 'connecting'
                  ? 'Подключаем поток логов контейнера.'
                  : 'Как только контейнер что-то выведет, строки появятся здесь.'
              }
            />
          ) : (
            visibleLines.map((line) => (
              <ConsoleLineRow key={`${line.seq}-${line.timestamp}`} line={line} />
            ))
          )}
          {!atBottom ? (
            <button type="button" className={styles.jump} onClick={scrollToBottom}>
              <ArrowDown size={13} /> К последним
            </button>
          ) : null}
        </div>

        <form
          className={styles.inputRow}
          onSubmit={(event) => {
            event.preventDefault()
            void submit()
          }}
        >
          <span className={styles.prompt} aria-hidden>$</span>
          <input
            ref={inputRef}
            className={styles.input}
            value={command}
            onChange={(event) => setCommand(event.target.value)}
            placeholder="Команда в контейнер и Enter"
            aria-label="Команда в контейнер"
            disabled={!instance.agentOnline}
            autoComplete="off"
            spellCheck={false}
            onKeyDown={(event) => {
              if (event.key === 'ArrowUp') {
                event.preventDefault()
                if (!history.length) return
                const nextIndex = historyIndex < 0 ? history.length - 1 : Math.max(0, historyIndex - 1)
                setHistoryIndex(nextIndex)
                setCommand(history[nextIndex] ?? '')
              } else if (event.key === 'ArrowDown') {
                event.preventDefault()
                if (historyIndex < 0) return
                const nextIndex = historyIndex + 1
                if (nextIndex >= history.length) {
                  setHistoryIndex(-1)
                  setCommand('')
                } else {
                  setHistoryIndex(nextIndex)
                  setCommand(history[nextIndex] ?? '')
                }
              }
            }}
          />
          <Button
            type="submit"
            variant="secondary"
            size="sm"
            icon={<Send size={14} />}
            loading={sending}
            disabled={!command.trim() || !instance.agentOnline}
            aria-label="Отправить команду"
          >
            Отправить
          </Button>
        </form>
      </section>
    </div>
  )
}
