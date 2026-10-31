import { Navigate, Outlet, Route, Routes } from 'react-router-dom'
import { LiveProvider } from './data/live'
import { useSession } from './data/session'
import { AppLayout } from './layouts/AppLayout'
import { BootScreen } from './layouts/BootScreen'
import { AgentsPage } from './pages/AgentsPage'
import { ImagesPage } from './pages/ImagesPage'
import { InstancePage } from './pages/InstancePage'
import { InstancesPage } from './pages/InstancesPage'
import { LoginPage } from './pages/LoginPage'
import { OverviewPage } from './pages/OverviewPage'
import { SecurityPage } from './pages/SecurityPage'
import { SettingsPage } from './pages/SettingsPage'
import { SetupPage } from './pages/SetupPage'
import { UsersPage } from './pages/UsersPage'

function RequireSession() {
  const { loading, user, initialized } = useSession()
  if (loading) return <BootScreen />
  if (!initialized) return <Navigate to="/setup" replace />
  if (!user) return <Navigate to="/login" replace />
  return <Outlet />
}

function SetupGate() {
  const { loading, user, initialized } = useSession()
  if (loading) return <BootScreen />
  if (initialized) return <Navigate to={user ? '/' : '/login'} replace />
  return <Outlet />
}

function LoginGate() {
  const { loading, user, initialized } = useSession()
  if (loading) return <BootScreen />
  if (!initialized) return <Navigate to="/setup" replace />
  if (user) return <Navigate to="/" replace />
  return <Outlet />
}

function AdminGate() {
  const { isAdmin } = useSession()
  if (!isAdmin) return <Navigate to="/" replace />
  return <Outlet />
}

export function App() {
  return (
    <LiveProvider>
      <Routes>
        <Route element={<SetupGate />}>
          <Route path="/setup" element={<SetupPage />} />
        </Route>
        <Route element={<LoginGate />}>
          <Route path="/login" element={<LoginPage />} />
        </Route>

        <Route element={<RequireSession />}>
          <Route element={<AppLayout />}>
            <Route index element={<OverviewPage />} />
            <Route path="instances" element={<InstancesPage />} />
            <Route path="instances/:id" element={<InstancePage />} />
            <Route path="settings" element={<SettingsPage />} />
            <Route element={<AdminGate />}>
              <Route path="agents" element={<AgentsPage />} />
              <Route path="images" element={<ImagesPage />} />
              <Route path="users" element={<UsersPage />} />
              <Route path="security" element={<SecurityPage />} />
            </Route>
          </Route>
        </Route>

        <Route path="*" element={<Navigate to="/" replace />} />
      </Routes>
    </LiveProvider>
  )
}
