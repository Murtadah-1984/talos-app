// Types mirroring the platform's Go domain model (see internal/domain/*).
// Kept intentionally minimal — only the fields the UI actually renders.

export type ProviderMode = 'DIRECT_TALOS' | 'CLUSTER_API'

export type ClusterState =
  | 'DRAFT'
  | 'PLANNING'
  | 'VALIDATING'
  | 'PROVISIONING'
  | 'BOOTSTRAPPING'
  | 'INSTALLING'
  | 'CONFIGURING'
  | 'READY'
  | 'DEGRADED'
  | 'UPGRADING'
  | 'FAILED'
  | 'DELETING'
  | 'DELETED'

export interface ClusterSpec {
  KubernetesVersion: string
  TalosVersion: string
  ControlPlane: { Replicas: number; CPU: number; MemoryGB: number }
  Workers: { Name: string; Replicas: number; CPU: number; MemoryGB: number }[]
  Network: { PodCIDR: string; ServiceCIDR: string }
  CNI: { Type: string }
  Storage: { CSI: string }
  Ingress: { Controller: string }
  ArgoCD: { Enabled: boolean; Project: string }
}

export interface Cluster {
  ID: string
  OrganizationID: string
  ProjectID: string
  EnvironmentID: string
  SiteID: string
  Name: string
  Endpoint: string
  ProviderMode: ProviderMode
  State: ClusterState
  Spec: ClusterSpec
  GitCommitSHA: string
  createdAt: string
  updatedAt: string
}

export type MachineRole = 'CONTROL_PLANE' | 'WORKER'
export type MachinePhase =
  | 'DISCOVERED'
  | 'PROVISIONING'
  | 'INSTALLING'
  | 'READY'
  | 'UPGRADING'
  | 'DRAINING'
  | 'CORDONED'
  | 'UNHEALTHY'
  | 'DELETING'
  | 'DELETED'

export interface Machine {
  ID: string
  ClusterID: string | null
  Hostname: string
  ManagementIP: string
  Role: MachineRole
  Phase: MachinePhase
  TalosVersion: string
  KubernetesVersion: string
  CPU: number
  MemoryBytes: number
}

export type WorkflowStatus =
  'PENDING' | 'RUNNING' | 'SUCCEEDED' | 'FAILED' | 'NEEDS_ATTENTION' | 'CANCELLED'

export interface Workflow {
  ID: string
  Type: string
  Status: WorkflowStatus
  ClusterID: string | null
  CurrentStep: number
  Error: string
  createdAt: string
  updatedAt: string
}

export interface WorkflowStep {
  ID: string
  Sequence: number
  Name: string
  Status: WorkflowStatus
  Error: string
}

export interface Organization {
  ID: string
  Name: string
  Slug: string
}

export interface Project {
  ID: string
  OrganizationID: string
  Name: string
  Slug: string
}

export interface Site {
  ID: string
  OrganizationID: string
  Name: string
  Country: string
  City: string
}

export interface AuditLog {
  ID: string
  ActorEmail: string
  Action: string
  TargetKind: string
  TargetID: string
  Result: 'SUCCESS' | 'FAILURE' | 'DENIED'
  OccurredAt: string
}

export interface PlanResult {
  Actions: string[]
}

export interface GitOpsStatus {
  Applications: {
    Name: string
    SyncStatus: string
    HealthStatus: string
    Revision: string
  }[]
  ChangeSets: {
    ID: string
    Description: string
    PullRequestURL: string
    Status: string
  }[]
}
