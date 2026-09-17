import type { ReactNode } from 'react'
import { Navigate, Route, Routes } from 'react-router-dom'
import { AuthProvider } from './auth/AuthContext'
import { useAuth } from './auth/useAuth'
import { Layout } from './components/Layout'
import { Login } from './pages/Login'
import { Dashboard } from './pages/Dashboard'
import { Organizations } from './pages/Organizations'
import { Projects } from './pages/Projects'
import { Sites } from './pages/Sites'
import { Clusters } from './pages/Clusters'
import { ClusterCreate } from './pages/ClusterCreate'
import { ClusterDetail } from './pages/ClusterDetail'
import { Machines } from './pages/Machines'
import { Templates } from './pages/Templates'
import { Infrastructure } from './pages/Infrastructure'
import { GitOpsPage } from './pages/GitOps'
import { Workflows } from './pages/Workflows'
import { WorkflowDetail } from './pages/WorkflowDetail'
import { AuditLogs } from './pages/AuditLogs'
import { Settings } from './pages/Settings'

function RequireAuth({ children }: { children: ReactNode }) {
  const { token } = useAuth()
  if (!token) return <Navigate to="/login" replace />
  return <>{children}</>
}

function AppRoutes() {
  return (
    <Routes>
      <Route path="/login" element={<Login />} />
      <Route
        path="/"
        element={
          <RequireAuth>
            <Layout />
          </RequireAuth>
        }
      >
        <Route index element={<Dashboard />} />
        <Route path="organizations" element={<Organizations />} />
        <Route path="projects" element={<Projects />} />
        <Route path="sites" element={<Sites />} />
        <Route path="clusters" element={<Clusters />} />
        <Route path="clusters/new" element={<ClusterCreate />} />
        <Route path="clusters/:id" element={<ClusterDetail />} />
        <Route path="machines" element={<Machines />} />
        <Route path="templates" element={<Templates />} />
        <Route path="infrastructure" element={<Infrastructure />} />
        <Route path="gitops" element={<GitOpsPage />} />
        <Route path="workflows" element={<Workflows />} />
        <Route path="workflows/:id" element={<WorkflowDetail />} />
        <Route path="audit" element={<AuditLogs />} />
        <Route path="settings" element={<Settings />} />
      </Route>
    </Routes>
  )
}

export function App() {
  return (
    <AuthProvider>
      <AppRoutes />
    </AuthProvider>
  )
}
