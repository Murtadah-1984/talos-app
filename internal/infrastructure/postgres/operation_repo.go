package postgres

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/talos-platform/talos-platform/internal/domain/operation"
	"github.com/talos-platform/talos-platform/internal/domain/shared"
)

type OperationRepository struct {
	pool *pgxpool.Pool
}

func NewOperationRepository(pool *pgxpool.Pool) *OperationRepository {
	return &OperationRepository{pool: pool}
}

const operationColumns = `id, idempotency_key, kind, cluster_id, machine_id, requested_by, status, result, started_at, finished_at, created_at, updated_at`

func (r *OperationRepository) Create(ctx context.Context, o *operation.Operation) error {
	o.Touch()
	if o.ID == (shared.ID{}) {
		o.ID = shared.NewID()
	}
	_, err := r.pool.Exec(ctx,
		`INSERT INTO operations (id, idempotency_key, kind, cluster_id, machine_id, requested_by, status, result, started_at, finished_at, created_at, updated_at)
		 VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12)`,
		o.ID, o.IdempotencyKey, o.Kind, o.ClusterID, o.MachineID, o.RequestedBy, o.Status, o.Result, o.StartedAt, o.FinishedAt, o.CreatedAt, o.UpdatedAt)
	if err != nil {
		return fmt.Errorf("inserting operation: %w", err)
	}
	return nil
}

func (r *OperationRepository) Get(ctx context.Context, id shared.ID) (*operation.Operation, error) {
	row := r.pool.QueryRow(ctx, `SELECT `+operationColumns+` FROM operations WHERE id = $1`, id)
	return scanOperation(row)
}

func (r *OperationRepository) GetByIdempotencyKey(ctx context.Context, key string) (*operation.Operation, error) {
	row := r.pool.QueryRow(ctx, `SELECT `+operationColumns+` FROM operations WHERE idempotency_key = $1`, key)
	return scanOperation(row)
}

func (r *OperationRepository) List(ctx context.Context, filter operation.Filter, page shared.Page) ([]*operation.Operation, error) {
	where := []string{"1=1"}
	args := []any{}
	arg := func(v any) string {
		args = append(args, v)
		return fmt.Sprintf("$%d", len(args))
	}
	if filter.ClusterID != nil {
		where = append(where, "cluster_id = "+arg(*filter.ClusterID))
	}
	if filter.MachineID != nil {
		where = append(where, "machine_id = "+arg(*filter.MachineID))
	}
	if filter.Kind != "" {
		where = append(where, "kind = "+arg(filter.Kind))
	}
	if filter.Status != "" {
		where = append(where, "status = "+arg(filter.Status))
	}
	limitArg, offsetArg := arg(page.Limit), arg(page.Offset)
	query := fmt.Sprintf(`SELECT %s FROM operations WHERE %s ORDER BY created_at DESC LIMIT %s OFFSET %s`,
		operationColumns, strings.Join(where, " AND "), limitArg, offsetArg)

	rows, err := r.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("listing operations: %w", err)
	}
	defer rows.Close()

	var out []*operation.Operation
	for rows.Next() {
		o, err := scanOperation(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, o)
	}
	return out, rows.Err()
}

func (r *OperationRepository) Update(ctx context.Context, o *operation.Operation) error {
	o.Touch()
	tag, err := r.pool.Exec(ctx,
		`UPDATE operations SET status=$2, result=$3, started_at=$4, finished_at=$5, updated_at=$6 WHERE id=$1`,
		o.ID, o.Status, o.Result, o.StartedAt, o.FinishedAt, o.UpdatedAt)
	if err != nil {
		return fmt.Errorf("updating operation: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return shared.ErrNotFound
	}
	return nil
}

func scanOperation(row rowScanner) (*operation.Operation, error) {
	o := &operation.Operation{}
	var kind, status string
	err := row.Scan(&o.ID, &o.IdempotencyKey, &kind, &o.ClusterID, &o.MachineID, &o.RequestedBy, &status, &o.Result, &o.StartedAt, &o.FinishedAt, &o.CreatedAt, &o.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, shared.ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("scanning operation: %w", err)
	}
	o.Kind = operation.Kind(kind)
	o.Status = operation.Status(status)
	return o, nil
}
