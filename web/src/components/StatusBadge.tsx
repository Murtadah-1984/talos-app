const TONE_BY_VALUE: Record<string, 'good' | 'warn' | 'bad' | 'neutral'> = {
  READY: 'good',
  SUCCEEDED: 'good',
  SYNCED: 'good',
  HEALTHY: 'good',
  SUCCESS: 'good',
  DEGRADED: 'warn',
  UPGRADING: 'warn',
  PROVISIONING: 'warn',
  BOOTSTRAPPING: 'warn',
  INSTALLING: 'warn',
  CONFIGURING: 'warn',
  PLANNING: 'warn',
  VALIDATING: 'warn',
  PENDING: 'warn',
  RUNNING: 'warn',
  PROGRESSING: 'warn',
  NEEDS_ATTENTION: 'warn',
  OUTOFSYNC: 'warn',
  FAILED: 'bad',
  FAILURE: 'bad',
  DENIED: 'bad',
  DEGRADED_STATE: 'bad',
  DELETING: 'neutral',
  DELETED: 'neutral',
  CANCELLED: 'neutral',
  DRAFT: 'neutral',
}

// StatusBadge renders any platform state/status enum (cluster state, workflow
// status, Argo CD sync/health) with a consistent color so operators can scan
// the dashboard at a glance (§15).
export function StatusBadge({ value }: { value: string }) {
  const tone = TONE_BY_VALUE[value.toUpperCase()] ?? 'neutral'
  return <span className={`badge badge-${tone}`}>{value}</span>
}
