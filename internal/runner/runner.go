package runner

import (
	"context"
	"fmt"

	"github.com/okakoh/tmux-manager/internal/config"
	"github.com/okakoh/tmux-manager/internal/planner"
	"github.com/okakoh/tmux-manager/internal/tmux"
)

type Executor interface {
	ExecuteStep(context.Context, planner.PlanStep) error
}

type TmuxExecutor struct {
	Client tmux.Client
}

type Runner struct {
	Executor Executor
}

type StepResult struct {
	Step planner.PlanStep
	Err  error
}

type Result struct {
	Steps          []StepResult
	PartialSuccess bool
	Err            error
}

func New(executor Executor) Runner {
	return Runner{Executor: executor}
}

func (r Runner) Run(ctx context.Context, plan planner.ActionPlan) Result {
	result := Result{Steps: make([]StepResult, 0, len(plan.Steps))}
	if r.Executor == nil {
		result.Err = fmt.Errorf("runner executor is nil")
		return result
	}

	for i, step := range plan.Steps {
		err := r.Executor.ExecuteStep(ctx, step)
		result.Steps = append(result.Steps, StepResult{Step: step, Err: err})
		if err == nil {
			continue
		}
		result.Err = err

		if shouldContinue(plan.FailurePolicy, step, i, len(plan.Steps)) {
			result.PartialSuccess = true
			continue
		}
		return result
	}
	return result
}

func (e TmuxExecutor) ExecuteStep(ctx context.Context, step planner.PlanStep) error {
	switch step.Kind {
	case planner.StepNewSession:
		return e.Client.NewSession(ctx, step.TargetSession, step.TargetWindow, step.CWD, step.ShellCommand)
	case planner.StepNewWindow:
		return e.Client.NewWindow(ctx, step.TargetSession, step.TargetWindow, step.CWD, step.ShellCommand)
	case planner.StepSelectWindow:
		return e.Client.SelectWindow(ctx, step.TargetSession, step.TargetWindow)
	case planner.StepAttach:
		return e.Client.AttachSession(ctx, step.TargetSession)
	case planner.StepKillSession:
		return e.Client.KillSession(ctx, step.TargetSession)
	default:
		return fmt.Errorf("unsupported plan step %q", step.Kind)
	}
}

func shouldContinue(policy config.FailurePolicy, step planner.PlanStep, index, total int) bool {
	if policy != config.FailurePolicyContinue {
		return false
	}
	if step.Kind == planner.StepAttach {
		return false
	}
	if step.Kind != planner.StepNewWindow {
		return false
	}
	return index < total-1
}
