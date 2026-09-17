import { useState } from 'react'
import { useParams } from 'react-router-dom'
import { api } from '../api/client'
import { useApiGet } from '../api/useApi'
import type { Cluster, GitOpsStatus, Machine } from '../api/types'
import { StatusBadge } from '../components/StatusBadge'

type Tab = 'overview' | 'nodes' | 'gitops' | 'operations'

// ClusterDetail is the cluster page from §29: health, nodes, GitOps status,
// and the operational actions §17 lists (upgrade, scale, delete). Tabs the
// backend doesn't yet observe (Networking, Storage, Observability, Cluster
// API resources) are intentionally left off rather than shown with fake
// data — see docs/roadmap.md for when those integrations land.
export function ClusterDetail() {
  const { id } = useParams<{ id: string }>()
  const [tab, setTab] = useState<Tab>('overview')
  const {
    data: cluster,
    error,
    loading,
    reload,
  } = useApiGet<Cluster>(id ? `/api/v1/clusters/${id}` : null, [id])

  if (loading) return <p>Loading…</p>
  if (error) return <p className="error">{error}</p>
  if (!cluster) return null

  return (
    <div>
      <div className="page-header">
        <h1>{cluster.Name}</h1>
        <StatusBadge value={cluster.State} />
      </div>

      <nav className="tab-bar">
        {(['overview', 'nodes', 'gitops', 'operations'] as Tab[]).map((t) => (
          <button
            key={t}
            type="button"
            className={tab === t ? 'tab active' : 'tab'}
            onClick={() => setTab(t)}
          >
            {t.charAt(0).toUpperCase() + t.slice(1)}
          </button>
        ))}
      </nav>

      {tab === 'overview' && <OverviewTab cluster={cluster} />}
      {tab === 'nodes' && <NodesTab clusterId={cluster.ID} />}
      {tab === 'gitops' && <GitOpsTab clusterId={cluster.ID} />}
      {tab === 'operations' && <OperationsTab cluster={cluster} onChanged={reload} />}
    </div>
  )
}

function OverviewTab({ cluster }: { cluster: Cluster }) {
  return (
    <dl className="detail-grid">
      <dt>Provider mode</dt>
      <dd>{cluster.ProviderMode}</dd>
      <dt>Kubernetes version</dt>
      <dd>{cluster.Spec.KubernetesVersion}</dd>
      <dt>Talos version</dt>
      <dd>{cluster.Spec.TalosVersion}</dd>
      <dt>Control plane replicas</dt>
      <dd>{cluster.Spec.ControlPlane.Replicas}</dd>
      <dt>Worker pools</dt>
      <dd>{cluster.Spec.Workers.map((w) => `${w.Name}: ${w.Replicas}`).join(', ') || 'none'}</dd>
      <dt>Pod / Service CIDR</dt>
      <dd>
        {cluster.Spec.Network.PodCIDR} / {cluster.Spec.Network.ServiceCIDR}
      </dd>
      <dt>CNI</dt>
      <dd>{cluster.Spec.CNI.Type || '—'}</dd>
      <dt>Argo CD</dt>
      <dd>{cluster.Spec.ArgoCD.Enabled ? 'enabled' : 'disabled'}</dd>
      <dt>Git commit</dt>
      <dd>{cluster.GitCommitSHA || 'not yet committed'}</dd>
    </dl>
  )
}

function NodesTab({ clusterId }: { clusterId: string }) {
  const {
    data: machines,
    error,
    loading,
  } = useApiGet<Machine[]>(`/api/v1/clusters/${clusterId}/nodes`)

  if (loading) return <p>Loading…</p>
  if (error) return <p className="error">{error}</p>
  if (!machines || machines.length === 0)
    return <p className="empty-state">No nodes assigned yet.</p>

  return (
    <table className="data-table">
      <thead>
        <tr>
          <th>Hostname</th>
          <th>Role</th>
          <th>Phase</th>
          <th>Management IP</th>
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
            <td>{m.ManagementIP}</td>
          </tr>
        ))}
      </tbody>
    </table>
  )
}

function GitOpsTab({ clusterId }: { clusterId: string }) {
  const { data, error, loading } = useApiGet<GitOpsStatus>(`/api/v1/clusters/${clusterId}/gitops`)

  if (loading) return <p>Loading…</p>
  if (error) return <p className="error">{error}</p>
  if (!data) return null

  return (
    <div>
      <h3>Argo CD Applications</h3>
      {data.Applications.length === 0 ? (
        <p className="empty-state">No Argo CD Applications observed yet.</p>
      ) : (
        <table className="data-table">
          <thead>
            <tr>
              <th>Name</th>
              <th>Sync</th>
              <th>Health</th>
              <th>Revision</th>
            </tr>
          </thead>
          <tbody>
            {data.Applications.map((a) => (
              <tr key={a.Name}>
                <td>{a.Name}</td>
                <td>
                  <StatusBadge value={a.SyncStatus} />
                </td>
                <td>
                  <StatusBadge value={a.HealthStatus} />
                </td>
                <td>{a.Revision}</td>
              </tr>
            ))}
          </tbody>
        </table>
      )}

      <h3>Change Sets</h3>
      {data.ChangeSets.length === 0 ? (
        <p className="empty-state">No GitOps change sets yet.</p>
      ) : (
        <ul className="link-list">
          {data.ChangeSets.map((c) => (
            <li key={c.ID}>
              <StatusBadge value={c.Status} /> {c.Description}{' '}
              {c.PullRequestURL && (
                <a href={c.PullRequestURL} target="_blank" rel="noreferrer">
                  PR
                </a>
              )}
            </li>
          ))}
        </ul>
      )}
    </div>
  )
}

function OperationsTab({ cluster, onChanged }: { cluster: Cluster; onChanged: () => void }) {
  const [kubernetesVersion, setKubernetesVersion] = useState(cluster.Spec.KubernetesVersion)
  const [talosVersion, setTalosVersion] = useState(cluster.Spec.TalosVersion)
  const [replicas, setReplicas] = useState(cluster.Spec.Workers[0]?.Replicas ?? 0)
  const [busy, setBusy] = useState<string | null>(null)
  const [message, setMessage] = useState<string | null>(null)

  async function run(action: string, fn: () => Promise<unknown>) {
    setBusy(action)
    setMessage(null)
    try {
      await fn()
      setMessage(`${action} started`)
      onChanged()
    } catch (err) {
      setMessage(err instanceof Error ? err.message : `${action} failed`)
    } finally {
      setBusy(null)
    }
  }

  return (
    <div className="operations-panel">
      <section>
        <h3>Upgrade</h3>
        <div className="inline-form">
          <input value={kubernetesVersion} onChange={(e) => setKubernetesVersion(e.target.value)} />
          <input value={talosVersion} onChange={(e) => setTalosVersion(e.target.value)} />
          <button
            type="button"
            disabled={busy !== null}
            onClick={() =>
              run('Upgrade', () =>
                api.post(`/api/v1/clusters/${cluster.ID}/upgrade`, {
                  kubernetesVersion,
                  talosVersion,
                }),
              )
            }
          >
            Upgrade
          </button>
        </div>
      </section>

      <section>
        <h3>Scale Workers</h3>
        <div className="inline-form">
          <input
            type="number"
            min={0}
            value={replicas}
            onChange={(e) => setReplicas(Number(e.target.value))}
          />
          <button
            type="button"
            disabled={busy !== null}
            onClick={() =>
              run('Scale', () =>
                api.post(`/api/v1/clusters/${cluster.ID}/scale`, {
                  pool: cluster.Spec.Workers[0]?.Name ?? 'default',
                  replicas,
                }),
              )
            }
          >
            Scale
          </button>
        </div>
      </section>

      <section>
        <h3>Destroy</h3>
        <button
          type="button"
          className="danger"
          disabled={busy !== null}
          onClick={() => {
            if (window.confirm(`Destroy cluster ${cluster.Name}? This cannot be undone.`)) {
              run('Delete', () => api.del(`/api/v1/clusters/${cluster.ID}`))
            }
          }}
        >
          Destroy Cluster
        </button>
      </section>

      {message && <p>{message}</p>}
    </div>
  )
}
