import { useState, type FormEvent } from 'react'
import { useNavigate } from 'react-router-dom'
import { errorMessage } from '../api/client'
import { useSession } from '../data/session'
import { AuthLayout } from '../layouts/AuthLayout'
import { Button } from '../ui/Button'
import { Field, Input } from '../ui/Field'
import { Logo } from '../ui/Logo'
import { useToast } from '../ui/Toast'
import styles from './AuthPage.module.css'

const loginPattern = /^[a-zA-Z0-9_.-]{3,32}$/

export function SetupPage() {
  const { createAdmin } = useSession()
  const navigate = useNavigate()
  const toast = useToast()
  const [login, setLogin] = useState('')
  const [password, setPassword] = useState('')
  const [confirm, setConfirm] = useState('')
  const [errors, setErrors] = useState<Record<string, string>>({})
  const [error, setError] = useState('')
  const [busy, setBusy] = useState(false)

  const submit = async (event: FormEvent) => {
    event.preventDefault()
    const nextErrors: Record<string, string> = {}
    if (!loginPattern.test(login.trim())) {
      nextErrors.login = '3–32 символа: латиница, цифры, точка, дефис или подчёркивание'
    }
    if (password.length < 10) {
      nextErrors.password = 'Не менее 10 символов'
    } else if (!/[A-Za-zА-Яа-я]/.test(password) || !/[0-9]/.test(password)) {
      nextErrors.password = 'Добавьте хотя бы одну букву и одну цифру'
    } else if (password.length > 72) {
      nextErrors.password = 'Не более 72 символов'
    }
    if (confirm !== password) {
      nextErrors.confirm = 'Пароли не совпадают'
    }
    setErrors(nextErrors)
    if (Object.keys(nextErrors).length) return
    setBusy(true)
    setError('')
    try {
      await createAdmin(login.trim(), password)
      toast.success('Администратор создан', 'Теперь панель готова к работе.')
      navigate('/', { replace: true })
    } catch (submitError) {
      setError(errorMessage(submitError))
    } finally {
      setBusy(false)
    }
  }

  return (
    <AuthLayout>
      <div className={styles.mobileBrand}>
        <Logo />
      </div>
      <header className={styles.head}>
        <h1 className={styles.title}>Первый запуск</h1>
        <p className={styles.lead}>
          Создайте администратора панели. Этот аккаунт получит полный доступ к агентам и экземплярам.
        </p>
      </header>
      <form className={styles.form} onSubmit={submit} noValidate>
        <Field label="Логин" htmlFor="setup-login" error={errors.login} required>
          <Input
            id="setup-login"
            autoComplete="username"
            autoFocus
            value={login}
            onChange={(event) => setLogin(event.target.value)}
            placeholder="admin"
            maxLength={32}
            invalid={Boolean(errors.login)}
            disabled={busy}
          />
        </Field>
        <Field
          label="Пароль"
          htmlFor="setup-password"
          error={errors.password}
          hint="Не менее 10 символов, буквы и цифры."
          required
        >
          <Input
            id="setup-password"
            type="password"
            autoComplete="new-password"
            value={password}
            onChange={(event) => setPassword(event.target.value)}
            placeholder="Минимум 10 символов"
            maxLength={72}
            invalid={Boolean(errors.password)}
            disabled={busy}
          />
        </Field>
        <Field label="Повторите пароль" htmlFor="setup-confirm" error={errors.confirm} required>
          <Input
            id="setup-confirm"
            type="password"
            autoComplete="new-password"
            value={confirm}
            onChange={(event) => setConfirm(event.target.value)}
            placeholder="Ещё раз"
            maxLength={72}
            invalid={Boolean(errors.confirm)}
            disabled={busy}
          />
        </Field>
        {error ? <p className={styles.error} role="alert">{error}</p> : null}
        <Button type="submit" variant="primary" size="lg" fullWidth loading={busy}>
          Создать администратора
        </Button>
      </form>
      <div className={styles.footer}>
        <span>Пароль хранится в виде bcrypt-хэша и не передаётся в открытом виде.</span>
      </div>
    </AuthLayout>
  )
}
