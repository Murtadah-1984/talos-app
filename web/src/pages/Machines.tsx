import { useState } from 'react'
import { api } from '../api/client'
import { useApiGet } from '../api/useApi'
import type { Machine } from '../api/types'
import { StatusBadge } from '../components/StatusBadge'

export function Machines() {
  const { data: machines, error, loading, reload } = useApiGet<Machine[]>('/api/v1/machines')
  const [busy, setBusy] = useState<string | null>(null)

  async function runAction(id: string, label: string, path: string) {
    setBusy(id)
    try {
      await api.post(`/api/v1/machines/${id}${path}`)
      reload()
    } catch (err) {
      window.alert(err instanceof Error ? err.message : `${label} failed`)
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
      {machines && machines.length === 0 && <p className="empty-state">No machines discovered yet.</p>}
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
                  <div className="inline-form">
                    <button
                      type="button"
                      disabled={busy === m.ID}
                      onClick={() => runAction(m.ID, 'Reboot', '/reboot')}
                      title="Ask Talos to reboot the OS gracefully"
                    >
                      {busy === m.ID ? '…' : 'Reboot'}
                    </button>
                    <button
                      type="button"
                      disabled={busy === m.ID}
                      onClick={() => {
                        if (window.confirm(`Hard power-cycle ${m.Hostname} via its BMC/hypervisor? This bypasses the OS.`)) {
                          runAction(m.ID, 'Power cycle', '/power/cycle')
                        }
                      }}
                      title="Force a hard reset via the infrastructure provider (BMC/hypervisor) — works even if the OS is unresponsive"
                    >
                      Power Cycle
                    </button>
                  </div>
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      )}
    </div>
  )
}
