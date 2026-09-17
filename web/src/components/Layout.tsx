import { NavLink, Outlet } from 'react-router-dom'
import { useAuth } from '../auth/useAuth'

const NAV_SECTIONS: { to: string; label: string }[] = [
  { to: '/', label: 'Dashboard' },
  { to: '/organizations', label: 'Organizations' },
  { to: '/projects', label: 'Projects' },
  { to: '/sites', label: 'Sites' },
  { to: '/clusters', label: 'Clusters' },
  { to: '/machines', label: 'Machines' },
  { to: '/templates', label: 'Templates' },
  { to: '/infrastructure', label: 'Infrastructure' },
  { to: '/gitops', label: 'GitOps' },
  { to: '/workflows', label: 'Workflows' },
  { to: '/audit', label: 'Audit Logs' },
  { to: '/settings', label: 'Settings' },
]

export function Layout() {
  const { logout } = useAuth()

  return (
    <div className="app-shell">
      <aside className="sidebar">
        <div className="brand">Talos Platform</div>
        <nav>
          {NAV_SECTIONS.map((section) => (
            <NavLink
              key={section.to}
              to={section.to}
              end={section.to === '/'}
              className={({ isActive }) => (isActive ? 'nav-link active' : 'nav-link')}
            >
              {section.label}
            </NavLink>
          ))}
        </nav>
        <button type="button" className="logout" onClick={logout}>
          Sign out
        </button>
      </aside>
      <main className="content">
        <Outlet />
      </main>
    </div>
  )
}
