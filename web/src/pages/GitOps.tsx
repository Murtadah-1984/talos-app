import { useState } from 'react'
import { useApiGet } from '../api/useApi'

interface GitRepository {
  ID: string
  Owner: string
  Name: string
  DefaultBranch: string
}

// GitOps lists registered Git repositories used as the source of truth for
// managed clusters (ADR-0004). Per-cluster sync/drift status lives on each
// cluster's own GitOps tab (see ClusterDetail), since that's where an
// Argo CD Application is actually scoped.
export function GitOpsPage() {
  const [organizationId, setOrganizationId] = useState('')
  const {
    data: repos,
    error,
    loading,
  } = useApiGet<GitRepository[]>(
    organizationId ? `/api/v1/gitops/repositories?organizationId=${organizationId}` : null,
    [organizationId],
  )

  return (
    <div>
      <h1>GitOps</h1>
      <p className="muted">
        Git is the source of truth for desired cluster state (ADR-0004); Argo CD reconciles it
        (ADR-0003). Open a cluster's GitOps tab for its live sync status.
      </p>

      <label>
        Organization ID
        <input
          placeholder="paste an organization ID"
          value={organizationId}
          onChange={(e) => setOrganizationId(e.target.value)}
        />
      </label>

      {loading && <p>Loading…</p>}
      {error && <p className="error">{error}</p>}
      {organizationId && repos && repos.length === 0 && (
        <p className="empty-state">No Git repositories registered yet.</p>
      )}
      {repos && repos.length > 0 && (
        <table className="data-table">
          <thead>
            <tr>
              <th>Owner</th>
              <th>Repository</th>
              <th>Default Branch</th>
            </tr>
          </thead>
          <tbody>
            {repos.map((r) => (
              <tr key={r.ID}>
                <td>{r.Owner}</td>
                <td>{r.Name}</td>
                <td>{r.DefaultBranch}</td>
              </tr>
            ))}
          </tbody>
        </table>
      )}
    </div>
  )
}
