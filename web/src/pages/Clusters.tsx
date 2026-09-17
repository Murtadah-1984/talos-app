import { Link } from 'react-router-dom'
import { useApiGet } from '../api/useApi'
import type { Cluster } from '../api/types'
import { StatusBadge } from '../components/StatusBadge'

export function Clusters() {
  const { data: clusters, error, loading } = useApiGet<Cluster[]>('/api/v1/clusters')

  return (
    <div>
      <div className="page-header">
        <h1>Clusters</h1>
        <Link to="/clusters/new" className="button-link">
          + Create Cluster
        </Link>
      </div>
      <p className="muted">
        Filter by organization, project, environment, provider, or state using the API's query
        parameters.
      </p>

      {loading && <p>Loading…</p>}
      {error && <p className="error">{error}</p>}
      {clusters && clusters.length === 0 && <p className="empty-state">No clusters yet.</p>}
      {clusters && clusters.length > 0 && (
        <table className="data-table">
          <thead>
            <tr>
              <th>Name</th>
              <th>Provider Mode</th>
              <th>State</th>
              <th>Talos</th>
              <th>Kubernetes</th>
            </tr>
          </thead>
          <tbody>
            {clusters.map((c) => (
              <tr key={c.ID}>
                <td>
                  <Link to={`/clusters/${c.ID}`}>{c.Name}</Link>
                </td>
                <td>{c.ProviderMode}</td>
                <td>
                  <StatusBadge value={c.State} />
                </td>
                <td>{c.Spec.TalosVersion}</td>
                <td>{c.Spec.KubernetesVersion}</td>
              </tr>
            ))}
          </tbody>
        </table>
      )}
    </div>
  )
}
