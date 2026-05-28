package planner

import (
	"errors"
	"fmt"
	"strings"

	"github.com/okakoh/tmux-manager/internal/config"
	"github.com/okakoh/tmux-manager/internal/tmux"
)

type Action string
type PlanKind string
type StepKind string
type PromptKind string

const (
	ActionLaunchOrAttach Action = "launch-or-attach"
	ActionRecreate       Action = "recreate"
	ActionKill           Action = "kill"
	ActionSelectWindow   Action = "select-window"

	PlanLaunch       PlanKind = "launch"
	PlanAttach       PlanKind = "attach"
	PlanRecreate     PlanKind = "recreate"
	PlanKill         PlanKind = "kill"
	PlanSelectWindow PlanKind = "select-window"

	StepNewSession   StepKind = "new-session"
	StepNewWindow    StepKind = "new-window"
	StepSelectWindow StepKind = "select-window"
	StepAttach       StepKind = "attach"
	StepKillSession  StepKind = "kill-session"

	PromptNone     PromptKind = ""
	PromptExisting PromptKind = "existing-session"
	PromptWindow   PromptKind = "window"
)

type Request struct {
	Action         Action
	SelectedWindow string
}

type ActionPlan struct {
	ProjectName          string
	SessionName          string
	Kind                 PlanKind
	Prompt               PromptKind
	RequiresConfirmation bool
	FailurePolicy        config.FailurePolicy
	Steps                []PlanStep
}

type PlanStep struct {
	Kind           StepKind
	Description    string
	ToolID         string
	TargetSession  string
	TargetWindow   string
	CWD            string
	Command        string
	Env            map[string]string
	Shell          string
	ShellCommand   string
	CommandPreview []string
	Destructive    bool
}

func Plan(project config.ResolvedProject, state tmux.State, request Request) (ActionPlan, error) {
	action := request.Action
	if action == "" {
		action = ActionLaunchOrAttach
	}
	base := ActionPlan{
		ProjectName:   project.Name,
		SessionName:   project.Session,
		FailurePolicy: project.FailurePolicy,
	}

	switch action {
	case ActionKill:
		base.Kind = PlanKill
		base.RequiresConfirmation = project.ConfirmKill
		base.Steps = []PlanStep{killStep(project.Session)}
		return base, nil
	case ActionRecreate:
		base.Kind = PlanRecreate
		return recreatePlan(base, project, request.SelectedWindow)
	case ActionSelectWindow:
		base.Kind = PlanSelectWindow
		if request.SelectedWindow == "" {
			base.Prompt = PromptWindow
			return base, nil
		}
		base.Steps = []PlanStep{selectWindowStep(project.Session, request.SelectedWindow)}
		return base, nil
	case ActionLaunchOrAttach:
		return launchOrAttachPlan(base, project, state, request.SelectedWindow)
	default:
		return ActionPlan{}, fmt.Errorf("unknown planner action %q", action)
	}
}

func launchOrAttachPlan(base ActionPlan, project config.ResolvedProject, state tmux.State, selectedWindow string) (ActionPlan, error) {
	exists := state.HasSession(project.Session)
	if exists {
		switch project.OnExisting {
		case config.ExistingPolicyAttach:
			base.Kind = PlanAttach
			return attachPlan(base, project, selectedWindow)
		case config.ExistingPolicyRecreate:
			base.Kind = PlanRecreate
			return recreatePlan(base, project, selectedWindow)
		case config.ExistingPolicyPrompt:
			base.Kind = PlanAttach
			base.Prompt = PromptExisting
			return base, nil
		default:
			return ActionPlan{}, fmt.Errorf("unsupported existing-session policy %q", project.OnExisting)
		}
	}
	base.Kind = PlanLaunch
	return launchPlan(base, project, selectedWindow)
}

func recreatePlan(base ActionPlan, project config.ResolvedProject, selectedWindow string) (ActionPlan, error) {
	launch, err := launchPlan(base, project, selectedWindow)
	if err != nil || launch.Prompt != PromptNone {
		return launch, err
	}
	launch.Kind = PlanRecreate
	launch.RequiresConfirmation = project.ConfirmKill
	launch.Steps = append([]PlanStep{killStep(project.Session)}, launch.Steps...)
	return launch, nil
}

func attachPlan(base ActionPlan, project config.ResolvedProject, selectedWindow string) (ActionPlan, error) {
	target, prompt, err := targetWindow(project, selectedWindow)
	if err != nil {
		return ActionPlan{}, err
	}
	if prompt {
		base.Prompt = PromptWindow
		return base, nil
	}
	base.Steps = []PlanStep{selectWindowStep(project.Session, target), attachStep(project.Session)}
	return base, nil
}

func launchPlan(base ActionPlan, project config.ResolvedProject, selectedWindow string) (ActionPlan, error) {
	if len(project.Windows) == 0 {
		return ActionPlan{}, errors.New("project has no windows to launch")
	}
	target, prompt, err := targetWindow(project, selectedWindow)
	if err != nil {
		return ActionPlan{}, err
	}
	if prompt {
		base.Prompt = PromptWindow
		return base, nil
	}

	steps := make([]PlanStep, 0, len(project.Windows)+2)
	for i, window := range project.Windows {
		if i == 0 {
			steps = append(steps, newSessionStep(project.Session, window))
			continue
		}
		steps = append(steps, newWindowStep(project.Session, window))
	}
	steps = append(steps, selectWindowStep(project.Session, target), attachStep(project.Session))
	base.Steps = steps
	return base, nil
}

func targetWindow(project config.ResolvedProject, selectedWindow string) (string, bool, error) {
	if selectedWindow != "" {
		return selectedWindow, false, nil
	}
	if project.WindowSelection == config.WindowSelectionPrompt {
		return "", true, nil
	}
	if project.DefaultWindow != "" {
		return project.DefaultWindow, false, nil
	}
	if len(project.Windows) == 0 {
		return "", false, errors.New("project has no windows")
	}
	return project.Windows[0].Window, false, nil
}

func newSessionStep(session string, window config.ResolvedWindow) PlanStep {
	args := tmux.NewSessionArgs(session, window.Window, window.CWD, window.ShellCommand)
	return PlanStep{
		Kind:           StepNewSession,
		Description:    fmt.Sprintf("Create tmux session %q with window %q", session, window.Window),
		ToolID:         window.ToolID,
		TargetSession:  session,
		TargetWindow:   window.Window,
		CWD:            window.CWD,
		Command:        window.Command,
		Env:            window.Env,
		Shell:          window.Shell,
		ShellCommand:   window.ShellCommand,
		CommandPreview: args,
	}
}

func newWindowStep(session string, window config.ResolvedWindow) PlanStep {
	args := tmux.NewWindowArgs(session, window.Window, window.CWD, window.ShellCommand)
	return PlanStep{
		Kind:           StepNewWindow,
		Description:    fmt.Sprintf("Create tmux window %q", window.Window),
		ToolID:         window.ToolID,
		TargetSession:  session,
		TargetWindow:   window.Window,
		CWD:            window.CWD,
		Command:        window.Command,
		Env:            window.Env,
		Shell:          window.Shell,
		ShellCommand:   window.ShellCommand,
		CommandPreview: args,
	}
}

func selectWindowStep(session, window string) PlanStep {
	args := tmux.SelectWindowArgs(session, window)
	return PlanStep{
		Kind:           StepSelectWindow,
		Description:    fmt.Sprintf("Select window %q", window),
		TargetSession:  session,
		TargetWindow:   window,
		CommandPreview: args,
	}
}

func attachStep(session string) PlanStep {
	args := tmux.AttachSessionArgs(session)
	return PlanStep{
		Kind:           StepAttach,
		Description:    fmt.Sprintf("Attach tmux session %q", session),
		TargetSession:  session,
		CommandPreview: args,
	}
}

func killStep(session string) PlanStep {
	args := tmux.KillSessionArgs(session)
	return PlanStep{
		Kind:           StepKillSession,
		Description:    fmt.Sprintf("Kill tmux session %q", session),
		TargetSession:  session,
		CommandPreview: args,
		Destructive:    true,
	}
}

func PreviewString(args []string) string {
	return "tmux " + strings.Join(args, " ")
}
