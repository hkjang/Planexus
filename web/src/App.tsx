import { Box, CircularProgress } from '@mui/material'
import { Navigate, Route, Routes } from 'react-router-dom'
import { useAuth } from './auth/AuthContext'
import { LoginPage } from './pages/LoginPage'
import { AppShell } from './layout/AppShell'
import { ExecutiveDashboard } from './pages/ExecutiveDashboard'
import { PersonalPage } from './pages/PersonalPage'
import { EntityPage } from './pages/EntityPage'
import { AdminPage } from './pages/AdminPage'
import { ProfileSecurityPage } from './pages/ProfileSecurityPage'
import { AIAssistantPage } from './pages/AIAssistantPage'

export function App() {
  const { user, loading, has } = useAuth()
  if (loading) return <Box sx={{ minHeight: '100vh', display: 'grid', placeItems: 'center' }}><CircularProgress aria-label="로딩 중" /></Box>
  if (!user) return <Routes><Route path="*" element={<LoginPage />} /></Routes>
  if (user.mustChangePassword) return <Routes><Route element={<AppShell />}><Route path="profile/security" element={<ProfileSecurityPage />} /><Route path="*" element={<Navigate to="/profile/security" replace />} /></Route></Routes>
  return <Routes>
    <Route element={<AppShell />}>
      <Route index element={has('dashboard:read') ? <ExecutiveDashboard /> : <Navigate to="/personal" replace />} />
      <Route path="personal" element={<PersonalPage />} />
      <Route path="strategy" element={<EntityPage kind="strategy" />} />
      <Route path="kpi" element={<EntityPage kind="kpi" />} />
      <Route path="projects" element={<EntityPage kind="project" />} />
      <Route path="plans" element={<EntityPage kind="plan" />} />
      <Route path="decisions" element={<EntityPage kind="decision" />} />
      <Route path="intelligence" element={<EntityPage kind="intelligence" />} />
      <Route path="scenarios" element={<EntityPage kind="scenario" />} />
      <Route path="ai" element={<AIAssistantPage />} />
      <Route path="profile/security" element={<ProfileSecurityPage />} />
      <Route path="admin/:section?" element={has('*') ? <AdminPage /> : <Navigate to="/personal" replace />} />
      <Route path="*" element={<Navigate to="/" replace />} />
    </Route>
  </Routes>
}
