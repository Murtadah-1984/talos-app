import { useState, type FormEvent } from 'react'
import { useNavigate } from 'react-router-dom'
import { api } from '../api/client'
import type { Cluster, PlanResult } from '../api/types'

// ClusterCreate implements the guided cluster-creation flow (§49) as one
// sectioned form rather than ten separate screens — the fields are the same
// ones the wizard in the spec walks through: cluster info, infrastructure,
// control plane, workers, networking, storage, and GitOps. Review/Plan is a
// separate step: Create leaves the cluster in DRAFT, then Plan shows the
// exact diff before Provision commits to it.
export function ClusterCreate() {
  const navigate = useNavigate()

  const [organizationId, setOrganizationId] = useState('')
  const [projectId, setProjectId] = useState('')
  const [environmentId, setEnvironmentId] = useState('')
  const [siteId, setSiteId] = useState('')
  const [name, setName] = useState('')
  const [providerMode, setProviderMode] = useState<'DIRECT_TALOS' | 'CLUSTER_API'>('DIRECT_TALOS')

  const [kubernetesVersion, setKubernetesVersion] = useState('v1.31.1')
  const [talosVersion, setTalosVersion] = useState('v1.8.2')
  const [cpReplicas, setCpReplicas] = useState(3)
  const [workerReplicas, setWorkerReplicas] = useState(3)
  const [podCIDR, setPodCIDR] = useState('10.244.0.0/16')
  const [serviceCIDR, setServiceCIDR] = useState('10.96.0.0/12')
  const [cni, setCni] = useState('calico')
  const [csi, setCsi] = useState('')
  const [ingress, setIngress] = useState('')
  const [argoCDEnabled, setArgoCDEnabled] = useState(true)

  const [created, setCreated] = useState<Cluster | null>(null)
  const [plan, setPlan] = useState<PlanResult | null>(null)
  const [error, setError] = useState<string | null>(null)
  const [submitting, setSubmitting] = useState(false)

  async function onCreate(e: FormEvent) {
    e.preventDefault()
    setError(null)
    setSubmitting(true)
    try {
      const cluster = await api.post<Cluster>('/api/v1/clusters', {
        OrganizationID: organizationId,
        ProjectID: projectId,
        EnvironmentID: environmentId,
        SiteID: siteId,
        Name: name,
        ProviderMode: providerMode,
        Spec: {
          KubernetesVersion: kubernetesVersion,
          TalosVersion: talosVersion,
          ControlPlane: { Replicas: cpReplicas, CPU: 4, MemoryGB: 8 },
          Workers: [{ Name: 'default', Replicas: workerReplicas, CPU: 4, MemoryGB: 16 }],
          Network: { PodCIDR: podCIDR, ServiceCIDR: serviceCIDR },
          CNI: { Type: cni },
          Storage: { CSI: csi },
          Ingress: { Controller: ingress },
          ArgoCD: { Enabled: argoCDEnabled, Project: 'default' },
        },
      })
      setCreated(cluster)
      const planResult = await api.post<PlanResult>(`/api/v1/clusters/${cluster.ID}/plan`)
      setPlan(planResult)
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to create cluster')
    } finally {
      setSubmitting(false)
    }
  }

  async function onProvision() {
    if (!created) return
    setSubmitting(true)
    setError(null)
    try {
      await api.post(`/api/v1/clusters/${created.ID}/provision`)
      navigate(`/clusters/${created.ID}`)
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to start provisioning')
    } finally {
      setSubmitting(false)
    }
  }

  if (created && plan) {
    return (
      <div>
        <h1>Review: {created.Name}</h1>
        <p className="muted">This is the exact set of changes provisioning will make (§49).</p>
        <pre className="plan-box">{plan.Actions.map((a) => `${a}\n`).join('')}</pre>
        {error && <p className="error">{error}</p>}
        <button type="button" disabled={submitting} onClick={onProvision}>
          {submitting ? 'Starting…' : 'Deploy'}
        </button>
      </div>
    )
  }

  return (
    <div>
      <h1>Create Cluster</h1>
      <form className="wizard-form" onSubmit={onCreate}>
        <fieldset>
          <legend>1. Cluster Information</legend>
          <label>
            Name
            <input value={name} onChange={(e) => setName(e.target.value)} required />
          </label>
          <label>
            Provider mode
            <select
              value={providerMode}
              onChange={(e) => setProviderMode(e.target.value as typeof providerMode)}
            >
              <option value="DIRECT_TALOS">Direct Talos</option>
              <option value="CLUSTER_API">Cluster API</option>
            </select>
          </label>
        </fieldset>

        <fieldset>
          <legend>2. Infrastructure</legend>
          <label>
            Organization ID
            <input
              value={organizationId}
              onChange={(e) => setOrganizationId(e.target.value)}
              required
            />
          </label>
          <label>
            Project ID
            <input value={projectId} onChange={(e) => setProjectId(e.target.value)} required />
          </label>
          <label>
            Environment ID
            <input
              value={environmentId}
              onChange={(e) => setEnvironmentId(e.target.value)}
              required
            />
          </label>
          <label>
            Site ID
            <input value={siteId} onChange={(e) => setSiteId(e.target.value)} required />
          </label>
        </fieldset>

        <fieldset>
          <legend>3. Control Plane &amp; 4. Workers</legend>
          <label>
            Kubernetes version
            <input
              value={kubernetesVersion}
              onChange={(e) => setKubernetesVersion(e.target.value)}
              required
            />
          </label>
          <label>
            Talos version
            <input
              value={talosVersion}
              onChange={(e) => setTalosVersion(e.target.value)}
              required
            />
          </label>
          <label>
            Control plane replicas
            <input
              type="number"
              min={1}
              value={cpReplicas}
              onChange={(e) => setCpReplicas(Number(e.target.value))}
            />
          </label>
          <label>
            Worker replicas
            <input
              type="number"
              min={0}
              value={workerReplicas}
              onChange={(e) => setWorkerReplicas(Number(e.target.value))}
            />
          </label>
        </fieldset>

        <fieldset>
          <legend>5. Networking &amp; 6. Storage</legend>
          <label>
            Pod CIDR
            <input value={podCIDR} onChange={(e) => setPodCIDR(e.target.value)} required />
          </label>
          <label>
            Service CIDR
            <input value={serviceCIDR} onChange={(e) => setServiceCIDR(e.target.value)} required />
          </label>
          <label>
            CNI
            <input value={cni} onChange={(e) => setCni(e.target.value)} placeholder="calico" />
          </label>
          <label>
            CSI (optional)
            <input
              value={csi}
              onChange={(e) => setCsi(e.target.value)}
              placeholder="e.g. rook-ceph"
            />
          </label>
          <label>
            Ingress controller (optional)
            <input
              value={ingress}
              onChange={(e) => setIngress(e.target.value)}
              placeholder="e.g. ingress-nginx"
            />
          </label>
        </fieldset>

        <fieldset>
          <legend>7. GitOps</legend>
          <label className="checkbox-label">
            <input
              type="checkbox"
              checked={argoCDEnabled}
              onChange={(e) => setArgoCDEnabled(e.target.checked)}
            />
            Configure Argo CD GitOps for this cluster
          </label>
        </fieldset>

        {error && <p className="error">{error}</p>}

        <button type="submit" disabled={submitting}>
          {submitting ? 'Creating…' : '8. Review Plan'}
        </button>
      </form>
    </div>
  )
}
