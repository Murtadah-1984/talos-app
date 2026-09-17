import { useApiGet } from '../api/useApi'
import type { AuditLog } from '../api/types'
import { StatusBadge } from '../components/StatusBadge'

export function AuditLogs() {
  const { data: logs, error, loading } = useApiGet<AuditLog[]>('/api/v1/audit')

  return (
    <div>
      <h1>Audit Logs</h1>
      <p className="muted">Who did what, when, to what, and with what result (§32).</p>

      {loading && <p>Loading…</p>}
      {error && <p className="error">{error}</p>}
      {logs && logs.length === 0 && <p className="empty-state">No audited actions yet.</p>}
      {logs && logs.length > 0 && (
        <table className="data-table">
          <thead>
            <tr>
              <th>When</th>
              <th>Actor</th>
              <th>Action</th>
              <th>Target</th>
              <th>Result</th>
            </tr>
          </thead>
          <tbody>
            {logs.map((l) => (
              <tr key={l.ID}>
                <td>{new Date(l.OccurredAt).toLocaleString()}</td>
                <td>{l.ActorEmail}</td>
                <td>{l.Action}</td>
                <td>
                  {l.TargetKind}/{l.TargetID}
                </td>
                <td>
                  <StatusBadge value={l.Result} />
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      )}
    </div>
  )
}
