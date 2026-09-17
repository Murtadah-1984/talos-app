package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/talos-platform/talos-platform/internal/domain/shared"
	"github.com/talos-platform/talos-platform/internal/domain/workflow"
)

// WorkflowRepository implements workflow.Repository against Postgres,
// including the ClaimNextStep primitive that lets multiple platform-worker
// replicas safely share the pending-step queue (ADR-0005).
type WorkflowRepository struct {
	pool *pgxpool.Pool
}

func NewWorkflowRepository(pool *pgxpool.Pool) *WorkflowRepository {
	return &WorkflowRepository{pool: pool}
}

func (r *WorkflowRepository) Create(ctx context.Context, w *workflow.Workflow, steps []*workflow.Step) error {
	w.Touch()
	if w.ID == (shared.ID{}) {
		w.ID = shared.NewID()
	}
	inputJSON, err := json.Marshal(w.Input)
	if err != nil {
		return fmt.Errorf("marshaling workflow input: %w", err)
	}

	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("beginning tx: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	_, err = tx.Exec(ctx,
		`INSERT INTO workflows (id, type, idempotency_key, cluster_id, machine_id, requested_by, input,
			status, current_step, error, started_at, finished_at, created_at, updated_at)
		 VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14)`,
		w.ID, w.Type, w.IdempotencyKey, w.ClusterID, w.MachineID, w.RequestedBy, inputJSON,
		w.Status, w.CurrentStep, w.Error, w.StartedAt, w.FinishedAt, w.CreatedAt, w.UpdatedAt)
	if err != nil {
		return fmt.Errorf("inserting workflow: %w", err)
	}

	for _, s := range steps {
		s.Touch()
		if s.ID == (shared.ID{}) {
			s.ID = shared.NewID()
		}
		s.WorkflowID = w.ID
		outputJSON, err := json.Marshal(s.Output)
		if err != nil {
			return fmt.Errorf("marshaling step output: %w", err)
		}
		if _, err := tx.Exec(ctx,
			`INSERT INTO workflow_steps (id, workflow_id, sequence, name, status, output, error, attempts,
				started_at, finished_at, created_at, updated_at)
			 VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12)`,
			s.ID, s.WorkflowID, s.Sequence, s.Name, s.Status, outputJSON, s.Error, s.Attempts,
			s.StartedAt, s.FinishedAt, s.CreatedAt, s.UpdatedAt); err != nil {
			return fmt.Errorf("inserting workflow step: %w", err)
		}
	}

	return tx.Commit(ctx)
}

const workflowColumns = `id, type, idempotency_key, cluster_id, machine_id, requested_by, input,
	status, current_step, error, started_at, finished_at, created_at, updated_at`

func (r *WorkflowRepository) Get(ctx context.Context, id shared.ID) (*workflow.Workflow, error) {
	row := r.pool.QueryRow(ctx, `SELECT `+workflowColumns+` FROM workflows WHERE id = $1`, id)
	return scanWorkflow(row)
}

func (r *WorkflowRepository) GetByIdempotencyKey(ctx context.Context, key string) (*workflow.Workflow, error) {
	row := r.pool.QueryRow(ctx, `SELECT `+workflowColumns+` FROM workflows WHERE idempotency_key = $1`, key)
	return scanWorkflow(row)
}

func (r *WorkflowRepository) List(ctx context.Context, filter workflow.Filter, page shared.Page) ([]*workflow.Workflow, error) {
	where := []string{"1=1"}
	args := []any{}
	arg := func(v any) string {
		args = append(args, v)
		return fmt.Sprintf("$%d", len(args))
	}
	if filter.ClusterID != nil {
		where = append(where, "cluster_id = "+arg(*filter.ClusterID))
	}
	if filter.Type != "" {
		where = append(where, "type = "+arg(filter.Type))
	}
	if filter.Status != "" {
		where = append(where, "status = "+arg(filter.Status))
	}
	limitArg, offsetArg := arg(page.Limit), arg(page.Offset)
	query := fmt.Sprintf(`SELECT %s FROM workflows WHERE %s ORDER BY created_at DESC LIMIT %s OFFSET %s`,
		workflowColumns, strings.Join(where, " AND "), limitArg, offsetArg)

	rows, err := r.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("listing workflows: %w", err)
	}
	defer rows.Close()

	var out []*workflow.Workflow
	for rows.Next() {
		w, err := scanWorkflow(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, w)
	}
	return out, rows.Err()
}

func (r *WorkflowRepository) Update(ctx context.Context, w *workflow.Workflow) error {
	w.Touch()
	inputJSON, err := json.Marshal(w.Input)
	if err != nil {
		return fmt.Errorf("marshaling workflow input: %w", err)
	}
	tag, err := r.pool.Exec(ctx,
		`UPDATE workflows SET status=$2, current_step=$3, error=$4, started_at=$5, finished_at=$6, input=$7, updated_at=$8
		 WHERE id=$1`,
		w.ID, w.Status, w.CurrentStep, w.Error, w.StartedAt, w.FinishedAt, inputJSON, w.UpdatedAt)
	if err != nil {
		return fmt.Errorf("updating workflow: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return shared.ErrNotFound
	}
	return nil
}

func (r *WorkflowRepository) ListSteps(ctx context.Context, workflowID shared.ID) ([]*workflow.Step, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT id, workflow_id, sequence, name, status, output, error, attempts, started_at, finished_at, created_at, updated_at
		 FROM workflow_steps WHERE workflow_id = $1 ORDER BY sequence ASC`, workflowID)
	if err != nil {
		return nil, fmt.Errorf("listing workflow steps: %w", err)
	}
	defer rows.Close()

	var out []*workflow.Step
	for rows.Next() {
		s, err := scanStep(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, rows.Err()
}

func (r *WorkflowRepository) UpdateStep(ctx context.Context, s *workflow.Step) error {
	s.Touch()
	outputJSON, err := json.Marshal(s.Output)
	if err != nil {
		return fmt.Errorf("marshaling step output: %w", err)
	}
	tag, err := r.pool.Exec(ctx,
		`UPDATE workflow_steps SET status=$2, output=$3, error=$4, attempts=$5, started_at=$6, finished_at=$7, updated_at=$8
		 WHERE id=$1`,
		s.ID, s.Status, outputJSON, s.Error, s.Attempts, s.StartedAt, s.FinishedAt, s.UpdatedAt)
	if err != nil {
		return fmt.Errorf("updating workflow step: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return shared.ErrNotFound
	}
	return nil
}

// ClaimNextStep locks the next PENDING step for workflowID using
// SELECT ... FOR UPDATE SKIP LOCKED, then marks it RUNNING within the same
// transaction, so two platform-worker replicas never claim the same step
// (ADR-0005). Returns shared.ErrNotFound if there is no pending step.
func (r *WorkflowRepository) ClaimNextStep(ctx context.Context, workflowID shared.ID) (*workflow.Step, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("beginning tx: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	row := tx.QueryRow(ctx,
		`SELECT id, workflow_id, sequence, name, status, output, error, attempts, started_at, finished_at, created_at, updated_at
		 FROM workflow_steps
		 WHERE workflow_id = $1 AND status = $2
		 ORDER BY sequence ASC
		 LIMIT 1
		 FOR UPDATE SKIP LOCKED`,
		workflowID, workflow.StatusPending)

	step, err := scanStep(row)
	if err != nil {
		return nil, err
	}

	step.Status = workflow.StatusRunning
	step.Attempts++
	step.Touch()
	outputJSON, _ := json.Marshal(step.Output)
	if _, err := tx.Exec(ctx,
		`UPDATE workflow_steps SET status=$2, attempts=$3, output=$4, updated_at=$5 WHERE id=$1`,
		step.ID, step.Status, step.Attempts, outputJSON, step.UpdatedAt); err != nil {
		return nil, fmt.Errorf("claiming workflow step: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("committing step claim: %w", err)
	}
	return step, nil
}

func scanWorkflow(row rowScanner) (*workflow.Workflow, error) {
	w := &workflow.Workflow{}
	var typ, status string
	var inputJSON []byte
	err := row.Scan(&w.ID, &typ, &w.IdempotencyKey, &w.ClusterID, &w.MachineID, &w.RequestedBy, &inputJSON,
		&status, &w.CurrentStep, &w.Error, &w.StartedAt, &w.FinishedAt, &w.CreatedAt, &w.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, shared.ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("scanning workflow: %w", err)
	}
	w.Type = workflow.Type(typ)
	w.Status = workflow.Status(status)
	if len(inputJSON) > 0 {
		if err := json.Unmarshal(inputJSON, &w.Input); err != nil {
			return nil, fmt.Errorf("unmarshaling workflow input: %w", err)
		}
	}
	return w, nil
}

func scanStep(row rowScanner) (*workflow.Step, error) {
	s := &workflow.Step{}
	var status string
	var outputJSON []byte
	err := row.Scan(&s.ID, &s.WorkflowID, &s.Sequence, &s.Name, &status, &outputJSON, &s.Error, &s.Attempts,
		&s.StartedAt, &s.FinishedAt, &s.CreatedAt, &s.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, shared.ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("scanning workflow step: %w", err)
	}
	s.Status = workflow.Status(status)
	if len(outputJSON) > 0 {
		if err := json.Unmarshal(outputJSON, &s.Output); err != nil {
			return nil, fmt.Errorf("unmarshaling step output: %w", err)
		}
	}
	return s, nil
}
