import { useState } from 'react'
import { useApiGet } from '../api/useApi'

interface ClusterTemplate {
  ID: string
  Name: string
  Version: string
  Description: string
  ProviderMode: string
}

// Templates lists versioned, reusable cluster templates (§13). Templates are
// immutable once created — a new version is a new row, never an edit.
export function Templates() {
  const [organizationId, setOrganizationId] = useState('')
  const {
    data: templates,
    error,
    loading,
  } = useApiGet<ClusterTemplate[]>(
    organizationId ? `/api/v1/templates?organizationId=${organizationId}` : null,
    [organizationId],
  )

  return (
    <div>
      <h1>Cluster Templates</h1>
      <p className="muted">Versioned, reusable cluster configurations (§13).</p>

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
      {organizationId && templates && templates.length === 0 && (
        <p className="empty-state">No templates for this organization yet.</p>
      )}
      {templates && templates.length > 0 && (
        <table className="data-table">
          <thead>
            <tr>
              <th>Name</th>
              <th>Version</th>
              <th>Provider Mode</th>
              <th>Description</th>
            </tr>
          </thead>
          <tbody>
            {templates.map((t) => (
              <tr key={t.ID}>
                <td>{t.Name}</td>
                <td>{t.Version}</td>
                <td>{t.ProviderMode}</td>
                <td>{t.Description}</td>
              </tr>
            ))}
          </tbody>
        </table>
      )}
    </div>
  )
}
