// Package workflows implements the durable, resumable workflow engine
// described in ADR-0005. A Definition is a named, ordered list of Steps;
// Engine persists progress via workflow.Repository and dispatches step
// execution through a ports.Broker so platform-worker replicas can share the
// load and survive restarts without losing progress.
package workflows

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/talos-platform/talos-platform/internal/application/ports"
	"github.com/talos-platform/talos-platform/internal/domain/audit"
	"github.com/talos-platform/talos-platform/internal/domain/shared"
	"github.com/talos-platform/talos-platform/internal/domain/workflow"
)

// StepFunc executes one workflow step. It receives the parent workflow (for
// context such as ClusterID/Input) and must be idempotent: the engine may
// call it more than once for the same step if a worker crashes mid-execution
// before the step's completion is persisted.
type StepFunc func(ctx context.Context, wf *workflow.Workflow) (map[string]any, error)

// StepDefinition names one step in a Definition's sequence.
type StepDefinition struct {
	Name string
	Run  StepFunc
}

// Definition is the registered step sequence for a workflow.Type (§23).
type Definition struct {
	Type  workflow.Type
	Steps []StepDefinition
}

const dispatchTopic = "workflow.dispatch"

// Engine coordinates workflow persistence, dispatch, and step execution.
type Engine struct {
	repo        workflow.Repository
	broker      ports.Broker
	audit       audit.Repository
	registry    map[workflow.Type]Definition
	unsubscribe func()
}

func NewEngine(repo workflow.Repository, broker ports.Broker, auditRepo audit.Repository) *Engine {
	return &Engine{
		repo:     repo,
		broker:   broker,
		audit:    auditRepo,
		registry: make(map[workflow.Type]Definition),
	}
}

// Register adds a workflow Definition. Call this during startup for every
// workflow type the process should be able to run (§23).
func (e *Engine) Register(def Definition) {
	e.registry[def.Type] = def
}

// StartConsuming subscribes to the dispatch topic so this process's workers
// execute claimed steps. Call once per process after all Definitions are
// registered. Safe to call from multiple platform-worker replicas: step
// claims are serialized in Postgres (ClaimNextStep), not by this
// subscription.
func (e *Engine) StartConsuming(ctx context.Context) error {
	unsubscribe, err := e.broker.Subscribe(ctx, dispatchTopic, e.handleDispatch)
	if err != nil {
		return fmt.Errorf("subscribing to workflow dispatch: %w", err)
	}
	e.unsubscribe = unsubscribe
	return nil
}

func (e *Engine) Stop() {
	if e.unsubscribe != nil {
		e.unsubscribe()
	}
}

type dispatchMessage struct {
	WorkflowID shared.ID `json:"workflowId"`
}

// Enqueue creates a durable workflow run (or returns the existing run for a
// repeated idempotency key, §34) and publishes the initial dispatch message.
func (e *Engine) Enqueue(ctx context.Context, wtype workflow.Type, idempotencyKey string, input map[string]any, clusterID, machineID *shared.ID, requestedBy shared.ID) (*workflow.Workflow, error) {
	if existing, err := e.repo.GetByIdempotencyKey(ctx, idempotencyKey); err == nil {
		return existing, nil
	} else if !errors.Is(err, shared.ErrNotFound) {
		return nil, fmt.Errorf("checking idempotency key: %w", err)
	}

	def, ok := e.registry[wtype]
	if !ok {
		return nil, fmt.Errorf("no workflow definition registered for type %s", wtype)
	}

	wf := &workflow.Workflow{
		ID:             shared.NewID(),
		Type:           wtype,
		IdempotencyKey: idempotencyKey,
		ClusterID:      clusterID,
		MachineID:      machineID,
		RequestedBy:    requestedBy,
		Input:          input,
		Status:         workflow.StatusPending,
	}

	steps := make([]*workflow.Step, len(def.Steps))
	for i, sd := range def.Steps {
		steps[i] = &workflow.Step{
			Sequence: i,
			Name:     sd.Name,
			Status:   workflow.StatusPending,
		}
	}

	if err := e.repo.Create(ctx, wf, steps); err != nil {
		return nil, fmt.Errorf("creating workflow: %w", err)
	}

	if err := e.dispatch(ctx, wf.ID); err != nil {
		return nil, fmt.Errorf("dispatching workflow: %w", err)
	}
	return wf, nil
}

func (e *Engine) dispatch(ctx context.Context, workflowID shared.ID) error {
	payload, err := json.Marshal(dispatchMessage{WorkflowID: workflowID})
	if err != nil {
		return err
	}
	return e.broker.Publish(ctx, dispatchTopic, payload)
}

func (e *Engine) handleDispatch(ctx context.Context, payload []byte) error {
	var msg dispatchMessage
	if err := json.Unmarshal(payload, &msg); err != nil {
		return fmt.Errorf("decoding dispatch message: %w", err)
	}
	return e.executeNext(ctx, msg.WorkflowID)
}

// executeNext claims and runs the next pending step of workflowID. If none
// remain, the workflow is marked SUCCEEDED. On step failure the workflow
// moves to NEEDS_ATTENTION (never an automatic destructive rollback, per §35)
// and stops — an operator must intervene or retry explicitly.
func (e *Engine) executeNext(ctx context.Context, workflowID shared.ID) error {
	wf, err := e.repo.Get(ctx, workflowID)
	if err != nil {
		return fmt.Errorf("loading workflow %s: %w", workflowID, err)
	}
	if wf.Status == workflow.StatusSucceeded || wf.Status == workflow.StatusFailed ||
		wf.Status == workflow.StatusCancelled || wf.Status == workflow.StatusNeedsAttention {
		return nil // terminal; nothing to do
	}

	if wf.Status == workflow.StatusPending {
		now := time.Now().UTC()
		wf.Status = workflow.StatusRunning
		wf.StartedAt = &now
		if err := e.repo.Update(ctx, wf); err != nil {
			return fmt.Errorf("marking workflow running: %w", err)
		}
	}

	step, err := e.repo.ClaimNextStep(ctx, workflowID)
	if errors.Is(err, shared.ErrNotFound) {
		return e.finish(ctx, wf, workflow.StatusSucceeded, "")
	}
	if err != nil {
		return fmt.Errorf("claiming next step for workflow %s: %w", workflowID, err)
	}

	def, ok := e.registry[wf.Type]
	if !ok || step.Sequence >= len(def.Steps) {
		return e.finish(ctx, wf, workflow.StatusFailed, fmt.Sprintf("no step definition at sequence %d for type %s", step.Sequence, wf.Type))
	}

	output, runErr := def.Steps[step.Sequence].Run(ctx, wf)
	now := time.Now().UTC()
	step.FinishedAt = &now
	if runErr != nil {
		step.Status = workflow.StatusFailed
		step.Error = runErr.Error()
		_ = e.repo.UpdateStep(ctx, step)
		return e.finish(ctx, wf, workflow.StatusNeedsAttention, fmt.Sprintf("step %q failed: %v", step.Name, runErr))
	}

	step.Status = workflow.StatusSucceeded
	step.Output = output
	if err := e.repo.UpdateStep(ctx, step); err != nil {
		return fmt.Errorf("persisting step result: %w", err)
	}

	wf.CurrentStep = step.Sequence + 1
	if err := e.repo.Update(ctx, wf); err != nil {
		return fmt.Errorf("updating workflow progress: %w", err)
	}

	e.recordEvent(ctx, wf, fmt.Sprintf("step %q succeeded", step.Name), audit.SeverityInfo)
	return e.dispatch(ctx, workflowID)
}

func (e *Engine) finish(ctx context.Context, wf *workflow.Workflow, status workflow.Status, errMsg string) error {
	now := time.Now().UTC()
	wf.Status = status
	wf.Error = errMsg
	wf.FinishedAt = &now
	if err := e.repo.Update(ctx, wf); err != nil {
		return fmt.Errorf("finishing workflow: %w", err)
	}
	sev := audit.SeverityInfo
	if status != workflow.StatusSucceeded {
		sev = audit.SeverityError
	}
	e.recordEvent(ctx, wf, fmt.Sprintf("workflow %s: %s", status, errMsg), sev)
	return nil
}

func (e *Engine) recordEvent(ctx context.Context, wf *workflow.Workflow, message string, sev audit.EventSeverity) {
	if e.audit == nil {
		return
	}
	targetID := wf.ID.String()
	_ = e.audit.RecordEvent(ctx, &audit.Event{
		Source:     "workflow-engine",
		Kind:       string(wf.Type),
		Severity:   sev,
		TargetKind: "workflow",
		TargetID:   targetID,
		Message:    message,
		OccurredAt: time.Now().UTC(),
	})
}

// Get returns the current workflow record and its steps, for API/CLI display.
func (e *Engine) Get(ctx context.Context, id shared.ID) (*workflow.Workflow, []*workflow.Step, error) {
	wf, err := e.repo.Get(ctx, id)
	if err != nil {
		return nil, nil, err
	}
	steps, err := e.repo.ListSteps(ctx, id)
	if err != nil {
		return nil, nil, err
	}
	return wf, steps, nil
}

// List proxies to the repository for API/CLI listing.
func (e *Engine) List(ctx context.Context, filter workflow.Filter, page shared.Page) ([]*workflow.Workflow, error) {
	return e.repo.List(ctx, filter, page)
}
