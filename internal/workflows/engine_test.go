package workflows_test

import (
	"context"
	"fmt"
	"sync"
	"testing"

	"github.com/talos-platform/talos-platform/internal/domain/shared"
	"github.com/talos-platform/talos-platform/internal/domain/workflow"
	"github.com/talos-platform/talos-platform/internal/infrastructure/inprocess"
	"github.com/talos-platform/talos-platform/internal/workflows"
)

// fakeRepo is a minimal in-memory workflow.Repository used to exercise the
// engine's dispatch/claim/finish logic without a real Postgres instance.
type fakeRepo struct {
	mu        sync.Mutex
	workflows map[shared.ID]*workflow.Workflow
	steps     map[shared.ID][]*workflow.Step
	byKey     map[string]shared.ID
}

func newFakeRepo() *fakeRepo {
	return &fakeRepo{
		workflows: make(map[shared.ID]*workflow.Workflow),
		steps:     make(map[shared.ID][]*workflow.Step),
		byKey:     make(map[string]shared.ID),
	}
}

func (f *fakeRepo) Create(_ context.Context, w *workflow.Workflow, steps []*workflow.Step) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.workflows[w.ID] = w
	f.byKey[w.IdempotencyKey] = w.ID
	for _, s := range steps {
		s.WorkflowID = w.ID
		if s.ID == (shared.ID{}) {
			s.ID = shared.NewID()
		}
	}
	f.steps[w.ID] = steps
	return nil
}

func (f *fakeRepo) Get(_ context.Context, id shared.ID) (*workflow.Workflow, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	w, ok := f.workflows[id]
	if !ok {
		return nil, shared.ErrNotFound
	}
	return w, nil
}

func (f *fakeRepo) GetByIdempotencyKey(_ context.Context, key string) (*workflow.Workflow, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	id, ok := f.byKey[key]
	if !ok {
		return nil, shared.ErrNotFound
	}
	return f.workflows[id], nil
}

func (f *fakeRepo) List(_ context.Context, _ workflow.Filter, _ shared.Page) ([]*workflow.Workflow, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []*workflow.Workflow
	for _, w := range f.workflows {
		out = append(out, w)
	}
	return out, nil
}

func (f *fakeRepo) Update(_ context.Context, w *workflow.Workflow) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.workflows[w.ID] = w
	return nil
}

func (f *fakeRepo) ListSteps(_ context.Context, workflowID shared.ID) ([]*workflow.Step, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.steps[workflowID], nil
}

func (f *fakeRepo) UpdateStep(_ context.Context, s *workflow.Step) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, existing := range f.steps[s.WorkflowID] {
		if existing.ID == s.ID {
			*existing = *s
		}
	}
	return nil
}

func (f *fakeRepo) ClaimNextStep(_ context.Context, workflowID shared.ID) (*workflow.Step, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, s := range f.steps[workflowID] {
		if s.Status == workflow.StatusPending {
			s.Status = workflow.StatusRunning
			s.Attempts++
			return s, nil
		}
	}
	return nil, shared.ErrNotFound
}

func TestEngine_RunsStepsInOrderAndSucceeds(t *testing.T) {
	repo := newFakeRepo()
	broker := inprocess.NewBroker()
	engine := workflows.NewEngine(repo, broker, nil)

	var executed []string
	engine.Register(workflows.Definition{
		Type: "TEST_WORKFLOW",
		Steps: []workflows.StepDefinition{
			{Name: "step-1", Run: func(_ context.Context, _ *workflow.Workflow) (map[string]any, error) {
				executed = append(executed, "step-1")
				return map[string]any{"ok": true}, nil
			}},
			{Name: "step-2", Run: func(_ context.Context, _ *workflow.Workflow) (map[string]any, error) {
				executed = append(executed, "step-2")
				return nil, nil
			}},
		},
	})

	if err := engine.StartConsuming(context.Background()); err != nil {
		t.Fatalf("StartConsuming: %v", err)
	}
	defer engine.Stop()

	wf, err := engine.Enqueue(context.Background(), "TEST_WORKFLOW", "test-key-1", nil, nil, nil, shared.NewID())
	if err != nil {
		t.Fatalf("Enqueue: %v", err)
	}

	if len(executed) != 2 || executed[0] != "step-1" || executed[1] != "step-2" {
		t.Fatalf("expected steps to run in order, got %v", executed)
	}

	got, _, err := engine.Get(context.Background(), wf.ID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.Status != workflow.StatusSucceeded {
		t.Fatalf("expected workflow to succeed, got status %s (error: %s)", got.Status, got.Error)
	}
}

func TestEngine_Enqueue_IsIdempotent(t *testing.T) {
	repo := newFakeRepo()
	broker := inprocess.NewBroker()
	engine := workflows.NewEngine(repo, broker, nil)

	calls := 0
	engine.Register(workflows.Definition{
		Type: "TEST_WORKFLOW",
		Steps: []workflows.StepDefinition{
			{Name: "only-step", Run: func(_ context.Context, _ *workflow.Workflow) (map[string]any, error) {
				calls++
				return nil, nil
			}},
		},
	})
	if err := engine.StartConsuming(context.Background()); err != nil {
		t.Fatalf("StartConsuming: %v", err)
	}
	defer engine.Stop()

	first, err := engine.Enqueue(context.Background(), "TEST_WORKFLOW", "same-key", nil, nil, nil, shared.NewID())
	if err != nil {
		t.Fatalf("first Enqueue: %v", err)
	}
	second, err := engine.Enqueue(context.Background(), "TEST_WORKFLOW", "same-key", nil, nil, nil, shared.NewID())
	if err != nil {
		t.Fatalf("second Enqueue: %v", err)
	}

	if first.ID != second.ID {
		t.Fatalf("expected same workflow ID for repeated idempotency key, got %s and %s", first.ID, second.ID)
	}
	if calls != 1 {
		t.Fatalf("expected the step to run exactly once, ran %d times", calls)
	}
}

func TestEngine_StepFailure_MovesToNeedsAttention(t *testing.T) {
	repo := newFakeRepo()
	broker := inprocess.NewBroker()
	engine := workflows.NewEngine(repo, broker, nil)

	engine.Register(workflows.Definition{
		Type: "TEST_WORKFLOW",
		Steps: []workflows.StepDefinition{
			{Name: "failing-step", Run: func(_ context.Context, _ *workflow.Workflow) (map[string]any, error) {
				return nil, fmt.Errorf("boom")
			}},
		},
	})
	if err := engine.StartConsuming(context.Background()); err != nil {
		t.Fatalf("StartConsuming: %v", err)
	}
	defer engine.Stop()

	wf, err := engine.Enqueue(context.Background(), "TEST_WORKFLOW", "failing-key", nil, nil, nil, shared.NewID())
	if err != nil {
		t.Fatalf("Enqueue: %v", err)
	}

	got, _, err := engine.Get(context.Background(), wf.ID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.Status != workflow.StatusNeedsAttention {
		t.Fatalf("expected NEEDS_ATTENTION, got %s", got.Status)
	}

	// Sanity: time-related fields should have been stamped, not left zero.
	if got.StartedAt == nil || got.FinishedAt == nil {
		t.Fatal("expected StartedAt/FinishedAt to be set")
	}
	if got.FinishedAt.Before(*got.StartedAt) {
		t.Fatal("FinishedAt should not be before StartedAt")
	}
}

func TestEngine_ErrAwaitingApproval_PausesThenResumeReRunsSameStep(t *testing.T) {
	repo := newFakeRepo()
	broker := inprocess.NewBroker()
	engine := workflows.NewEngine(repo, broker, nil)

	var approved bool
	var attempts int
	engine.Register(workflows.Definition{
		Type: "TEST_WORKFLOW",
		Steps: []workflows.StepDefinition{
			{Name: "await-approval", Run: func(_ context.Context, _ *workflow.Workflow) (map[string]any, error) {
				attempts++
				if !approved {
					return nil, workflows.ErrAwaitingApproval
				}
				return map[string]any{"approved": true}, nil
			}},
			{Name: "after-approval", Run: func(_ context.Context, _ *workflow.Workflow) (map[string]any, error) {
				return nil, nil
			}},
		},
	})
	if err := engine.StartConsuming(context.Background()); err != nil {
		t.Fatalf("StartConsuming: %v", err)
	}
	defer engine.Stop()

	wf, err := engine.Enqueue(context.Background(), "TEST_WORKFLOW", "awaiting-key", nil, nil, nil, shared.NewID())
	if err != nil {
		t.Fatalf("Enqueue: %v", err)
	}

	got, steps, err := engine.Get(context.Background(), wf.ID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.Status != workflow.StatusAwaitingApproval {
		t.Fatalf("expected AWAITING_APPROVAL, got %s", got.Status)
	}
	if attempts != 1 {
		t.Fatalf("expected the step to have run once, ran %d times", attempts)
	}
	if len(steps) != 2 || steps[0].Status != workflow.StatusPending {
		t.Fatalf("expected the paused step to be reverted to PENDING (not FAILED), got %+v", steps[0])
	}

	// Resuming before the condition is actually satisfied should just pause
	// again, not panic or lose the workflow.
	approved = false
	if err := engine.Resume(context.Background(), wf.ID); err != nil {
		t.Fatalf("Resume (still not approved): %v", err)
	}
	if attempts != 2 {
		t.Fatalf("expected the step to have run twice, ran %d times", attempts)
	}

	approved = true
	if err := engine.Resume(context.Background(), wf.ID); err != nil {
		t.Fatalf("Resume: %v", err)
	}

	got, _, err = engine.Get(context.Background(), wf.ID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.Status != workflow.StatusSucceeded {
		t.Fatalf("expected the workflow to succeed after approval, got %s (error: %s)", got.Status, got.Error)
	}
	if attempts != 3 {
		t.Fatalf("expected the step to have run three times total, ran %d times", attempts)
	}
}

func TestEngine_Resume_RejectsWorkflowNotAwaitingApproval(t *testing.T) {
	repo := newFakeRepo()
	broker := inprocess.NewBroker()
	engine := workflows.NewEngine(repo, broker, nil)

	engine.Register(workflows.Definition{
		Type: "TEST_WORKFLOW",
		Steps: []workflows.StepDefinition{
			{Name: "only-step", Run: func(_ context.Context, _ *workflow.Workflow) (map[string]any, error) {
				return nil, nil
			}},
		},
	})
	if err := engine.StartConsuming(context.Background()); err != nil {
		t.Fatalf("StartConsuming: %v", err)
	}
	defer engine.Stop()

	wf, err := engine.Enqueue(context.Background(), "TEST_WORKFLOW", "already-done-key", nil, nil, nil, shared.NewID())
	if err != nil {
		t.Fatalf("Enqueue: %v", err)
	}

	if err := engine.Resume(context.Background(), wf.ID); err == nil {
		t.Fatal("expected Resume to reject a workflow that already succeeded")
	}
}
