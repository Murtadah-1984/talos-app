import { useState, type FormEvent } from 'react'
import { Link } from 'react-router-dom'
import { api } from '../api/client'
import { useApiGet } from '../api/useApi'
import type { Organization } from '../api/types'

export function Organizations() {
  const { data: orgs, error, loading, reload } = useApiGet<Organization[]>('/api/v1/organizations')
  const [name, setName] = useState('')
  const [slug, setSlug] = useState('')
  const [submitting, setSubmitting] = useState(false)
  const [formError, setFormError] = useState<string | null>(null)

  async function onCreate(e: FormEvent) {
    e.preventDefault()
    setSubmitting(true)
    setFormError(null)
    try {
      await api.post('/api/v1/organizations', { Name: name, Slug: slug })
      setName('')
      setSlug('')
      reload()
    } catch (err) {
      setFormError(err instanceof Error ? err.message : 'Failed to create organization')
    } finally {
      setSubmitting(false)
    }
  }

  return (
    <div>
      <h1>Organizations</h1>
      <p className="muted">
        The top level of the tenancy hierarchy: Organization → Project → Environment → Cluster.
      </p>

      <form className="inline-form" onSubmit={onCreate}>
        <input placeholder="Name" value={name} onChange={(e) => setName(e.target.value)} required />
        <input placeholder="slug" value={slug} onChange={(e) => setSlug(e.target.value)} required />
        <button type="submit" disabled={submitting}>
          Create
        </button>
      </form>
      {formError && <p className="error">{formError}</p>}

      {loading && <p>Loading…</p>}
      {error && <p className="error">{error}</p>}
      {orgs && orgs.length === 0 && <p className="empty-state">No organizations yet.</p>}
      {orgs && orgs.length > 0 && (
        <table className="data-table">
          <thead>
            <tr>
              <th>Name</th>
              <th>Slug</th>
              <th></th>
            </tr>
          </thead>
          <tbody>
            {orgs.map((o) => (
              <tr key={o.ID}>
                <td>{o.Name}</td>
                <td>{o.Slug}</td>
                <td>
                  <Link to={`/projects?organizationId=${o.ID}`}>View projects</Link>
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      )}
    </div>
  )
}
