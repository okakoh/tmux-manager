package planner

import (
	"testing"

	"github.com/okakoh/tmux-manager/internal/config"
	"github.com/okakoh/tmux-manager/internal/tmux"
)

func TestMissingSessionProducesLaunchPlan(t *testing.T) {
	project := testProject(config.ExistingPolicyAttach, config.WindowSelectionConfigured, true)
	plan, err := Plan(project, tmux.State{}, Request{})
	if err != nil {
		t.Fatalf("Plan() error = %v", err)
	}
	if plan.Kind != PlanLaunch {
		t.Fatalf("Kind = %q, want %q", plan.Kind, PlanLaunch)
	}
	assertStepKinds(t, plan.Steps, []StepKind{StepNewSession, StepNewWindow, StepSelectWindow, StepAttach})
	if plan.Steps[2].TargetWindow != "codex" {
		t.Fatalf("selected window = %q, want codex", plan.Steps[2].TargetWindow)
	}
}

func TestExistingSessionAttachProducesSelectAndAttach(t *testing.T) {
	project := testProject(config.ExistingPolicyAttach, config.WindowSelectionConfigured, true)
	plan, err := Plan(project, stateWithSession("sample-api"), Request{})
	if err != nil {
		t.Fatalf("Plan() error = %v", err)
	}
	if plan.Kind != PlanAttach {
		t.Fatalf("Kind = %q, want %q", plan.Kind, PlanAttach)
	}
	assertStepKinds(t, plan.Steps, []StepKind{StepSelectWindow, StepAttach})
}

func TestExistingSessionRecreateProducesDestructiveLaunch(t *testing.T) {
	project := testProject(config.ExistingPolicyRecreate, config.WindowSelectionConfigured, true)
	plan, err := Plan(project, stateWithSession("sample-api"), Request{})
	if err != nil {
		t.Fatalf("Plan() error = %v", err)
	}
	if plan.Kind != PlanRecreate {
		t.Fatalf("Kind = %q, want %q", plan.Kind, PlanRecreate)
	}
	if !plan.RequiresConfirmation {
		t.Fatal("RequiresConfirmation should be true")
	}
	assertStepKinds(t, plan.Steps, []StepKind{StepKillSession, StepNewSession, StepNewWindow, StepSelectWindow, StepAttach})
	if !plan.Steps[0].Destructive {
		t.Fatal("kill step should be destructive")
	}
}

func TestExistingSessionPromptReturnsPromptPlan(t *testing.T) {
	project := testProject(config.ExistingPolicyPrompt, config.WindowSelectionConfigured, true)
	plan, err := Plan(project, stateWithSession("sample-api"), Request{})
	if err != nil {
		t.Fatalf("Plan() error = %v", err)
	}
	if plan.Prompt != PromptExisting {
		t.Fatalf("Prompt = %q, want %q", plan.Prompt, PromptExisting)
	}
	if len(plan.Steps) != 0 {
		t.Fatalf("Steps = %#v, want empty", plan.Steps)
	}
}

func TestWindowSelectionPromptBlocksUntilSelected(t *testing.T) {
	project := testProject(config.ExistingPolicyAttach, config.WindowSelectionPrompt, true)
	plan, err := Plan(project, tmux.State{}, Request{})
	if err != nil {
		t.Fatalf("Plan() error = %v", err)
	}
	if plan.Prompt != PromptWindow {
		t.Fatalf("Prompt = %q, want %q", plan.Prompt, PromptWindow)
	}

	plan, err = Plan(project, tmux.State{}, Request{SelectedWindow: "yazi"})
	if err != nil {
		t.Fatalf("Plan() with selection error = %v", err)
	}
	if plan.Prompt != PromptNone {
		t.Fatalf("Prompt = %q, want none", plan.Prompt)
	}
	if plan.Steps[len(plan.Steps)-2].TargetWindow != "yazi" {
		t.Fatalf("selected window = %q, want yazi", plan.Steps[len(plan.Steps)-2].TargetWindow)
	}
}

func TestConfirmKillControlsConfirmation(t *testing.T) {
	project := testProject(config.ExistingPolicyAttach, config.WindowSelectionConfigured, false)
	plan, err := Plan(project, stateWithSession("sample-api"), Request{Action: ActionKill})
	if err != nil {
		t.Fatalf("Plan() error = %v", err)
	}
	if plan.RequiresConfirmation {
		t.Fatal("RequiresConfirmation should follow ConfirmKill=false")
	}
}

func testProject(existing config.ExistingPolicy, selection config.WindowSelection, confirmKill bool) config.ResolvedProject {
	return config.ResolvedProject{
		Name:            "sample-api",
		Path:            "/Users/me/sample-api",
		Session:         "sample-api",
		DefaultWindow:   "codex",
		WindowSelection: selection,
		OnExisting:      existing,
		ConfirmKill:     confirmKill,
		FailurePolicy:   config.FailurePolicyStop,
		Windows: []config.ResolvedWindow{
			{ToolID: "yazi", Window: "yazi", CWD: "/Users/me/sample-api", ShellCommand: `zsh -lc "yazi; exec zsh"`},
			{ToolID: "codex", Window: "codex", CWD: "/Users/me/sample-api", ShellCommand: `zsh -lc "codex; exec zsh"`},
		},
	}
}

func stateWithSession(name string) tmux.State {
	return tmux.State{Sessions: []tmux.Session{{Name: name, WindowCount: 2}}}
}

func assertStepKinds(t *testing.T, steps []PlanStep, want []StepKind) {
	t.Helper()
	if len(steps) != len(want) {
		t.Fatalf("len(steps) = %d, want %d: %#v", len(steps), len(want), steps)
	}
	for i := range want {
		if steps[i].Kind != want[i] {
			t.Fatalf("steps[%d].Kind = %q, want %q", i, steps[i].Kind, want[i])
		}
	}
}
