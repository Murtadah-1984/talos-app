import { useState } from 'react'
import { api } from '../api/client'
import { useApiGet } from '../api/useApi'
import type { Machine } from '../api/types'
import { StatusBadge } from '../components/StatusBadge'

export function Machines() {
  const { data: machines, error, loading, reload } = useApiGet<Machine[]>('/api/v1/machines')
  const [busy, setBusy] = useState<string | null>(null)

  async function reboot(id: string) {
    setBusy(id)
    try {
      await api.post(`/api/v1/machines/${id}/reboot`)
      reload()
    } catch (err) {
      window.alert(err instanceof Error ? err.message : 'Reboot failed')
    } finally {
      setBusy(null)
    }
  }

  return (
    <div>
      <h1>Machines</h1>
      <p className="muted">Every Talos machine the platform knows about, across all clusters.</p>

      {loading && <p>Loading…</p>}
      {error && <p className="error">{error}</p>}
      {machines && machines.length === 0 && (
        <p className="empty-state">No machines discovered yet.</p>
      )}
      {machines && machines.length > 0 && (
        <table className="data-table">
          <thead>
            <tr>
              <th>Hostname</th>
              <th>Role</th>
              <th>Phase</th>
              <th>Talos</th>
              <th>Kubernetes</th>
              <th>Management IP</th>
              <th></th>
            </tr>
          </thead>
          <tbody>
            {machines.map((m) => (
              <tr key={m.ID}>
                <td>{m.Hostname}</td>
                <td>{m.Role}</td>
                <td>
                  <StatusBadge value={m.Phase} />
                </td>
                <td>{m.TalosVersion}</td>
                <td>{m.KubernetesVersion}</td>
                <td>{m.ManagementIP}</td>
                <td>
                  <button type="button" disabled={busy === m.ID} onClick={() => reboot(m.ID)}>
                    {busy === m.ID ? 'Rebooting…' : 'Reboot'}
                  </button>
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      )}
    </div>
  )
}
