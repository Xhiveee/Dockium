import { useMemo, useState } from 'react'
import { useQueryClient } from '@tanstack/react-query'
import {
  Activity,
  AlertTriangle,
  Ban as BanIcon,
  CheckCircle2,
  KeyRound,
  Plus,
  ShieldCheck,
  Trash2,
  XCircle,
} from 'lucide-react'
import { api, errorMessage } from '../api/client'
import {
  keys,
  useApiRequestsQuery,
  useBansQuery,
  useLoginLogsQuery,
  useSessionsQuery,
} from '../data/queries'
import { useSession } from '../data/session'
import type { Ban, SessionInfo } from '../gen/panel/v1/panel_pb'
import { formatDate, formatRelative } from '../lib/format'
import { cx } from '../lib/cx'
import {
  Badge,
  EmptyState,
  InlineNotice,
  PageHeader,
  Skeleton,
} from '../ui/Display'
import { Button } from '../ui/Button'
import { Dialog } from '../ui/Dialog'
import { Field, Input, Select } from '../ui/Field'
import { Tabs } from '../ui/Tabs'
import { SearchField } from '../ui/SearchField'
import { useToast } from '../ui/Toast'
import styles from './SecurityPage.module.css'

type TabId = 'logins' | 'bans' | 'sessions' | 'api'

function statusTone(status: number): 'success' | 'warning' | 'danger' | 'neutral' {
  if (status >= 200 && status < 300) return 'success'
  if (status >= 400 && status < 500) return 'warning'
  if (status >= 500) return 'danger'
  return 'neutral'
}

function shortProcedure(procedure: string): { service: string; method: string } {
  const [service = '—', method = ''] = procedure.split('/')
  const shortService = service.split('.').pop() ?? service
  return { service: shortService, method }
}

function browserLabel(userAgent: string): string {
  const value = userAgent.toLowerCase()
  const browsers: [string, string][] = [
    ['edg/', 'Edge'],
    ['opr/', 'Opera'],
    ['firefox/', 'Firefox'],
    ['chrome/', 'Chrome'],
    ['safari/', 'Safari'],
    ['curl/', 'curl'],
  ]
  const browser = browsers.find(([token]) => value.includes(token))?.[1] ?? 'Браузер'
  const systems: [string, string][] = [
    ['windows', 'Windows'],
    ['android', 'Android'],
    ['iphone', 'iOS'],
    ['ipad', 'iOS'],
    ['mac os', 'macOS'],
    ['linux', 'Linux'],
  ]
  const system = systems.find(([token]) => value.includes(token))?.[1] ?? ''
  return system ? `${browser} · ${system}` : browser
}

export function SecurityPage() {
  const session = useSession()
  const queryClient = useQueryClient()
  const toast = useToast()
  const [tab, setTab] = useState<TabId>('logins')
  const [search, setSearch] = useState('')
  const [loginFilter, setLoginFilter] = useState('all')
  const [statusFilter, setStatusFilter] = useState('all')
  const [banOpen, setBanOpen] = useState(false)
  const [banIp, setBanIp] = useState('')
  const [banHours, setBanHours] = useState('24')
  const [banReason, setBanReason] = useState('')
  const [banError, setBanError] = useState('')
  const [savingBan, setSavingBan] = useState(false)
  const [banDelete, setBanDelete] = useState<Ban | null>(null)
  const [sessionRevoke, setSessionRevoke] = useState<SessionInfo | null>(null)
  const [working, setWorking] = useState(false)

  const loginsQuery = useLoginLogsQuery()
  const bansQuery = useBansQuery()
  const sessionsQuery = useSessionsQuery()
  const apiQuery = useApiRequestsQuery(tab === 'api')

  const logins = loginsQuery.data?.items ?? []
  const bans = bansQuery.data?.items ?? []
  const sessions = sessionsQuery.data?.items ?? []
  const apiLogs = apiQuery.data?.items ?? []
  const query = search.trim().toLowerCase()

  const filteredLogins = useMemo(
    () =>
      logins.filter((entry) => {
        if (loginFilter === 'failed' && entry.success) return false
        if (loginFilter === 'success' && !entry.success) return false
        if (!query) return true
        return `${entry.login} ${entry.ip} ${entry.country} ${entry.reason} ${entry.browser} ${entry.os}`.toLowerCase().includes(query)
      }),
    [logins, loginFilter, query],
  )

  const filteredBans = useMemo(
    () =>
      query
        ? bans.filter((ban) => `${ban.ip} ${ban.country} ${ban.reason} ${ban.source}`.toLowerCase().includes(query))
        : bans,
    [bans, query],
  )

  const filteredSessions = useMemo(
    () =>
      query
        ? sessions.filter((item) => `${item.login} ${item.ip} ${item.userAgent}`.toLowerCase().includes(query))
        : sessions,
    [sessions, query],
  )

  const filteredApi = useMemo(
    () =>
      apiLogs.filter((entry) => {
        if (statusFilter === 'errors' && entry.status < 400) return false
        if (!query) return true
        return `${entry.procedure} ${entry.login} ${entry.ip} ${entry.status}`.toLowerCase().includes(query)
      }),
    [apiLogs, statusFilter, query],
  )

  const addBan = async () => {
    if (!banIp.trim()) {
      setBanError('Укажите IP-адрес')
      return
    }
    setSavingBan(true)
    setBanError('')
    try {
      await api.security.createBan({
        ip: banIp.trim(),
        hours: Number(banHours) || 24,
        reason: banReason.trim(),
      })
      toast.success('IP заблокирован', banIp.trim())
      setBanOpen(false)
      setBanIp('')
      setBanReason('')
      await queryClient.invalidateQueries({ queryKey: keys.bans })
    } catch (error) {
      setBanError(errorMessage(error))
    } finally {
      setSavingBan(false)
    }
  }

  const removeBan = async () => {
    if (!banDelete) return
    setWorking(true)
    try {
      await api.security.deleteBan({ id: banDelete.id })
      toast.success('Блокировка снята', banDelete.ip)
      await queryClient.invalidateQueries({ queryKey: keys.bans })
      setBanDelete(null)
    } catch (error) {
      toast.error('Не удалось снять блокировку', errorMessage(error))
    } finally {
      setWorking(false)
    }
  }

  const revokeSession = async () => {
    if (!sessionRevoke) return
    setWorking(true)
    try {
      await api.security.revokeSession({ id: sessionRevoke.id })
      toast.success('Сессия завершена', sessionRevoke.login)
      await queryClient.invalidateQueries({ queryKey: keys.sessions })
      setSessionRevoke(null)
      await session.refresh()
    } catch (error) {
      toast.error('Не удалось завершить сессию', errorMessage(error))
    } finally {
      setWorking(false)
    }
  }

  return (
    <div className={styles.page}>
      <PageHeader
        title="Безопасность"
        actions={
          tab === 'bans' ? (
            <Button variant="primary" icon={<Plus size={14} />} onClick={() => setBanOpen(true)}>
              Заблокировать IP
            </Button>
          ) : null
        }
      />

      <Tabs<TabId>
        ariaLabel="Разделы безопасности"
        value={tab}
        onChange={setTab}
        items={[
          { id: 'logins', label: 'Входы', count: loginsQuery.data?.total ?? logins.length },
          { id: 'bans', label: 'Блокировки', count: bans.length },
          { id: 'sessions', label: 'Сессии', count: sessions.length },
          { id: 'api', label: 'API', count: apiQuery.data?.total ?? apiLogs.length },
        ]}
      />

      <div className={styles.toolbar}>
        <SearchField
          value={search}
          onChange={setSearch}
          placeholder="Логин, IP или действие"
          className={styles.search}
        />
        {tab === 'logins' ? (
          <Select value={loginFilter} onChange={(event) => setLoginFilter(event.target.value)} aria-label="Фильтр входов">
            <option value="all">Все попытки</option>
            <option value="failed">Неудачные</option>
            <option value="success">Успешные</option>
          </Select>
        ) : null}
        {tab === 'api' ? (
          <Select value={statusFilter} onChange={(event) => setStatusFilter(event.target.value)} aria-label="Фильтр API">
            <option value="all">Все запросы</option>
            <option value="errors">Только ошибки</option>
          </Select>
        ) : null}
        <span className={styles.toolbarHint}>
          {tab === 'logins'
            ? 'Неудачные попытки после 5 промахов за 10 минут блокируют IP на час.'
            : tab === 'sessions'
              ? 'Смена пароля завершает остальные сессии пользователя.'
              : ''}
        </span>
      </div>

      {tab === 'logins' ? (
        <div className={styles.panel}>
          <div className={styles.head}>
            <span>Статус</span>
            <span>Логин</span>
            <span>IP</span>
            <span>Страна</span>
            <span>Браузер</span>
            <span>ОС</span>
            <span>Причина</span>
            <span>Время</span>
          </div>
          {loginsQuery.isPending ? (
            <SkeletonRows rows={6} columns={8} />
          ) : loginsQuery.isError ? (
            <ErrorNotice message={errorMessage(loginsQuery.error)} onRetry={() => void loginsQuery.refetch()} />
          ) : filteredLogins.length === 0 ? (
            <EmptyState icon={ShieldCheck} title="Записей нет" description={query ? 'Измените поиск.' : 'Входы в панель появятся здесь.'} />
          ) : (
            filteredLogins.map((entry) => (
              <div className={styles.row} key={entry.id.toString()}>
                <div className={styles.statusCell}>
                  {entry.success ? (
                    <CheckCircle2 size={14} className={styles.ok} aria-label="успешно" />
                  ) : (
                    <XCircle size={14} className={styles.fail} aria-label="отклонено" />
                  )}
                  <span>{entry.success ? 'успешно' : 'отклонён'}</span>
                </div>
                <span className={styles.primaryText}>{entry.login || '—'}</span>
                <span className={styles.mono}>{entry.ip || '—'}</span>
                <span className={styles.secondaryText}>{entry.country || '—'}</span>
                <span className={styles.secondaryText}>{entry.browser || '—'}</span>
                <span className={cx(styles.secondaryText, styles.osCell)}>{entry.os || '—'}</span>
                <span className={styles.reason}>{entry.reason || '—'}</span>
                <span className={styles.time} title={formatDate(entry.createdAt)}>
                  {formatRelative(entry.createdAt)}
                </span>
              </div>
            ))
          )}
        </div>
      ) : null}

      {tab === 'bans' ? (
        <div className={styles.panel}>
          <div className={styles.headBans}>
            <span>IP</span>
            <span>Страна</span>
            <span>Причина</span>
            <span>Источник</span>
            <span>Истекает</span>
            <span />
          </div>
          {bansQuery.isPending ? (
            <SkeletonRows rows={3} columns={6} />
          ) : bansQuery.isError ? (
            <ErrorNotice message={errorMessage(bansQuery.error)} onRetry={() => void bansQuery.refetch()} />
          ) : filteredBans.length === 0 ? (
            <EmptyState
              icon={BanIcon}
              title="Активных блокировок нет"
              description="IP блокируется вручную или автоматически после неудачных входов."
              action={<Button onClick={() => setBanOpen(true)}>Добавить блокировку</Button>}
            />
          ) : (
            filteredBans.map((ban) => (
              <div className={cx(styles.row, styles.banRow)} key={ban.id.toString()}>
                <span className={styles.mono}>{ban.ip}</span>
                <span className={styles.secondaryText}>{ban.country || '—'}</span>
                <span className={styles.reason}>{ban.reason || '—'}</span>
                <div>
                  <Badge tone={ban.source === 'auto' ? 'warning' : 'neutral'}>
                    {ban.source === 'auto' ? 'авто' : 'вручную'}
                  </Badge>
                </div>
                <span className={styles.time} title={formatDate(ban.expiresAt)}>
                  {formatDate(ban.expiresAt, true)}
                </span>
                <div className={styles.rowActions}>
                  <Button
                    variant="ghost"
                    size="sm"
                    iconOnly
                    aria-label={`Снять блокировку ${ban.ip}`}
                    icon={<Trash2 size={14} />}
                    onClick={() => setBanDelete(ban)}
                  />
                </div>
              </div>
            ))
          )}
        </div>
      ) : null}

      {tab === 'sessions' ? (
        <div className={styles.sessionGrid}>
          {sessionsQuery.isPending ? (
            <SkeletonRows rows={3} columns={1} />
          ) : sessionsQuery.isError ? (
            <ErrorNotice message={errorMessage(sessionsQuery.error)} onRetry={() => void sessionsQuery.refetch()} />
          ) : filteredSessions.length === 0 ? (
            <EmptyState icon={KeyRound} title="Активных сессий нет" description={query ? 'Измените поиск.' : 'Сессии появятся после входа в панель.'} />
          ) : (
            filteredSessions.map((item) => (
              <div className={styles.sessionCard} key={item.id}>
                <div className={styles.sessionTop}>
                  <span className={styles.sessionIcon} aria-hidden>
                    <KeyRound size={14} />
                  </span>
                  <span className={styles.sessionLogin}>{item.login}</span>
                  {item.current ? <Badge tone="success" dot>текущая</Badge> : null}
                  <span className={styles.sessionIp}>{item.ip || '—'}</span>
                </div>
                <p className={styles.sessionAgent}>{browserLabel(item.userAgent)}</p>
                <div className={styles.sessionMeta}>
                  <span>Начата {formatDate(item.createdAt, true)}</span>
                  <span>·</span>
                  <span>Активность {formatRelative(item.lastSeenAt)}</span>
                </div>
                <div className={styles.sessionFooter}>
                  <span className={styles.mono}>истекает {formatDate(item.expiresAt, true)}</span>
                  <Button
                    size="sm"
                    variant="ghost"
                    onClick={() => setSessionRevoke(item)}
                  >
                    Завершить
                  </Button>
                </div>
              </div>
            ))
          )}
        </div>
      ) : null}

      {tab === 'api' ? (
        <div className={styles.panel}>
          <div className={styles.headApi}>
            <span>Код</span>
            <span>Метод</span>
            <span>Пользователь</span>
            <span>IP</span>
            <span>Время</span>
          </div>
          {apiQuery.isPending ? (
            <SkeletonRows rows={7} columns={5} />
          ) : apiQuery.isError ? (
            <ErrorNotice message={errorMessage(apiQuery.error)} onRetry={() => void apiQuery.refetch()} />
          ) : filteredApi.length === 0 ? (
            <EmptyState icon={Activity} title="Запросов нет" description={query || statusFilter === 'errors' ? 'Измените фильтр.' : 'API-запросы появятся здесь.'} />
          ) : (
            filteredApi.map((entry) => {
              const procedure = shortProcedure(entry.procedure)
              return (
                <div className={cx(styles.row, styles.apiRow)} key={entry.id.toString()}>
                  <div>
                    <Badge tone={statusTone(entry.status)}>{entry.status || '—'}</Badge>
                  </div>
                  <div className={styles.twoLine}>
                    <span className={styles.primaryText}>{procedure.method || procedure.service}</span>
                    <span className={styles.metaSmall}>{procedure.service}</span>
                  </div>
                  <span className={styles.secondaryText}>{entry.login}</span>
                  <span className={styles.mono}>{entry.ip || '—'}</span>
                  <span className={styles.time} title={formatDate(entry.createdAt)}>
                    {formatRelative(entry.createdAt)}
                  </span>
                </div>
              )
            })
          )}
        </div>
      ) : null}

      <Dialog
        open={banOpen}
        onClose={() => setBanOpen(false)}
        title="Заблокировать IP"
        description="Блокировка действует на вход в панель и API с этого адреса."
        size="sm"
        dismissible={!savingBan}
        footer={
          <>
            <Button variant="ghost" onClick={() => setBanOpen(false)} disabled={savingBan}>Отмена</Button>
            <Button variant="primary" onClick={() => void addBan()} loading={savingBan}>Заблокировать</Button>
          </>
        }
      >
        <div className={styles.form}>
          <Field label="IP-адрес" required error={banError} htmlFor="ban-ip">
            <Input
              id="ban-ip"
              value={banIp}
              onChange={(event) => setBanIp(event.target.value)}
              placeholder="203.0.113.10"
              mono
              autoFocus
              invalid={Boolean(banError)}
            />
          </Field>
          <Field label="Срок, часов" htmlFor="ban-hours">
            <Input
              id="ban-hours"
              value={banHours}
              onChange={(event) => setBanHours(event.target.value.replace(/[^0-9]/g, ''))}
              inputMode="numeric"
              mono
            />
          </Field>
          <Field label="Причина" htmlFor="ban-reason" optional>
            <Input
              id="ban-reason"
              value={banReason}
              onChange={(event) => setBanReason(event.target.value)}
              placeholder="Подозрительная активность"
              maxLength={256}
            />
          </Field>
        </div>
      </Dialog>

      <Dialog
        open={Boolean(banDelete)}
        onClose={() => setBanDelete(null)}
        title="Снять блокировку?"
        description={banDelete?.ip}
        size="sm"
        dismissible={!working}
        footer={
          <>
            <Button variant="ghost" onClick={() => setBanDelete(null)} disabled={working}>Отмена</Button>
            <Button variant="primary" onClick={() => void removeBan()} loading={working}>Снять</Button>
          </>
        }
      >
        <p className={styles.dialogText}>Адрес снова сможет входить в панель.</p>
      </Dialog>

      <Dialog
        open={Boolean(sessionRevoke)}
        onClose={() => setSessionRevoke(null)}
        title="Завершить сессию?"
        description={sessionRevoke ? `${sessionRevoke.login} · ${sessionRevoke.ip}` : undefined}
        size="sm"
        dismissible={!working}
        footer={
          <>
            <Button variant="ghost" onClick={() => setSessionRevoke(null)} disabled={working}>Отмена</Button>
            <Button variant="dangerSolid" onClick={() => void revokeSession()} loading={working}>Завершить</Button>
          </>
        }
      >
        <p className={styles.dialogText}>
          {sessionRevoke?.current
            ? 'Это ваша текущая сессия — после завершения потребуется войти снова.'
            : 'Пользователь будет разлогинен на этом устройстве.'}
        </p>
      </Dialog>
    </div>
  )
}

function SkeletonRows({ rows, columns }: { rows: number; columns: number }) {
  return (
    <>
      {Array.from({ length: rows }, (_, index) => (
        <div className={styles.row} key={index}>
          {Array.from({ length: columns }, (_, position) => (
            <Skeleton key={position} width={position === 0 ? 90 : '70%'} height={11} />
          ))}
        </div>
      ))}
    </>
  )
}

function ErrorNotice({ message, onRetry }: { message: string; onRetry: () => void }) {
  return (
    <div className={styles.stateWrap}>
      <InlineNotice
        tone="danger"
        title="Не удалось загрузить данные"
        icon={AlertTriangle}
        action={<Button size="sm" onClick={onRetry}>Повторить</Button>}
      >
        {message}
      </InlineNotice>
    </div>
  )
}
