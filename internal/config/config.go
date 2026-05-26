package config

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"unicode"

	"gopkg.in/yaml.v3"
)

type WindowSelection string
type ExistingPolicy string
type FailurePolicy string
type AfterExit string

const (
	WindowSelectionConfigured WindowSelection = "configured"
	WindowSelectionPrompt     WindowSelection = "prompt"

	ExistingPolicyAttach   ExistingPolicy = "attach"
	ExistingPolicyPrompt   ExistingPolicy = "prompt"
	ExistingPolicyRecreate ExistingPolicy = "recreate"

	FailurePolicyStop     FailurePolicy = "stop"
	FailurePolicyContinue FailurePolicy = "continue"

	AfterExitShell AfterExit = "shell"

	DefaultShell = "sh"
)

type Config struct {
	TmuxBinary string          `yaml:"tmux_binary,omitempty"`
	Tools      map[string]Tool `yaml:"tools"`
	Projects   []Project       `yaml:"projects"`
}

type Tool struct {
	Window                string            `yaml:"window,omitempty"`
	Command               string            `yaml:"command,omitempty"`
	AfterExit             AfterExit         `yaml:"after_exit,omitempty"`
	CWD                   string            `yaml:"cwd,omitempty"`
	Env                   map[string]string `yaml:"env,omitempty"`
	DefaultForNewProjects bool              `yaml:"default_for_new_projects,omitempty"`
}

type Project struct {
	Name            string          `yaml:"name"`
	Path            string          `yaml:"path"`
	Session         string          `yaml:"session,omitempty"`
	DefaultWindow   string          `yaml:"default_window,omitempty"`
	WindowSelection WindowSelection `yaml:"window_selection,omitempty"`
	OnExisting      ExistingPolicy  `yaml:"on_existing,omitempty"`
	ConfirmKill     *bool           `yaml:"confirm_kill,omitempty"`
	FailurePolicy   FailurePolicy   `yaml:"failure_policy,omitempty"`
	Tools           []ProjectTool   `yaml:"tools"`
}

type ProjectTool struct {
	Name     string
	Override ToolOverride
}

type ToolOverride struct {
	Window    string            `yaml:"window,omitempty"`
	Command   string            `yaml:"command,omitempty"`
	AfterExit AfterExit         `yaml:"after_exit,omitempty"`
	CWD       string            `yaml:"cwd,omitempty"`
	Env       map[string]string `yaml:"env,omitempty"`
	Enabled   *bool             `yaml:"enabled,omitempty"`
}

type ResolvedConfig struct {
	Tools    map[string]ResolvedTool
	Projects []ResolvedProject
}

type ResolvedTool struct {
	ID                    string
	Window                string
	Command               string
	AfterExit             AfterExit
	CWD                   string
	Env                   map[string]string
	DefaultForNewProjects bool
}

type ResolvedProject struct {
	Name            string
	Path            string
	Session         string
	DefaultWindow   string
	WindowSelection WindowSelection
	OnExisting      ExistingPolicy
	ConfirmKill     bool
	FailurePolicy   FailurePolicy
	Windows         []ResolvedWindow
}

type ResolvedWindow struct {
	ToolID       string
	Window       string
	CWD          string
	Command      string
	AfterExit    AfterExit
	Env          map[string]string
	ShellCommand string
}

type ResolveOptions struct {
	RequireExistingProjectPaths bool
	Shell                       string
}

type ValidationError struct {
	Problems []string
}

func (e *ValidationError) Error() string {
	return "config validation failed: " + strings.Join(e.Problems, "; ")
}

func LoadYAML(data []byte) (Config, error) {
	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return Config{}, err
	}
	if cfg.Tools == nil {
		cfg.Tools = map[string]Tool{}
	}
	return cfg, nil
}

func MarshalYAML(cfg Config) ([]byte, error) {
	return yaml.Marshal(cfg)
}

func Resolve(cfg Config, opts ResolveOptions) (ResolvedConfig, error) {
	var problems []string
	if cfg.Tools == nil {
		cfg.Tools = map[string]Tool{}
	}

	resolvedTools := make(map[string]ResolvedTool, len(cfg.Tools))
	toolNames := sortedKeys(cfg.Tools)
	for _, name := range toolNames {
		tool := cfg.Tools[name]
		if strings.TrimSpace(name) == "" {
			problems = append(problems, "tool name must not be empty")
			continue
		}
		rt := resolveTool(name, tool)
		if rt.Command == "" {
			problems = append(problems, fmt.Sprintf("tool %q command must not be empty", name))
		}
		if err := validateTmuxTargetName("tool "+strconvQuote(name)+" window", rt.Window); err != nil {
			problems = append(problems, err.Error())
		}
		if err := validateAfterExit(rt.AfterExit); err != nil {
			problems = append(problems, fmt.Sprintf("tool %q: %v", name, err))
		}
		resolvedTools[name] = rt
	}

	seenProjects := map[string]struct{}{}
	resolvedProjects := make([]ResolvedProject, 0, len(cfg.Projects))
	for i, project := range cfg.Projects {
		rp, projectProblems := resolveProject(project, i, resolvedTools, opts)
		problems = append(problems, projectProblems...)
		if rp.Name != "" {
			if _, exists := seenProjects[rp.Name]; exists {
				problems = append(problems, fmt.Sprintf("project %q is duplicated", rp.Name))
			}
			seenProjects[rp.Name] = struct{}{}
		}
		resolvedProjects = append(resolvedProjects, rp)
	}

	if len(problems) > 0 {
		return ResolvedConfig{}, &ValidationError{Problems: problems}
	}
	return ResolvedConfig{Tools: resolvedTools, Projects: resolvedProjects}, nil
}

func (pt *ProjectTool) UnmarshalYAML(value *yaml.Node) error {
	switch value.Kind {
	case yaml.ScalarNode:
		pt.Name = value.Value
		return nil
	case yaml.MappingNode:
		if len(value.Content) != 2 {
			return errors.New("project tool override must contain exactly one tool name")
		}
		pt.Name = value.Content[0].Value
		return value.Content[1].Decode(&pt.Override)
	default:
		return errors.New("project tool must be a tool name or single-key override map")
	}
}

func (pt ProjectTool) MarshalYAML() (any, error) {
	if pt.Override.isZero() {
		return pt.Name, nil
	}
	return map[string]ToolOverride{pt.Name: pt.Override}, nil
}

func resolveProject(project Project, index int, tools map[string]ResolvedTool, opts ResolveOptions) (ResolvedProject, []string) {
	var problems []string
	label := fmt.Sprintf("project[%d]", index)
	name := strings.TrimSpace(project.Name)
	if name == "" {
		problems = append(problems, label+" name must not be empty")
	}

	session := strings.TrimSpace(project.Session)
	if session == "" {
		session = name
	}
	if session == "" {
		problems = append(problems, label+" session must not be empty")
	} else if err := validateTmuxTargetName(label+" session", session); err != nil {
		problems = append(problems, err.Error())
	}

	projectPath, err := ExpandPath(project.Path)
	if err != nil {
		problems = append(problems, fmt.Sprintf("%s path: %v", label, err))
	}
	if projectPath == "" {
		problems = append(problems, fmt.Sprintf("%s path must not be empty", label))
	} else if opts.RequireExistingProjectPaths {
		if info, statErr := os.Stat(projectPath); statErr != nil {
			problems = append(problems, fmt.Sprintf("%s path %q does not exist", label, projectPath))
		} else if !info.IsDir() {
			problems = append(problems, fmt.Sprintf("%s path %q is not a directory", label, projectPath))
		}
	}

	windowSelection := project.WindowSelection
	if windowSelection == "" {
		windowSelection = WindowSelectionConfigured
	}
	if err := validateWindowSelection(windowSelection); err != nil {
		problems = append(problems, fmt.Sprintf("%s: %v", label, err))
	}

	onExisting := project.OnExisting
	if onExisting == "" {
		onExisting = ExistingPolicyAttach
	}
	if err := validateExistingPolicy(onExisting); err != nil {
		problems = append(problems, fmt.Sprintf("%s: %v", label, err))
	}

	failurePolicy := project.FailurePolicy
	if failurePolicy == "" {
		failurePolicy = FailurePolicyStop
	}
	if err := validateFailurePolicy(failurePolicy); err != nil {
		problems = append(problems, fmt.Sprintf("%s: %v", label, err))
	}

	confirmKill := true
	if project.ConfirmKill != nil {
		confirmKill = *project.ConfirmKill
	}

	windows := make([]ResolvedWindow, 0, len(project.Tools))
	for _, projectTool := range project.Tools {
		rw, ok, toolProblems := resolveProjectTool(projectTool, projectPath, tools, opts)
		problems = append(problems, prefixProblems(label+" tool "+strconvQuote(projectTool.Name), toolProblems)...)
		if ok {
			windows = append(windows, rw)
		}
	}

	if project.DefaultWindow != "" && windowSelection == WindowSelectionConfigured {
		if !containsWindow(windows, project.DefaultWindow) {
			problems = append(problems, fmt.Sprintf("%s default_window %q does not match a resolved window", label, project.DefaultWindow))
		}
	}

	return ResolvedProject{
		Name:            name,
		Path:            projectPath,
		Session:         session,
		DefaultWindow:   project.DefaultWindow,
		WindowSelection: windowSelection,
		OnExisting:      onExisting,
		ConfirmKill:     confirmKill,
		FailurePolicy:   failurePolicy,
		Windows:         windows,
	}, problems
}

func resolveProjectTool(projectTool ProjectTool, projectPath string, tools map[string]ResolvedTool, opts ResolveOptions) (ResolvedWindow, bool, []string) {
	var problems []string
	name := strings.TrimSpace(projectTool.Name)
	if name == "" {
		return ResolvedWindow{}, false, []string{"name must not be empty"}
	}
	if projectTool.Override.Enabled != nil && !*projectTool.Override.Enabled {
		return ResolvedWindow{}, false, nil
	}

	base, hasBase := tools[name]
	if !hasBase && strings.TrimSpace(projectTool.Override.Command) == "" {
		return ResolvedWindow{}, false, []string{"must reference a global tool or provide command override"}
	}
	merged := base
	if !hasBase {
		merged = ResolvedTool{ID: name, Window: name, AfterExit: AfterExitShell}
	}
	merged = applyOverride(merged, projectTool.Override)
	if merged.Window == "" {
		merged.Window = name
	}
	if err := validateTmuxTargetName("window", merged.Window); err != nil {
		problems = append(problems, err.Error())
	}
	if merged.Command == "" {
		problems = append(problems, "command must not be empty")
	}
	if merged.AfterExit == "" {
		merged.AfterExit = AfterExitShell
	}
	if err := validateAfterExit(merged.AfterExit); err != nil {
		problems = append(problems, err.Error())
	}
	cwd := merged.CWD
	if cwd == "" {
		cwd = projectPath
	}
	expandedCWD, err := ExpandPath(cwd)
	if err != nil {
		problems = append(problems, "cwd: "+err.Error())
	}
	return ResolvedWindow{
		ToolID:       name,
		Window:       merged.Window,
		CWD:          expandedCWD,
		Command:      merged.Command,
		AfterExit:    merged.AfterExit,
		Env:          cloneMap(merged.Env),
		ShellCommand: BuildShellCommandWithShell(merged.Command, merged.AfterExit, opts.Shell),
	}, len(problems) == 0, problems
}

func resolveTool(id string, tool Tool) ResolvedTool {
	window := strings.TrimSpace(tool.Window)
	if window == "" {
		window = id
	}
	afterExit := tool.AfterExit
	if afterExit == "" {
		afterExit = AfterExitShell
	}
	return ResolvedTool{
		ID:                    id,
		Window:                window,
		Command:               strings.TrimSpace(tool.Command),
		AfterExit:             afterExit,
		CWD:                   tool.CWD,
		Env:                   cloneMap(tool.Env),
		DefaultForNewProjects: tool.DefaultForNewProjects,
	}
}

func applyOverride(base ResolvedTool, override ToolOverride) ResolvedTool {
	if override.Window != "" {
		base.Window = override.Window
	}
	if override.Command != "" {
		base.Command = override.Command
	}
	if override.AfterExit != "" {
		base.AfterExit = override.AfterExit
	}
	if override.CWD != "" {
		base.CWD = override.CWD
	}
	if override.Env != nil {
		base.Env = cloneMap(override.Env)
	}
	return base
}

func BuildShellCommand(command string, afterExit AfterExit) string {
	return BuildShellCommandWithShell(command, afterExit, DefaultShell)
}

func BuildShellCommandWithShell(command string, afterExit AfterExit, shell string) string {
	shell = strings.TrimSpace(shell)
	if shell == "" {
		shell = DefaultShell
	}
	switch afterExit {
	case "", AfterExitShell:
		return fmt.Sprintf("%s -lc %q", quoteShellWord(shell), command+"; exec "+quoteShellWord(shell))
	default:
		return fmt.Sprintf("%s -lc %q", quoteShellWord(shell), command)
	}
}

func ResolveShellPath() (string, error) {
	return ResolveShellPathFromEnv(os.Getenv("SHELL"))
}

func ResolveShellPathFromEnv(shell string) (string, error) {
	shell = strings.TrimSpace(shell)
	if shell != "" {
		if path, err := exec.LookPath(shell); err == nil {
			return path, nil
		}
	}
	path, err := exec.LookPath(DefaultShell)
	if err != nil {
		return "", fmt.Errorf("shell not found: %w", err)
	}
	return path, nil
}

func ExpandPath(path string) (string, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return "", nil
	}
	if path == "~" || strings.HasPrefix(path, "~/") {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		if path == "~" {
			return home, nil
		}
		path = filepath.Join(home, strings.TrimPrefix(path, "~/"))
	}
	return filepath.Clean(path), nil
}

func validateWindowSelection(value WindowSelection) error {
	switch value {
	case WindowSelectionConfigured, WindowSelectionPrompt:
		return nil
	default:
		return fmt.Errorf("invalid window_selection %q", value)
	}
}

func validateExistingPolicy(value ExistingPolicy) error {
	switch value {
	case ExistingPolicyAttach, ExistingPolicyPrompt, ExistingPolicyRecreate:
		return nil
	default:
		return fmt.Errorf("invalid on_existing %q", value)
	}
}

func validateFailurePolicy(value FailurePolicy) error {
	switch value {
	case FailurePolicyStop, FailurePolicyContinue:
		return nil
	default:
		return fmt.Errorf("invalid failure_policy %q", value)
	}
}

func validateAfterExit(value AfterExit) error {
	switch value {
	case AfterExitShell:
		return nil
	default:
		return fmt.Errorf("invalid after_exit %q", value)
	}
}

func validateTmuxTargetName(label, value string) error {
	if strings.TrimSpace(value) == "" {
		return fmt.Errorf("%s must not be empty", label)
	}
	for _, r := range value {
		if r == ':' || unicode.IsControl(r) {
			return fmt.Errorf("%s %q must not contain ':' or control characters", label, value)
		}
	}
	return nil
}

func containsWindow(windows []ResolvedWindow, window string) bool {
	for _, candidate := range windows {
		if candidate.Window == window {
			return true
		}
	}
	return false
}

func sortedKeys[T any](m map[string]T) []string {
	keys := make([]string, 0, len(m))
	for key := range m {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func cloneMap(in map[string]string) map[string]string {
	if len(in) == 0 {
		return nil
	}
	out := make(map[string]string, len(in))
	for key, value := range in {
		out[key] = value
	}
	return out
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

func prefixProblems(prefix string, problems []string) []string {
	if len(problems) == 0 {
		return nil
	}
	out := make([]string, len(problems))
	for i, problem := range problems {
		out[i] = prefix + ": " + problem
	}
	return out
}

func strconvQuote(value string) string {
	if value == "" {
		return "<empty>"
	}
	return fmt.Sprintf("%q", value)
}

func (o ToolOverride) isZero() bool {
	return o.Window == "" && o.Command == "" && o.AfterExit == "" && o.CWD == "" && o.Env == nil && o.Enabled == nil
}
