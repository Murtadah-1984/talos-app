import { useAuth } from '../auth/useAuth'

export function Settings() {
  const { userId, logout } = useAuth()

  return (
    <div>
      <h1>Settings</h1>
      <section>
        <h2>Session</h2>
        <p>Signed in as user {userId}.</p>
        <button type="button" onClick={logout}>
          Sign out
        </button>
      </section>
      <section>
        <h2>About</h2>
        <p className="muted">
          Talos Platform — a Kubernetes cluster lifecycle and platform management system. Git and
          Argo CD remain the source of truth and reconciliation engine (see docs/architecture.md).
        </p>
      </section>
    </div>
  )
}
