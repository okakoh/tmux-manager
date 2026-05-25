package runner

import (
	"context"
	"errors"
	"testing"

	"github.com/okakoh/tmux-manager/internal/config"
	"github.com/okakoh/tmux-manager/internal/planner"
)

func TestFailurePolicyStopStopsAtFirstFailure(t *testing.T) {
	exec := &fakeExecutor{failAt: map[planner.StepKind]error{planner.StepNewWindow: errors.New("new-window failed")}}
	plan := testPlan(config.FailurePolicyStop)
	result := New(exec).Run(context.Background(), plan)
	if result.Err == nil {
		t.Fatal("Run() expected error")
	}
	if len(exec.steps) != 2 {
		t.Fatalf("executed steps = %d, want 2", len(exec.steps))
	}
}

func TestFailurePolicyContinueContinuesNonFinalWindowFailure(t *testing.T) {
	exec := &fakeExecutor{failAt: map[planner.StepKind]error{planner.StepNewWindow: errors.New("new-window failed")}}
	plan := testPlan(config.FailurePolicyContinue)
	result := New(exec).Run(context.Background(), plan)
	if result.Err == nil {
		t.Fatal("Run() should report partial failure")
	}
	if !result.PartialSuccess {
		t.Fatal("Run() should mark partial success")
	}
	if len(exec.steps) != len(plan.Steps) {
		t.Fatalf("executed steps = %d, want %d", len(exec.steps), len(plan.Steps))
	}
}

func TestAttachFailureIsAlwaysSurfaced(t *testing.T) {
	exec := &fakeExecutor{failAt: map[planner.StepKind]error{planner.StepAttach: errors.New("attach failed")}}
	plan := testPlan(config.FailurePolicyContinue)
	result := New(exec).Run(context.Background(), plan)
	if result.Err == nil {
		t.Fatal("Run() expected attach error")
	}
	if result.PartialSuccess {
		t.Fatal("attach failure should not be partial success")
	}
	if len(exec.steps) != len(plan.Steps) {
		t.Fatalf("executed steps = %d, want %d", len(exec.steps), len(plan.Steps))
	}
}

type fakeExecutor struct {
	failAt map[planner.StepKind]error
	steps  []planner.PlanStep
}

func (f *fakeExecutor) ExecuteStep(_ context.Context, step planner.PlanStep) error {
	f.steps = append(f.steps, step)
	if err := f.failAt[step.Kind]; err != nil {
		return err
	}
	return nil
}

func testPlan(policy config.FailurePolicy) planner.ActionPlan {
	return planner.ActionPlan{
		ProjectName:   "sample-api",
		SessionName:   "sample-api",
		Kind:          planner.PlanLaunch,
		FailurePolicy: policy,
		Steps: []planner.PlanStep{
			{Kind: planner.StepNewSession, TargetSession: "sample-api", TargetWindow: "yazi"},
			{Kind: planner.StepNewWindow, TargetSession: "sample-api", TargetWindow: "codex"},
			{Kind: planner.StepSelectWindow, TargetSession: "sample-api", TargetWindow: "codex"},
			{Kind: planner.StepAttach, TargetSession: "sample-api"},
		},
	}
}
