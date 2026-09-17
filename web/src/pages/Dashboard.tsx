import { Link } from 'react-router-dom'
import { useApiGet } from '../api/useApi'
import type { Cluster } from '../api/types'
import { StatusBadge } from '../components/StatusBadge'

// Dashboard is the global cluster dashboard from §15: name, environment,
// provider, versions, node counts, and health, with filtering left to the
// Clusters page's own filter controls.
export function Dashboard() {
  const { data: clusters, error, loading } = useApiGet<Cluster[]>('/api/v1/clusters')

  return (
    <div>
      <h1>Dashboard</h1>
      <p className="muted">Cluster fleet overview across every organization you can see.</p>

      {loading && <p>Loading clusters…</p>}
      {error && <p className="error">{error}</p>}

      {clusters && clusters.length === 0 && (
        <p className="empty-state">No clusters yet. Create one from the Clusters page.</p>
      )}

      {clusters && clusters.length > 0 && (
        <table className="data-table">
          <thead>
            <tr>
              <th>Name</th>
              <th>Provider</th>
              <th>Talos</th>
              <th>Kubernetes</th>
              <th>Control Plane</th>
              <th>Workers</th>
              <th>State</th>
            </tr>
          </thead>
          <tbody>
            {clusters.map((c) => (
              <tr key={c.ID}>
                <td>
                  <Link to={`/clusters/${c.ID}`}>{c.Name}</Link>
                </td>
                <td>{c.ProviderMode}</td>
                <td>{c.Spec.TalosVersion}</td>
                <td>{c.Spec.KubernetesVersion}</td>
                <td>{c.Spec.ControlPlane.Replicas}</td>
                <td>{c.Spec.Workers.reduce((sum, w) => sum + w.Replicas, 0)}</td>
                <td>
                  <StatusBadge value={c.State} />
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      )}
    </div>
  )
}
