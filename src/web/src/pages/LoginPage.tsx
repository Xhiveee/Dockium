import { useState, type FormEvent } from 'react'
import { Link, useNavigate } from 'react-router-dom'
import { errorMessage } from '../api/client'
import { useSession } from '../data/session'
import { AuthLayout } from '../layouts/AuthLayout'
import { Button } from '../ui/Button'
import { Field, Input } from '../ui/Field'
import { Logo } from '../ui/Logo'
import styles from './AuthPage.module.css'

export function LoginPage() {
  const { signIn } = useSession()
  const navigate = useNavigate()
  const [login, setLogin] = useState('')
  const [password, setPassword] = useState('')
  const [error, setError] = useState('')
  const [busy, setBusy] = useState(false)

  const submit = async (event: FormEvent) => {
    event.preventDefault()
    if (!login.trim() || !password) {
      setError('Введите логин и пароль')
      return
    }
    setBusy(true)
    setError('')
    try {
      await signIn(login.trim(), password)
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
        <h1 className={styles.title}>Вход в панель</h1>
        <p className={styles.lead}>Введите учётные данные, чтобы управлять экземплярами.</p>
      </header>
      <form className={styles.form} onSubmit={submit} noValidate>
        <Field label="Логин" htmlFor="login">
          <Input
            id="login"
            name="login"
            autoComplete="username"
            autoFocus
            value={login}
            onChange={(event) => setLogin(event.target.value)}
            placeholder="admin"
            maxLength={32}
            disabled={busy}
          />
        </Field>
        <Field label="Пароль" htmlFor="password">
          <Input
            id="password"
            name="password"
            type="password"
            autoComplete="current-password"
            value={password}
            onChange={(event) => setPassword(event.target.value)}
            placeholder="••••••••"
            maxLength={72}
            disabled={busy}
          />
        </Field>
        {error ? <p className={styles.error} role="alert">{error}</p> : null}
        <Button type="submit" variant="primary" size="lg" fullWidth loading={busy}>
          Войти
        </Button>
      </form>
      <div className={styles.footer}>
        <span>Нет доступа? Обратитесь к администратору панели.</span>
      </div>
      <Link to="/setup" className={styles.hiddenLink} tabIndex={-1} aria-hidden>Настройка</Link>
    </AuthLayout>
  )
}
