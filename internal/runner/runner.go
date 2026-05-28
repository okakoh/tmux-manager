package runner

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"sort"
	"strings"

	"github.com/okakoh/tmux-manager/internal/config"
	"github.com/okakoh/tmux-manager/internal/planner"
	"github.com/okakoh/tmux-manager/internal/tmux"
)

type Executor interface {
	ExecuteStep(context.Context, planner.PlanStep) error
}

type TmuxExecutor struct {
	Client tmux.Client
	Shell  string
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
		if err := e.preflightCommand(ctx, step); err != nil {
			return err
		}
		return e.Client.NewSession(ctx, step.TargetSession, step.TargetWindow, step.CWD, step.ShellCommand)
	case planner.StepNewWindow:
		if err := e.preflightCommand(ctx, step); err != nil {
			return err
		}
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

type MissingCommandError struct {
	ToolID     string
	Window     string
	Command    string
	Executable string
	Shell      string
}

func (e *MissingCommandError) Error() string {
	target := e.ToolID
	if target == "" {
		target = e.Window
	}
	return fmt.Sprintf("tool %q command %q cannot start: executable %q was not found in the configured shell environment. Use an absolute command path or set env.PATH for the tool.", target, e.Command, e.Executable)
}

func (e TmuxExecutor) preflightCommand(ctx context.Context, step planner.PlanStep) error {
	executable, ok := simpleExecutable(step.Command)
	if !ok {
		return nil
	}
	shell := e.Shell
	if shell == "" {
		shell = step.Shell
	}
	if shell == "" {
		shell = config.DefaultShell
	}
	cmd := exec.CommandContext(ctx, shell, "-lc", exportScript(step.Env)+"command -v "+quoteShellWord(executable))
	cmd.Env = envList(step.Env)
	if err := cmd.Run(); err != nil {
		return &MissingCommandError{
			ToolID:     step.ToolID,
			Window:     step.TargetWindow,
			Command:    step.Command,
			Executable: executable,
			Shell:      shell,
		}
	}
	return nil
}

func simpleExecutable(command string) (string, bool) {
	command = strings.TrimSpace(command)
	if command == "" || strings.ContainsAny(command, "\n\r;&|()<>`") {
		return "", false
	}
	fields := strings.Fields(command)
	for len(fields) > 0 {
		word := trimSimpleQuotes(fields[0])
		fields = fields[1:]
		if word == "" || isEnvAssignment(word) {
			continue
		}
		if word == "env" || word == "command" || word == "exec" {
			continue
		}
		return word, true
	}
	return "", false
}

func isEnvAssignment(value string) bool {
	name, _, ok := strings.Cut(value, "=")
	if !ok || name == "" {
		return false
	}
	for i, r := range name {
		if r == '_' || (r >= 'A' && r <= 'Z') || (r >= 'a' && r <= 'z') {
			continue
		}
		if i > 0 && r >= '0' && r <= '9' {
			continue
		}
		return false
	}
	return true
}

func trimSimpleQuotes(value string) string {
	if len(value) >= 2 {
		if (value[0] == '\'' && value[len(value)-1] == '\'') || (value[0] == '"' && value[len(value)-1] == '"') {
			return value[1 : len(value)-1]
		}
	}
	return value
}

func envList(overrides map[string]string) []string {
	env := os.Environ()
	if len(overrides) == 0 {
		return env
	}
	for _, key := range sortedKeys(overrides) {
		env = append(env, key+"="+overrides[key])
	}
	return env
}

func exportScript(env map[string]string) string {
	if len(env) == 0 {
		return ""
	}
	var parts []string
	for _, key := range sortedKeys(env) {
		parts = append(parts, "export "+key+"="+quoteShellWord(env[key]))
	}
	return strings.Join(parts, "; ") + "; "
}

func sortedKeys(in map[string]string) []string {
	keys := make([]string, 0, len(in))
	for key := range in {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func quoteShellWord(value string) string {
	if value == "" {
		return "''"
	}
	if isSafeShellWord(value) {
		return value
	}
	return "'" + strings.ReplaceAll(value, "'", "'\\''") + "'"
}

func isSafeShellWord(value string) bool {
	for _, r := range value {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') {
			continue
		}
		switch r {
		case '/', '.', '_', '-', '+':
			continue
		default:
			return false
		}
	}
	return true
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
