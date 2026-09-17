// Package audit models the audit log, generic events, and alerts (§32).
package audit

import (
	"context"
	"time"

	"github.com/talos-platform/talos-platform/internal/domain/shared"
)

// Result is the outcome of an audited action.
type Result string

const (
	ResultSuccess Result = "SUCCESS"
	ResultFailure Result = "FAILURE"
	ResultDenied  Result = "DENIED"
)

// Log is one immutable audit record. Every field in §32 has a home here.
type Log struct {
	ID          shared.ID
	RequestID   string
	ActorID     shared.ID
	ActorEmail  string
	Action      string
	TargetKind  string
	TargetID    string
	Before      map[string]any
	After       map[string]any
	Result      Result
	IPAddress   string
	SessionInfo string
	OccurredAt  time.Time
}

// EventSeverity classifies a system event.
type EventSeverity string

const (
	SeverityInfo    EventSeverity = "INFO"
	SeverityWarning EventSeverity = "WARNING"
	SeverityError   EventSeverity = "ERROR"
)

// Event is a generic, timestamped system event (workflow progress, health
// transitions, etc.) used to drive the WebSocket/SSE feed (§28) and history.
type Event struct {
	ID         shared.ID
	Source     string
	Kind       string
	Severity   EventSeverity
	TargetKind string
	TargetID   string
	Message    string
	Data       map[string]any
	OccurredAt time.Time
}

// AlertStatus is the lifecycle of an alert.
type AlertStatus string

const (
	AlertFiring   AlertStatus = "FIRING"
	AlertResolved AlertStatus = "RESOLVED"
	AlertAcked    AlertStatus = "ACKNOWLEDGED"
)

// Alert is a standing condition surfaced to operators (health, drift, upgrade
// availability, etc.).
type Alert struct {
	ID         shared.ID
	TargetKind string
	TargetID   string
	Severity   EventSeverity
	Status     AlertStatus
	Title      string
	Detail     string
	FiredAt    time.Time
	ResolvedAt *time.Time
}

// Repository persists audit logs, events, and alerts.
type Repository interface {
	RecordLog(ctx context.Context, l *Log) error
	ListLogs(ctx context.Context, filter LogFilter, page shared.Page) ([]*Log, error)

	RecordEvent(ctx context.Context, e *Event) error
	ListEvents(ctx context.Context, targetKind, targetID string, page shared.Page) ([]*Event, error)

	UpsertAlert(ctx context.Context, a *Alert) error
	ListAlerts(ctx context.Context, filter AlertFilter, page shared.Page) ([]*Alert, error)
}

// LogFilter narrows an audit log query.
type LogFilter struct {
	ActorID    *shared.ID
	TargetKind string
	TargetID   string
	Result     Result
}

// AlertFilter narrows an alert query.
type AlertFilter struct {
	TargetKind string
	TargetID   string
	Status     AlertStatus
}
