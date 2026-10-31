import { useSession } from '../data/session'
import { Logo } from '../ui/Logo'
import styles from './BootScreen.module.css'

export function BootScreen() {
  const { version } = useSession()
  return (
    <div className={styles.screen}>
      <div className={styles.inner}>
        <Logo />
        <div className={styles.progress} role="progressbar" aria-label="Загрузка панели">
          <span className={styles.bar} />
        </div>
        <p className={styles.caption}>Подключение к панели</p>
      </div>
      <span className={styles.version}>v{version || '…'}</span>
    </div>
  )
}
