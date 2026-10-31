import { useEffect, useMemo, useState } from 'react'
import { useQueryClient } from '@tanstack/react-query'
import { KeyRound, ShieldCheck, Trash2, UserPlus, Users } from 'lucide-react'
import { api, errorMessage } from '../api/client'
import { keys, useAccessQuery, useInstances, useUsersQuery } from '../data/queries'
import { useSession } from '../data/session'
import type { User } from '../gen/panel/v1/panel_pb'
import { formatDate, formatRelative, plural, roleLabel } from '../lib/format'
import { Badge, CountBadge, EmptyState, InlineNotice, PageHeader, Skeleton } from '../ui/Display'
import { Button } from '../ui/Button'
import { Dialog, Drawer } from '../ui/Dialog'
import { Field, Input, Select } from '../ui/Field'
import { Checkbox } from '../ui/Toggle'
import { Menu } from '../ui/Menu'
import { SearchField } from '../ui/SearchField'
import { useToast } from '../ui/Toast'
import styles from './UsersPage.module.css'

type AccessState = Record<string, { control: boolean; console: boolean; files: boolean }>

export function UsersPage() {
  const usersQuery = useUsersQuery()
  const instances = useInstances()
  const queryClient = useQueryClient()
  const toast = useToast()
  const { user: currentUser } = useSession()
  const [search, setSearch] = useState('')
  const [createOpen, setCreateOpen] = useState(false)
  const [editTarget, setEditTarget] = useState<User | null>(null)
  const [deleteTarget, setDeleteTarget] = useState<User | null>(null)
  const [accessTarget, setAccessTarget] = useState<User | null>(null)
  const [login, setLogin] = useState('')
  const [password, setPassword] = useState('')
  const [role, setRole] = useState('user')
  const [formError, setFormError] = useState('')
  const [busy, setBusy] = useState(false)

  const users = useMemo(() => usersQuery.data?.items ?? [], [usersQuery.data])

  const filtered = useMemo(() => {
    const query = search.trim().toLowerCase()
    if (!query) return users
    return users.filter((user) => user.login.toLowerCase().includes(query))
  }, [users, search])

  const refresh = async () => {
    await queryClient.invalidateQueries({ queryKey: keys.users })
  }

  const openCreate = () => {
    setLogin('')
    setPassword('')
    setRole('user')
    setFormError('')
    setCreateOpen(true)
  }

  const submitCreate = async () => {
    if (login.trim().length < 3) {
      setFormError('Логин: не менее 3 символов')
      return
    }
    if (password.length < 10 || !/[A-Za-zА-Яа-я]/.test(password) || !/[0-9]/.test(password)) {
      setFormError('Пароль: не менее 10 символов, буквы и цифры')
      return
    }
    setBusy(true)
    setFormError('')
    try {
      await api.users.create({ login: login.trim(), password, role })
      toast.success('Пользователь создан', login.trim())
      setCreateOpen(false)
      await refresh()
    } catch (error) {
      setFormError(errorMessage(error))
    } finally {
      setBusy(false)
    }
  }

  const openEdit = (user: User) => {
    setEditTarget(user)
    setLogin(user.login)
    setPassword('')
    setRole(user.role)
    setFormError('')
  }

  const submitEdit = async () => {
    if (!editTarget) return
    if (password && (password.length < 10 || !/[A-Za-zА-Яа-я]/.test(password) || !/[0-9]/.test(password))) {
      setFormError('Пароль: не менее 10 символов, буквы и цифры')
      return
    }
    setBusy(true)
    setFormError('')
    try {
      await api.users.update({ id: editTarget.id, role, password })
      toast.success('Пользователь обновлён', editTarget.login)
      setEditTarget(null)
      await refresh()
    } catch (error) {
      setFormError(errorMessage(error))
    } finally {
      setBusy(false)
    }
  }

  const remove = async () => {
    if (!deleteTarget) return
    setBusy(true)
    try {
      await api.users.delete({ id: deleteTarget.id })
      toast.success('Пользователь удалён', deleteTarget.login)
      setDeleteTarget(null)
      await refresh()
    } catch (error) {
      toast.error('Не удалось удалить пользователя', errorMessage(error))
    } finally {
      setBusy(false)
    }
  }

  return (
    <div className={styles.page}>
      <PageHeader
        title="Пользователи"
        badge={<CountBadge value={users.length} />}
        actions={<Button variant="primary" icon={<UserPlus size={14} />} onClick={openCreate}>Новый пользователь</Button>}
      />

      <div className={styles.toolbar}>
        <SearchField value={search} onChange={setSearch} placeholder="Поиск по логину" className={styles.search} />
      </div>

      {usersQuery.isError ? (
        <InlineNotice tone="danger" title="Не удалось загрузить пользователей">
          {errorMessage(usersQuery.error)}
        </InlineNotice>
      ) : null}

      {usersQuery.isPending ? (
        <div className={styles.panel}>
          {Array.from({ length: 4 }, (_, index) => (
            <div className={styles.row} key={index}>
              <Skeleton width={150} height={12} />
              <Skeleton width={80} height={18} />
              <Skeleton width={110} height={10} />
              <Skeleton width={100} height={10} />
              <Skeleton width={28} height={24} />
            </div>
          ))}
        </div>
      ) : filtered.length === 0 ? (
        <div className={styles.emptyPanel}>
          <EmptyState
            icon={Users}
            title={users.length ? 'Ничего не найдено' : 'Пользователей нет'}
            description={users.length ? 'Измените поисковый запрос.' : 'Создайте учётные записи для команды и выдайте доступ к экземплярам.'}
            action={users.length ? undefined : <Button variant="primary" onClick={openCreate}>Создать пользователя</Button>}
          />
        </div>
      ) : (
        <div className={styles.panel}>
          <div className={styles.head}>
            <span>Пользователь</span>
            <span>Роль</span>
            <span>Создан</span>
            <span>Последний вход</span>
            <span />
          </div>
          {filtered.map((user) => {
            const isSelf = user.id === currentUser?.id
            return (
              <div className={styles.row} key={user.id.toString()}>
                <div className={styles.userCell}>
                  <span className={styles.avatar} aria-hidden>{user.login.slice(0, 1).toUpperCase()}</span>
                  <div className={styles.nameBlock}>
                    <span className={styles.login}>
                      {user.login}
                      {isSelf ? <span className={styles.selfBadge}>вы</span> : null}
                    </span>
                    <span className={styles.id}>#{user.id}</span>
                  </div>
                </div>
                <div>
                  <Badge tone={user.role === 'admin' ? 'accent' : 'neutral'}>
                    {roleLabel(user.role)}
                  </Badge>
                </div>
                <span className={styles.date}>{formatDate(user.createdAt)}</span>
                <span className={styles.date}>
                  {user.lastLoginAt ? formatRelative(user.lastLoginAt) : '—'}
                </span>
                <div className={styles.actions}>
                  <Menu
                    label={`Действия: ${user.login}`}
                    items={[
                      {
                        id: 'edit',
                        label: 'Изменить',
                        icon: <KeyRound size={14} />,
                        onSelect: () => openEdit(user),
                      },
                      {
                        id: 'access',
                        label: 'Доступ к экземплярам',
                        icon: <ShieldCheck size={14} />,
                        disabled: user.role === 'admin',
                        onSelect: () => setAccessTarget(user),
                      },
                      {
                        id: 'delete',
                        label: 'Удалить',
                        icon: <Trash2 size={14} />,
                        danger: true,
                        separator: true,
                        disabled: isSelf,
                        onSelect: () => setDeleteTarget(user),
                      },
                    ]}
                  />
                </div>
              </div>
            )
          })}
        </div>
      )}

      <Dialog
        open={createOpen}
        onClose={() => setCreateOpen(false)}
        title="Новый пользователь"
        description="Пароль передайте пользователю безопасным каналом."
        size="md"
        dismissible={!busy}
        footer={
          <>
            <Button variant="ghost" onClick={() => setCreateOpen(false)} disabled={busy}>Отмена</Button>
            <Button variant="primary" onClick={() => void submitCreate()} loading={busy}>Создать</Button>
          </>
        }
      >
        <div className={styles.form}>
          <Field label="Логин" required htmlFor="user-login" hint="3–32 символа: латиница, цифры, точка, дефис, подчёркивание">
            <Input
              id="user-login"
              value={login}
              onChange={(event) => setLogin(event.target.value)}
              placeholder="operator"
              maxLength={32}
              autoFocus
            />
          </Field>
          <Field label="Пароль" required htmlFor="user-password" hint="Не менее 10 символов, буквы и цифры.">
            <Input
              id="user-password"
              type="password"
              autoComplete="new-password"
              value={password}
              onChange={(event) => setPassword(event.target.value)}
              placeholder="••••••••••"
              maxLength={72}
            />
          </Field>
          <Field label="Роль" htmlFor="user-role" hint="Администратор управляет агентами и доступами.">
            <Select id="user-role" value={role} onChange={(event) => setRole(event.target.value)}>
              <option value="user">Пользователь</option>
              <option value="admin">Администратор</option>
            </Select>
          </Field>
          {formError ? <InlineNotice tone="danger" title={formError} /> : null}
        </div>
      </Dialog>

      <Dialog
        open={Boolean(editTarget)}
        onClose={() => setEditTarget(null)}
        title={`Изменить ${editTarget?.login ?? ''}`}
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
          <Field label="Роль" htmlFor="edit-role">
            <Select id="edit-role" value={role} onChange={(event) => setRole(event.target.value)}>
              <option value="user">Пользователь</option>
              <option value="admin">Администратор</option>
            </Select>
          </Field>
          <Field
            label="Новый пароль"
            htmlFor="edit-password"
            optional
            hint="Если задать пароль, все сессии пользователя будут завершены."
          >
            <Input
              id="edit-password"
              type="password"
              autoComplete="new-password"
              value={password}
              onChange={(event) => setPassword(event.target.value)}
              placeholder="Оставьте пустым, чтобы не менять"
              maxLength={72}
            />
          </Field>
          {formError ? <InlineNotice tone="danger" title={formError} /> : null}
        </div>
      </Dialog>

      <Dialog
        open={Boolean(deleteTarget)}
        onClose={() => setDeleteTarget(null)}
        title="Удалить пользователя?"
        description={deleteTarget ? `${deleteTarget.login} · ${roleLabel(deleteTarget.role)}` : undefined}
        size="sm"
        dismissible={!busy}
        footer={
          <>
            <Button variant="ghost" onClick={() => setDeleteTarget(null)} disabled={busy}>Отмена</Button>
            <Button variant="dangerSolid" onClick={() => void remove()} loading={busy}>Удалить</Button>
          </>
        }
      >
        <p className={styles.dialogText}>
          Учётная запись и её сессии будут удалены. Экземпляры и их файлы останутся на месте.
        </p>
      </Dialog>

      <AccessDrawer
        user={accessTarget}
        onClose={() => setAccessTarget(null)}
        instances={instances}
        onSaved={() => {
          toast.success('Доступы сохранены')
          setAccessTarget(null)
        }}
      />
    </div>
  )
}

function AccessDrawer({
  user,
  onClose,
  instances,
  onSaved,
}: {
  user: User | null
  onClose: () => void
  instances: { id: bigint; name: string; agentName: string; image: string }[]
  onSaved: () => void
}) {
  const accessQuery = useAccessQuery(user?.id.toString())
  const toast = useToast()
  const [state, setState] = useState<AccessState>({})
  const [search, setSearch] = useState('')
  const [saving, setSaving] = useState(false)

  useEffect(() => {
    if (!user) return
    const next: AccessState = {}
    for (const item of accessQuery.data?.items ?? []) {
      next[item.instanceId.toString()] = {
        control: item.control,
        console: item.console,
        files: item.files,
      }
    }
    setState(next)
    setSearch('')
  }, [user, accessQuery.data])

  const filtered = useMemo(() => {
    const query = search.trim().toLowerCase()
    if (!query) return instances
    return instances.filter((instance) => `${instance.name} ${instance.agentName}`.toLowerCase().includes(query))
  }, [instances, search])

  const selectedCount = Object.values(state).filter(
    (item) => item.control || item.console || item.files,
  ).length

  const setPermission = (instanceId: string, permission: 'control' | 'console' | 'files', value: boolean) => {
    setState((current) => {
      const base = current[instanceId] ?? { control: false, console: false, files: false }
      return {
        ...current,
        [instanceId]: { ...base, [permission]: value },
      }
    })
  }

  const save = async () => {
    if (!user) return
    setSaving(true)
    try {
      const items = Object.entries(state)
        .filter(([, value]) => value.control || value.console || value.files)
        .map(([instanceId, value]) => ({
          instanceId: BigInt(instanceId),
          control: value.control,
          console: value.console,
          files: value.files,
        }))
      await api.users.setAccess({ userId: user.id, items })
      onSaved()
    } catch (error) {
      toast.error('Не удалось сохранить доступы', errorMessage(error))
    } finally {
      setSaving(false)
    }
  }

  return (
    <Drawer
      open={Boolean(user)}
      onClose={onClose}
      title={user ? `Доступ: ${user.login}` : 'Доступ'}
      description="Отметьте экземпляры и разрешения. Администраторы имеют полный доступ всегда."
      width={620}
      dismissible={!saving}
      footer={
        <>
          <Button variant="ghost" onClick={onClose} disabled={saving}>Отмена</Button>
          <Button variant="primary" onClick={() => void save()} loading={saving}>
            Сохранить доступы
          </Button>
        </>
      }
    >
      <div className={styles.accessBody}>
        {user?.role === 'admin' ? (
          <InlineNotice tone="info" title="Администратор">
            Роль администратора даёт полный доступ ко всем экземплярам, агентам и настройкам.
          </InlineNotice>
        ) : (
          <>
            <SearchField
              value={search}
              onChange={setSearch}
              placeholder="Найти экземпляр"
              ariaLabel="Поиск экземпляра"
            />
            <div className={styles.accessSummary}>
              <span>{instances.length} {plural(instances.length, ['экземпляр', 'экземпляра', 'экземпляров'])}</span>
              <span>·</span>
              <span>{selectedCount} с доступом</span>
            </div>
            {accessQuery.isPending ? (
              <div className={styles.accessLoading}>
                <Skeleton width="100%" height={46} />
                <Skeleton width="100%" height={46} />
                <Skeleton width="100%" height={46} />
              </div>
            ) : instances.length === 0 ? (
              <EmptyState compact icon={ShieldCheck} title="Экземпляров нет" description="Сначала создайте экземпляр, затем выдайте доступ." />
            ) : (
              <div className={styles.accessList}>
                <div className={styles.accessHead}>
                  <span>Экземпляр</span>
                  <span>Управление</span>
                  <span>Консоль</span>
                  <span>Файлы</span>
                </div>
                {filtered.map((instance) => {
                  const id = instance.id.toString()
                  const permission = state[id] ?? { control: false, console: false, files: false }
                  return (
                    <div className={styles.accessRow} key={id}>
                      <div className={styles.accessName}>
                        <span>{instance.name}</span>
                        <span className={styles.accessMeta}>{instance.agentName} · {instance.image}</span>
                      </div>
                      <Checkbox
                        checked={permission.control}
                        onChange={(value) => setPermission(id, 'control', value)}
                        label=""
                        ariaLabel={`Управление: ${instance.name}`}
                      />
                      <Checkbox
                        checked={permission.console}
                        onChange={(value) => setPermission(id, 'console', value)}
                        label=""
                        ariaLabel={`Консоль: ${instance.name}`}
                      />
                      <Checkbox
                        checked={permission.files}
                        onChange={(value) => setPermission(id, 'files', value)}
                        label=""
                        ariaLabel={`Файлы: ${instance.name}`}
                      />
                    </div>
                  )
                })}
              </div>
            )}
          </>
        )}
      </div>
    </Drawer>
  )
}
