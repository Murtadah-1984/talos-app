import { useState } from 'react'
import { useApiGet } from '../api/useApi'

interface InfrastructureProvider {
  ID: string
  Name: string
  Type: string
  Endpoint: string
}

// Infrastructure lists registered infrastructure providers (§10, ADR-0007).
// Phase 1 ships Bare Metal and Proxmox as mock adapters — see
// docs/roadmap.md Phase 6 for the real implementations.
export function Infrastructure() {
  const [organizationId, setOrganizationId] = useState('')
  const {
    data: providers,
    error,
    loading,
  } = useApiGet<InfrastructureProvider[]>(
    organizationId ? `/api/v1/infrastructure-providers?organizationId=${organizationId}` : null,
    [organizationId],
  )

  return (
    <div>
      <h1>Infrastructure Providers</h1>
      <p className="muted">
        Bare Metal and Proxmox today; the provider interface (ADR-0007) allows more to be added
        without touching cluster lifecycle code.
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
      {organizationId && providers && providers.length === 0 && (
        <p className="empty-state">No infrastructure providers registered yet.</p>
      )}
      {providers && providers.length > 0 && (
        <table className="data-table">
          <thead>
            <tr>
              <th>Name</th>
              <th>Type</th>
              <th>Endpoint</th>
            </tr>
          </thead>
          <tbody>
            {providers.map((p) => (
              <tr key={p.ID}>
                <td>{p.Name}</td>
                <td>{p.Type}</td>
                <td>{p.Endpoint || '—'}</td>
              </tr>
            ))}
          </tbody>
        </table>
      )}
    </div>
  )
}
