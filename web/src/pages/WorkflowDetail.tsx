import { useParams } from 'react-router-dom'
import { useApiGet } from '../api/useApi'
import type { Workflow, WorkflowStep } from '../api/types'
import { StatusBadge } from '../components/StatusBadge'

export function WorkflowDetail() {
  const { id } = useParams<{ id: string }>()
  const { data, error, loading } = useApiGet<{ workflow: Workflow; steps: WorkflowStep[] }>(
    id ? `/api/v1/workflows/${id}` : null,
    [id],
  )

  if (loading) return <p>Loading…</p>
  if (error) return <p className="error">{error}</p>
  if (!data) return null

  return (
    <div>
      <h1>{data.workflow.Type}</h1>
      <p>
        Status: <StatusBadge value={data.workflow.Status} />
      </p>
      {data.workflow.Error && <p className="error">{data.workflow.Error}</p>}

      <h2>Steps</h2>
      <ol className="step-list">
        {data.steps.map((s) => (
          <li key={s.ID}>
            <StatusBadge value={s.Status} /> {s.Name}
            {s.Error && <span className="error"> — {s.Error}</span>}
          </li>
        ))}
      </ol>
    </div>
  )
}
