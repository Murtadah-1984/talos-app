import { Link } from 'react-router-dom'
import { useApiGet } from '../api/useApi'
import type { Workflow } from '../api/types'
import { StatusBadge } from '../components/StatusBadge'

// Workflows lists every durable workflow run (§23, ADR-0005): cluster
// provisioning, upgrades, worker scaling, and so on.
export function Workflows() {
  const { data: workflows, error, loading } = useApiGet<Workflow[]>('/api/v1/workflows')

  return (
    <div>
      <h1>Workflows</h1>
      <p className="muted">Durable, resumable runs of multi-step platform operations.</p>

      {loading && <p>Loading…</p>}
      {error && <p className="error">{error}</p>}
      {workflows && workflows.length === 0 && (
        <p className="empty-state">No workflows have run yet.</p>
      )}
      {workflows && workflows.length > 0 && (
        <table className="data-table">
          <thead>
            <tr>
              <th>Type</th>
              <th>Status</th>
              <th>Step</th>
              <th>Started</th>
              <th></th>
            </tr>
          </thead>
          <tbody>
            {workflows.map((w) => (
              <tr key={w.ID}>
                <td>{w.Type}</td>
                <td>
                  <StatusBadge value={w.Status} />
                </td>
                <td>{w.CurrentStep}</td>
                <td>{new Date(w.createdAt).toLocaleString()}</td>
                <td>
                  <Link to={`/workflows/${w.ID}`}>Details</Link>
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      )}
    </div>
  )
}
