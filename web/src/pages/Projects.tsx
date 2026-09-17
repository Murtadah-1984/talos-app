import { Link, useSearchParams } from 'react-router-dom'
import { useApiGet } from '../api/useApi'
import type { Organization, Project } from '../api/types'

export function Projects() {
  const [params] = useSearchParams()
  const organizationId = params.get('organizationId')

  const { data: orgs } = useApiGet<Organization[]>('/api/v1/organizations')
  const {
    data: projects,
    error,
    loading,
  } = useApiGet<Project[]>(
    organizationId ? `/api/v1/organizations/${organizationId}/projects` : null,
    [organizationId],
  )

  return (
    <div>
      <h1>Projects</h1>
      <p className="muted">Select an organization to see its projects.</p>

      {!organizationId && (
        <ul className="link-list">
          {orgs?.map((o) => (
            <li key={o.ID}>
              <Link to={`/projects?organizationId=${o.ID}`}>{o.Name}</Link>
            </li>
          ))}
        </ul>
      )}

      {organizationId && loading && <p>Loading…</p>}
      {organizationId && error && <p className="error">{error}</p>}
      {organizationId && projects && projects.length === 0 && (
        <p className="empty-state">No projects in this organization yet.</p>
      )}
      {organizationId && projects && projects.length > 0 && (
        <table className="data-table">
          <thead>
            <tr>
              <th>Name</th>
              <th>Slug</th>
            </tr>
          </thead>
          <tbody>
            {projects.map((p) => (
              <tr key={p.ID}>
                <td>{p.Name}</td>
                <td>{p.Slug}</td>
              </tr>
            ))}
          </tbody>
        </table>
      )}
    </div>
  )
}
