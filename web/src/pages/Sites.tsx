import { useState, type FormEvent } from 'react'
import { useApiGet } from '../api/useApi'
import { api } from '../api/client'
import type { Organization, Site } from '../api/types'

// Sites models physical/logical locations (§16). Names, countries, and
// cities are entirely operator-supplied — nothing here is hardcoded.
export function Sites() {
  const { data: orgs } = useApiGet<Organization[]>('/api/v1/organizations')
  const [organizationId, setOrganizationId] = useState('')
  const {
    data: sites,
    error,
    loading,
    reload,
  } = useApiGet<Site[]>(organizationId ? `/api/v1/organizations/${organizationId}/sites` : null, [
    organizationId,
  ])

  const [name, setName] = useState('')
  const [country, setCountry] = useState('')
  const [city, setCity] = useState('')
  const [formError, setFormError] = useState<string | null>(null)

  async function onCreate(e: FormEvent) {
    e.preventDefault()
    setFormError(null)
    try {
      await api.post('/api/v1/sites', {
        OrganizationID: organizationId,
        Name: name,
        Country: country,
        City: city,
      })
      setName('')
      setCountry('')
      setCity('')
      reload()
    } catch (err) {
      setFormError(err instanceof Error ? err.message : 'Failed to create site')
    }
  }

  return (
    <div>
      <h1>Sites</h1>
      <p className="muted">Physical or logical locations that host infrastructure.</p>

      <label>
        Organization
        <select value={organizationId} onChange={(e) => setOrganizationId(e.target.value)}>
          <option value="">Select an organization…</option>
          {orgs?.map((o) => (
            <option key={o.ID} value={o.ID}>
              {o.Name}
            </option>
          ))}
        </select>
      </label>

      {organizationId && (
        <>
          <form className="inline-form" onSubmit={onCreate}>
            <input
              placeholder="Name"
              value={name}
              onChange={(e) => setName(e.target.value)}
              required
            />
            <input
              placeholder="Country"
              value={country}
              onChange={(e) => setCountry(e.target.value)}
            />
            <input placeholder="City" value={city} onChange={(e) => setCity(e.target.value)} />
            <button type="submit">Create</button>
          </form>
          {formError && <p className="error">{formError}</p>}

          {loading && <p>Loading…</p>}
          {error && <p className="error">{error}</p>}
          {sites && sites.length === 0 && <p className="empty-state">No sites yet.</p>}
          {sites && sites.length > 0 && (
            <table className="data-table">
              <thead>
                <tr>
                  <th>Name</th>
                  <th>Country</th>
                  <th>City</th>
                </tr>
              </thead>
              <tbody>
                {sites.map((s) => (
                  <tr key={s.ID}>
                    <td>{s.Name}</td>
                    <td>{s.Country}</td>
                    <td>{s.City}</td>
                  </tr>
                ))}
              </tbody>
            </table>
          )}
        </>
      )}
    </div>
  )
}
