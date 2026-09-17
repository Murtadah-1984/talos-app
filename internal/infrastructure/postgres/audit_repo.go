package postgres

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/talos-platform/talos-platform/internal/domain/audit"
	"github.com/talos-platform/talos-platform/internal/domain/shared"
)

type AuditRepository struct {
	pool *pgxpool.Pool
}

func NewAuditRepository(pool *pgxpool.Pool) *AuditRepository {
	return &AuditRepository{pool: pool}
}

func (r *AuditRepository) RecordLog(ctx context.Context, l *audit.Log) error {
	if l.ID == (shared.ID{}) {
		l.ID = shared.NewID()
	}
	before, err := json.Marshal(l.Before)
	if err != nil {
		return fmt.Errorf("marshaling audit before-state: %w", err)
	}
	after, err := json.Marshal(l.After)
	if err != nil {
		return fmt.Errorf("marshaling audit after-state: %w", err)
	}
	_, err = r.pool.Exec(ctx,
		`INSERT INTO audit_logs (id, request_id, actor_id, actor_email, action, target_kind, target_id,
			before_state, after_state, result, ip_address, session_info, occurred_at)
		 VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13)`,
		l.ID, l.RequestID, l.ActorID, l.ActorEmail, l.Action, l.TargetKind, l.TargetID,
		before, after, l.Result, l.IPAddress, l.SessionInfo, l.OccurredAt)
	if err != nil {
		return fmt.Errorf("recording audit log: %w", err)
	}
	return nil
}

func (r *AuditRepository) ListLogs(ctx context.Context, filter audit.LogFilter, page shared.Page) ([]*audit.Log, error) {
	where := []string{"1=1"}
	args := []any{}
	arg := func(v any) string {
		args = append(args, v)
		return fmt.Sprintf("$%d", len(args))
	}
	if filter.ActorID != nil {
		where = append(where, "actor_id = "+arg(*filter.ActorID))
	}
	if filter.TargetKind != "" {
		where = append(where, "target_kind = "+arg(filter.TargetKind))
	}
	if filter.TargetID != "" {
		where = append(where, "target_id = "+arg(filter.TargetID))
	}
	if filter.Result != "" {
		where = append(where, "result = "+arg(filter.Result))
	}
	limitArg, offsetArg := arg(page.Limit), arg(page.Offset)
	query := fmt.Sprintf(
		`SELECT id, request_id, actor_id, actor_email, action, target_kind, target_id, before_state, after_state,
			result, ip_address, session_info, occurred_at
		 FROM audit_logs WHERE %s ORDER BY occurred_at DESC LIMIT %s OFFSET %s`,
		strings.Join(where, " AND "), limitArg, offsetArg)

	rows, err := r.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("listing audit logs: %w", err)
	}
	defer rows.Close()

	var out []*audit.Log
	for rows.Next() {
		l := &audit.Log{}
		var before, after []byte
		var result string
		if err := rows.Scan(&l.ID, &l.RequestID, &l.ActorID, &l.ActorEmail, &l.Action, &l.TargetKind, &l.TargetID,
			&before, &after, &result, &l.IPAddress, &l.SessionInfo, &l.OccurredAt); err != nil {
			return nil, fmt.Errorf("scanning audit log: %w", err)
		}
		l.Result = audit.Result(result)
		_ = json.Unmarshal(before, &l.Before)
		_ = json.Unmarshal(after, &l.After)
		out = append(out, l)
	}
	return out, rows.Err()
}

func (r *AuditRepository) RecordEvent(ctx context.Context, e *audit.Event) error {
	if e.ID == (shared.ID{}) {
		e.ID = shared.NewID()
	}
	dataJSON, err := json.Marshal(e.Data)
	if err != nil {
		return fmt.Errorf("marshaling event data: %w", err)
	}
	_, err = r.pool.Exec(ctx,
		`INSERT INTO events (id, source, kind, severity, target_kind, target_id, message, data, occurred_at)
		 VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9)`,
		e.ID, e.Source, e.Kind, e.Severity, e.TargetKind, e.TargetID, e.Message, dataJSON, e.OccurredAt)
	if err != nil {
		return fmt.Errorf("recording event: %w", err)
	}
	return nil
}

func (r *AuditRepository) ListEvents(ctx context.Context, targetKind, targetID string, page shared.Page) ([]*audit.Event, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT id, source, kind, severity, target_kind, target_id, message, data, occurred_at
		 FROM events WHERE target_kind = $1 AND target_id = $2 ORDER BY occurred_at DESC LIMIT $3 OFFSET $4`,
		targetKind, targetID, page.Limit, page.Offset)
	if err != nil {
		return nil, fmt.Errorf("listing events: %w", err)
	}
	defer rows.Close()

	var out []*audit.Event
	for rows.Next() {
		e := &audit.Event{}
		var severity string
		var dataJSON []byte
		if err := rows.Scan(&e.ID, &e.Source, &e.Kind, &severity, &e.TargetKind, &e.TargetID, &e.Message, &dataJSON, &e.OccurredAt); err != nil {
			return nil, fmt.Errorf("scanning event: %w", err)
		}
		e.Severity = audit.EventSeverity(severity)
		_ = json.Unmarshal(dataJSON, &e.Data)
		out = append(out, e)
	}
	return out, rows.Err()
}

func (r *AuditRepository) UpsertAlert(ctx context.Context, a *audit.Alert) error {
	if a.ID == (shared.ID{}) {
		a.ID = shared.NewID()
	}
	_, err := r.pool.Exec(ctx,
		`INSERT INTO alerts (id, target_kind, target_id, severity, status, title, detail, fired_at, resolved_at)
		 VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9)
		 ON CONFLICT (target_kind, target_id, title, status)
		 DO UPDATE SET severity = EXCLUDED.severity, detail = EXCLUDED.detail, resolved_at = EXCLUDED.resolved_at`,
		a.ID, a.TargetKind, a.TargetID, a.Severity, a.Status, a.Title, a.Detail, a.FiredAt, a.ResolvedAt)
	if err != nil {
		return fmt.Errorf("upserting alert: %w", err)
	}
	return nil
}

func (r *AuditRepository) ListAlerts(ctx context.Context, filter audit.AlertFilter, page shared.Page) ([]*audit.Alert, error) {
	where := []string{"1=1"}
	args := []any{}
	arg := func(v any) string {
		args = append(args, v)
		return fmt.Sprintf("$%d", len(args))
	}
	if filter.TargetKind != "" {
		where = append(where, "target_kind = "+arg(filter.TargetKind))
	}
	if filter.TargetID != "" {
		where = append(where, "target_id = "+arg(filter.TargetID))
	}
	if filter.Status != "" {
		where = append(where, "status = "+arg(filter.Status))
	}
	limitArg, offsetArg := arg(page.Limit), arg(page.Offset)
	query := fmt.Sprintf(
		`SELECT id, target_kind, target_id, severity, status, title, detail, fired_at, resolved_at
		 FROM alerts WHERE %s ORDER BY fired_at DESC LIMIT %s OFFSET %s`,
		strings.Join(where, " AND "), limitArg, offsetArg)

	rows, err := r.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("listing alerts: %w", err)
	}
	defer rows.Close()

	var out []*audit.Alert
	for rows.Next() {
		a := &audit.Alert{}
		var severity, status string
		if err := rows.Scan(&a.ID, &a.TargetKind, &a.TargetID, &severity, &status, &a.Title, &a.Detail, &a.FiredAt, &a.ResolvedAt); err != nil {
			return nil, fmt.Errorf("scanning alert: %w", err)
		}
		a.Severity = audit.EventSeverity(severity)
		a.Status = audit.AlertStatus(status)
		out = append(out, a)
	}
	return out, rows.Err()
}
