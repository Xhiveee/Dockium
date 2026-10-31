import { useState } from 'react'
import { useQueryClient } from '@tanstack/react-query'
import { Link } from 'react-router-dom'
import { KeyRound, MonitorSmartphone, ShieldCheck, UserRound } from 'lucide-react'
import { api, errorMessage } from '../api/client'
import { useLive } from '../data/live'
import { keys } from '../data/queries'
import { useSession } from '../data/session'
import type { MeResponse } from '../gen/panel/v1/panel_pb'
import { roleLabel } from '../lib/format'
import { Badge, InlineNotice, KeyValue, PageHeader } from '../ui/Display'
import { Button } from '../ui/Button'
import { Field, Input } from '../ui/Field'
import { Panel } from '../ui/Panel'
import { useToast } from '../ui/Toast'
import styles from './SettingsPage.module.css'

export function SettingsPage() {
  const { user, isAdmin, version, refresh } = useSession()
  const live = useLive()
  const toast = useToast()
  const queryClient = useQueryClient()

  const [login, setLogin] = useState(user?.login ?? '')
  const [profileBusy, setProfileBusy] = useState(false)
  const [profileError, setProfileError] = useState('')

  const [currentPassword, setCurrentPassword] = useState('')
  const [newPassword, setNewPassword] = useState('')
  const [confirmPassword, setConfirmPassword] = useState('')
  const [passwordBusy, setPasswordBusy] = useState(false)
  const [passwordError, setPasswordError] = useState('')

  const saveProfile = async () => {
    if (!/^[a-zA-Z0-9_.-]{3,32}$/.test(login.trim())) {
      setProfileError('3–32 символа: латиница, цифры, точка, дефис или подчёркивание')
      return
    }
    setProfileBusy(true)
    setProfileError('')
    try {
      const updated = await api.auth.updateProfile({ login: login.trim() })
      queryClient.setQueryData<MeResponse | null>(['me'], (current) =>
        current ? { ...current, user: updated } : current,
      )
      toast.success('Логин обновлён', updated.login)
      await refresh()
    } catch (error) {
      setProfileError(errorMessage(error))
    } finally {
      setProfileBusy(false)
    }
  }

  const changePassword = async () => {
    if (!currentPassword) {
      setPasswordError('Введите текущий пароль')
      return
    }
    if (newPassword.length < 10 || !/[A-Za-zА-Яа-я]/.test(newPassword) || !/[0-9]/.test(newPassword)) {
      setPasswordError('Новый пароль: не менее 10 символов, буквы и цифры')
      return
    }
    if (newPassword !== confirmPassword) {
      setPasswordError('Пароли не совпадают')
      return
    }
    setPasswordBusy(true)
    setPasswordError('')
    try {
      await api.auth.changePassword({ currentPassword, newPassword })
      toast.success('Пароль изменён', 'Остальные сессии завершены.')
      setCurrentPassword('')
      setNewPassword('')
      setConfirmPassword('')
    } catch (error) {
      setPasswordError(errorMessage(error))
    } finally {
      setPasswordBusy(false)
    }
  }

  return (
    <div className={styles.page}>
      <PageHeader title="Настройки" description="Профиль и параметры текущей сессии." />

      <div className={styles.grid}>
        <Panel title="Профиль" icon={<UserRound size={14} />}>
          <div className={styles.stack}>
            <KeyValue
              columns={2}
              items={[
                { label: 'Роль', value: <Badge tone={isAdmin ? 'accent' : 'neutral'}>{roleLabel(user?.role ?? 'user')}</Badge> },
                { label: 'Идентификатор', value: `#${user?.id ?? '—'}` },
              ]}
            />
            <Field label="Логин" htmlFor="profile-login" hint="Используется для входа в панель." error={profileError}>
              <div className={styles.inlineForm}>
                <Input
                  id="profile-login"
                  value={login}
                  onChange={(event) => setLogin(event.target.value)}
                  maxLength={32}
                  invalid={Boolean(profileError)}
                />
                <Button variant="secondary" onClick={() => void saveProfile()} loading={profileBusy}>
                  Сохранить
                </Button>
              </div>
            </Field>
          </div>
        </Panel>

        <Panel title="Безопасность" icon={<ShieldCheck size={14} />} className={styles.securityPanel}>
          <div className={styles.stack}>
            <div className={styles.sessionPanel}>
              <div className={styles.sessionInfo}>
                <MonitorSmartphone size={15} aria-hidden />
                <div>
                  <p className={styles.sessionTitle}>Поток событий</p>
                  <p className={styles.sessionMeta}>
                    {live.status === 'live'
                      ? 'Соединение с панелью активно'
                      : live.status === 'reconnecting'
                        ? 'Переподключение'
                        : live.status === 'connecting'
                          ? 'Подключение'
                          : 'Нет соединения'}
                  </p>
                </div>
                <span className={styles.sessionDot} data-status={live.status} aria-hidden />
              </div>
              {isAdmin ? (
                <Link to="/security" className={styles.securityLink}>
                  <KeyRound size={14} />
                  Управление сессиями и блокировками
                </Link>
              ) : (
                <p className={styles.hint}>Сессии и журнал безопасности доступны администратору.</p>
              )}
            </div>
            <KeyValue
              columns={2}
              items={[
                { label: 'Версия панели', value: version || '—' },
                { label: 'Адрес', value: <code className={styles.code}>{window.location.origin}</code> },
              ]}
            />
          </div>
        </Panel>

        <Panel title="Пароль" icon={<KeyRound size={14} />}>
          <div className={styles.passwordForm}>
            <Field label="Текущий пароль" htmlFor="current-password">
              <Input
                id="current-password"
                type="password"
                autoComplete="current-password"
                value={currentPassword}
                onChange={(event) => setCurrentPassword(event.target.value)}
                maxLength={72}
              />
            </Field>
            <div className={styles.passwordRow}>
              <Field label="Новый пароль" htmlFor="new-password" hint="Не менее 10 символов, буквы и цифры.">
                <Input
                  id="new-password"
                  type="password"
                  autoComplete="new-password"
                  value={newPassword}
                  onChange={(event) => setNewPassword(event.target.value)}
                  maxLength={72}
                />
              </Field>
              <Field label="Повторите пароль" htmlFor="confirm-password">
                <Input
                  id="confirm-password"
                  type="password"
                  autoComplete="new-password"
                  value={confirmPassword}
                  onChange={(event) => setConfirmPassword(event.target.value)}
                  maxLength={72}
                />
              </Field>
            </div>
            {passwordError ? <InlineNotice tone="danger" title={passwordError} /> : null}
            <div className={styles.formActions}>
              <p className={styles.hint}>При смене пароля остальные сессии завершаются.</p>
              <Button variant="primary" onClick={() => void changePassword()} loading={passwordBusy}>
                Изменить пароль
              </Button>
            </div>
          </div>
        </Panel>
      </div>
    </div>
  )
}
