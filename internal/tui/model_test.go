package tui

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/okakoh/tmux-manager/internal/config"
	"github.com/okakoh/tmux-manager/internal/planner"
	"github.com/okakoh/tmux-manager/internal/runner"
	"github.com/okakoh/tmux-manager/internal/storage"
	"github.com/okakoh/tmux-manager/internal/tmux"
)

func TestHomeDisplaysConfiguredProjects(t *testing.T) {
	m := testModel(120, tmux.State{})
	view := m.View()
	for _, want := range []string{"tmux-manager", "sample-api", "notes"} {
		if !strings.Contains(view, want) {
			t.Fatalf("View() missing %q:\n%s", want, view)
		}
	}
}

func TestWideLayoutShowsListAndDetailTogether(t *testing.T) {
	m := testModel(120, tmux.State{})
	view := m.View()
	if !strings.Contains(view, "enter launch/attach") || !strings.Contains(view, "windows") {
		t.Fatalf("wide View() missing list/detail content:\n%s", view)
	}
}

func TestNarrowEnterNavigatesToProjectDetail(t *testing.T) {
	m := testModel(80, tmux.State{})
	next, _ := m.Update(key("enter"))
	got := next.(Model)
	if got.screen != ScreenProjectDetail {
		t.Fatalf("screen = %q, want %q", got.screen, ScreenProjectDetail)
	}
}

func TestHomeKeyTransitions(t *testing.T) {
	m := testModel(120, tmux.State{})
	next, _ := m.Update(key("down"))
	m = next.(Model)
	if m.selected != 1 {
		t.Fatalf("selected = %d, want 1", m.selected)
	}
	next, _ = m.Update(key("/"))
	m = next.(Model)
	if !m.searching {
		t.Fatal("slash should enter search")
	}
	next, _ = m.Update(key("h"))
	m = next.(Model)
	if !m.searching || m.search != "h" {
		t.Fatalf("search state = %v %q, want active search h", m.searching, m.search)
	}

	m.searching = false
	next, _ = m.Update(key("?"))
	m = next.(Model)
	if m.screen != ScreenHelp {
		t.Fatalf("screen = %q, want help", m.screen)
	}
	next, _ = m.Update(key("esc"))
	m = next.(Model)
	if m.screen != ScreenHome {
		t.Fatalf("screen = %q, want home", m.screen)
	}
}

func TestDestructivePlansShowConfirmation(t *testing.T) {
	m := testModel(120, tmux.State{Sessions: []tmux.Session{{Name: "sample-api"}}})
	next, cmd := m.Update(key("k"))
	got := next.(Model)
	if cmd != nil {
		t.Fatal("kill should wait for confirmation")
	}
	if got.screen != ScreenConfirm {
		t.Fatalf("screen = %q, want confirm", got.screen)
	}
}

func TestWindowPickerSelectsOverride(t *testing.T) {
	m := testModel(120, tmux.State{})
	next, _ := m.Update(key("w"))
	m = next.(Model)
	if m.screen != ScreenWindowPicker {
		t.Fatalf("screen = %q, want window picker", m.screen)
	}
	next, _ = m.Update(key("down"))
	m = next.(Model)
	next, _ = m.Update(key("enter"))
	m = next.(Model)
	if m.windowOverride != "codex" {
		t.Fatalf("windowOverride = %q, want codex", m.windowOverride)
	}
}

func TestRunnerErrorsSurfaceInErrorScreen(t *testing.T) {
	exec := &fakeExec{result: runner.Result{Err: errors.New("tmux failed")}}
	m := testModelWithExecutor(120, tmux.State{}, exec)
	next, cmd := m.Update(key("enter"))
	if cmd == nil {
		t.Fatal("enter should return execution command")
	}
	msg := cmd()
	next, _ = next.(Model).Update(msg)
	got := next.(Model)
	if got.screen != ScreenError {
		t.Fatalf("screen = %q, want error", got.screen)
	}
	if got.err == nil || !strings.Contains(got.err.Error(), "tmux failed") {
		t.Fatalf("err = %v, want tmux failed", got.err)
	}
}

func TestExecutePlanSplitsInteractiveAttach(t *testing.T) {
	exec := &fakeExec{}
	m := testModelWithExecutor(120, tmux.State{}, exec)
	plan := planner.ActionPlan{
		SessionName: "sample-api",
		Steps: []planner.PlanStep{
			{Kind: planner.StepNewSession, TargetSession: "sample-api", TargetWindow: "yazi"},
			{Kind: planner.StepAttach, TargetSession: "sample-api"},
		},
	}

	msg := m.executePlan(plan)()
	if _, ok := msg.(attachReadyMsg); !ok {
		t.Fatalf("executePlan() msg = %T, want attachReadyMsg", msg)
	}
	if len(exec.plans) != 1 {
		t.Fatalf("executed plans = %d, want 1", len(exec.plans))
	}
	if got := exec.plans[0].Steps; len(got) != 1 || got[0].Kind != planner.StepNewSession {
		t.Fatalf("pre-attach steps = %#v, want only new-session", got)
	}
}

func TestEnterSessionArgsSwitchesClientInsideTmux(t *testing.T) {
	t.Setenv("TMUX", "/private/tmp/tmux-501/default,123,0")
	m := testModel(120, tmux.State{})
	got := m.enterSessionArgs("sample-api")
	want := []string{"-u", "switch-client", "-t", "sample-api"}
	if strings.Join(got, "\x00") != strings.Join(want, "\x00") {
		t.Fatalf("enterSessionArgs() = %#v, want %#v", got, want)
	}
}

func TestAttachFinishedErrorIncludesTmuxCommand(t *testing.T) {
	m := testModel(120, tmux.State{})
	next, _ := m.Update(attachFinishedMsg{
		args: []string{"tmux", "-u", "attach-session", "-d", "-t", "sample-api"},
		err:  errors.New("exit status 1"),
	})
	got := next.(Model)
	if got.screen != ScreenError {
		t.Fatalf("screen = %q, want error", got.screen)
	}
	for _, want := range []string{"tmux command-failed", "attach-session", "sample-api", "exit status 1"} {
		if got.err == nil || !strings.Contains(got.err.Error(), want) {
			t.Fatalf("err = %v, want %q", got.err, want)
		}
	}
}

func TestSettingsEditorCanEditProjectField(t *testing.T) {
	raw, resolved := testConfig(t, t.TempDir())
	m := NewModelWithServices(raw, resolved, tmux.State{}, &fakeExec{}, &fakeStore{cfg: raw}, nil)
	m.width = 120

	next, _ := m.Update(key("s"))
	m = next.(Model)
	next, _ = m.Update(key("enter"))
	m = next.(Model)
	for _, value := range []string{"-", "v", "2"} {
		next, _ = m.Update(key(value))
		m = next.(Model)
	}
	next, _ = m.Update(key("enter"))
	m = next.(Model)

	if m.staged.Projects[0].Name != "sample-api-v2" {
		t.Fatalf("project name = %q, want sample-api-v2", m.staged.Projects[0].Name)
	}
}

func TestSettingsEditorCanEditGlobalToolField(t *testing.T) {
	raw, resolved := testConfig(t, t.TempDir())
	m := NewModelWithServices(raw, resolved, tmux.State{}, &fakeExec{}, &fakeStore{cfg: raw}, nil)
	m.width = 120

	next, _ := m.Update(key("s"))
	m = next.(Model)
	next, _ = m.Update(key("tab"))
	m = next.(Model)
	next, _ = m.Update(key("down"))
	m = next.(Model)
	next, _ = m.Update(key("down"))
	m = next.(Model)
	next, _ = m.Update(key("enter"))
	m = next.(Model)
	for _, value := range []string{" ", "-", "-", "f", "a", "s", "t"} {
		next, _ = m.Update(key(value))
		m = next.(Model)
	}
	next, _ = m.Update(key("enter"))
	m = next.(Model)

	if got := m.staged.Tools["codex"].Command; got != "codex --fast" {
		t.Fatalf("tool command = %q, want codex --fast", got)
	}
}

func TestSettingsProjectToolToggleDisablesToolWithoutDeletingGlobalTool(t *testing.T) {
	projectDir := t.TempDir()
	raw := config.Config{
		Tools: map[string]config.Tool{
			"codex": {Window: "codex", Command: "codex", AfterExit: config.AfterExitShell},
			"yazi":  {Window: "yazi", Command: "yazi", AfterExit: config.AfterExitShell},
		},
		Projects: []config.Project{{
			Name:            "sample-api",
			Path:            projectDir,
			Session:         "sample-api",
			DefaultWindow:   "yazi",
			WindowSelection: config.WindowSelectionConfigured,
			OnExisting:      config.ExistingPolicyAttach,
			ConfirmKill:     boolPtr(true),
			FailurePolicy:   config.FailurePolicyStop,
			Tools:           []config.ProjectTool{{Name: "yazi"}, {Name: "codex"}},
		}},
	}
	resolved, err := config.Resolve(raw, config.ResolveOptions{})
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	m := NewModelWithServices(raw, resolved, tmux.State{}, &fakeExec{}, &fakeStore{cfg: raw}, nil)
	m.width = 120

	next, _ := m.Update(key("s"))
	m = next.(Model)
	for i := 0; i < projectFieldCount; i++ {
		next, _ = m.Update(key("down"))
		m = next.(Model)
	}
	next, _ = m.Update(key("enter"))
	m = next.(Model)

	yazi := m.staged.Projects[0].Tools[0]
	if yazi.Override.Enabled == nil || *yazi.Override.Enabled {
		t.Fatalf("yazi enabled override = %v, want false", yazi.Override.Enabled)
	}
	if _, ok := m.staged.Tools["yazi"]; !ok {
		t.Fatal("global yazi tool should remain available")
	}
	if got := m.staged.Projects[0].DefaultWindow; got != "codex" {
		t.Fatalf("default_window = %q, want codex after disabling yazi", got)
	}
	after, err := config.Resolve(m.staged, config.ResolveOptions{})
	if err != nil {
		t.Fatalf("Resolve(staged) error = %v", err)
	}
	if got := after.Projects[0].Windows; len(got) != 1 || got[0].Window != "codex" {
		t.Fatalf("resolved windows = %#v, want only codex", got)
	}

	next, _ = m.Update(key("enter"))
	m = next.(Model)
	yazi = m.staged.Projects[0].Tools[0]
	if yazi.Override.Enabled != nil {
		t.Fatalf("yazi enabled override = %v, want nil after re-enable", yazi.Override.Enabled)
	}
}

func TestSettingsProjectCanAddAvailableGlobalTool(t *testing.T) {
	projectDir := t.TempDir()
	raw := config.Config{
		Tools: map[string]config.Tool{
			"codex": {Window: "codex", Command: "codex", AfterExit: config.AfterExitShell},
			"yazi":  {Window: "yazi", Command: "yazi", AfterExit: config.AfterExitShell},
		},
		Projects: []config.Project{{
			Name:            "sample-web",
			Path:            projectDir,
			Session:         "sample-web",
			DefaultWindow:   "codex",
			WindowSelection: config.WindowSelectionConfigured,
			OnExisting:      config.ExistingPolicyAttach,
			ConfirmKill:     boolPtr(true),
			FailurePolicy:   config.FailurePolicyStop,
			Tools:           []config.ProjectTool{{Name: "codex"}},
		}},
	}
	resolved, err := config.Resolve(raw, config.ResolveOptions{})
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	m := NewModelWithServices(raw, resolved, tmux.State{}, &fakeExec{}, &fakeStore{cfg: raw}, nil)
	m.width = 120

	next, _ := m.Update(key("s"))
	m = next.(Model)
	m.settingsCursor = projectFieldCount + 1
	next, _ = m.Update(key("enter"))
	m = next.(Model)

	if got := m.staged.Projects[0].Tools; len(got) != 2 || got[1].Name != "yazi" {
		t.Fatalf("project tools = %#v, want codex then yazi", got)
	}
	after, err := config.Resolve(m.staged, config.ResolveOptions{})
	if err != nil {
		t.Fatalf("Resolve(staged) error = %v", err)
	}
	if got := after.Projects[0].Windows; len(got) != 2 || got[1].Window != "yazi" {
		t.Fatalf("resolved windows = %#v, want yazi added", got)
	}
}

func TestSettingsProjectCanRemoveToolReferenceWithoutDeletingGlobalTool(t *testing.T) {
	raw, resolved := testConfig(t, t.TempDir())
	m := NewModelWithServices(raw, resolved, tmux.State{}, &fakeExec{}, &fakeStore{cfg: raw}, nil)
	m.width = 120

	next, _ := m.Update(key("s"))
	m = next.(Model)
	m.settingsCursor = projectFieldCount
	next, _ = m.Update(key("d"))
	m = next.(Model)

	if len(m.staged.Projects[0].Tools) != 0 {
		t.Fatalf("project tools = %#v, want empty", m.staged.Projects[0].Tools)
	}
	if _, ok := m.staged.Tools["codex"]; !ok {
		t.Fatal("global codex tool should remain available")
	}
}

func TestSettingsToolDefaultForNewProjectsCanBeToggled(t *testing.T) {
	raw := config.Config{
		Tools: map[string]config.Tool{
			"yazi": {Window: "yazi", Command: "yazi", AfterExit: config.AfterExitShell},
		},
	}
	resolved, err := config.Resolve(raw, config.ResolveOptions{})
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	m := NewModelWithServices(raw, resolved, tmux.State{}, &fakeExec{}, &fakeStore{cfg: raw}, nil)
	m.width = 120

	next, _ := m.Update(key("s"))
	m = next.(Model)
	next, _ = m.Update(key("tab"))
	m = next.(Model)
	m.settingsCursor = toolFieldCount - 1
	next, _ = m.Update(key("enter"))
	m = next.(Model)

	if !m.staged.Tools["yazi"].DefaultForNewProjects {
		t.Fatal("default_for_new_projects = false, want true")
	}
}

func TestDefaultForNewProjectsSeedsAddedProject(t *testing.T) {
	raw := config.Config{
		Tools: map[string]config.Tool{
			"codex": {Window: "codex", Command: "codex", AfterExit: config.AfterExitShell},
			"yazi":  {Window: "yazi", Command: "yazi", AfterExit: config.AfterExitShell, DefaultForNewProjects: true},
		},
	}
	resolved, err := config.Resolve(raw, config.ResolveOptions{})
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	m := NewModelWithServices(raw, resolved, tmux.State{}, &fakeExec{}, &fakeStore{cfg: raw}, nil)
	m.width = 120

	next, _ := m.Update(key("s"))
	m = next.(Model)
	next, _ = m.Update(key("enter"))
	m = next.(Model)

	if got := m.staged.Projects[0].Tools; len(got) != 1 || got[0].Name != "yazi" {
		t.Fatalf("project tools = %#v, want yazi", got)
	}
	if got := m.staged.Projects[0].DefaultWindow; got != "yazi" {
		t.Fatalf("default_window = %q, want yazi", got)
	}
}

func TestSettingsSaveRequiresExistingProjectPath(t *testing.T) {
	raw, resolved := testConfig(t, filepath.Join(t.TempDir(), "missing"))
	store := &fakeStore{cfg: raw}
	m := NewModelWithServices(raw, resolved, tmux.State{}, &fakeExec{}, store, nil)
	m.width = 120

	next, _ := m.Update(key("s"))
	m = next.(Model)
	next, _ = m.Update(key("ctrl+s"))
	m = next.(Model)

	if store.saved {
		t.Fatal("Save should not accept a missing project path")
	}
	if !strings.Contains(m.settingsMessage, "does not exist") {
		t.Fatalf("settingsMessage = %q, want path validation error", m.settingsMessage)
	}
}

func TestSettingsSaveBacksUpWritesReloadsAndReturnsHome(t *testing.T) {
	projectDir := t.TempDir()
	raw, resolved := testConfig(t, projectDir)
	configPath := filepath.Join(t.TempDir(), "config.yaml")
	store, err := storage.New(configPath)
	if err != nil {
		t.Fatalf("storage.New() error = %v", err)
	}
	if _, err := store.Save(raw, config.ResolveOptions{RequireExistingProjectPaths: true}); err != nil {
		t.Fatalf("initial Save() error = %v", err)
	}

	m := NewModelWithServices(raw, resolved, tmux.State{}, &fakeExec{}, store, nil)
	m.width = 120
	next, _ := m.Update(key("s"))
	m = next.(Model)
	next, _ = m.Update(key("enter"))
	m = next.(Model)
	for _, value := range []string{"-", "s", "a", "v", "e", "d"} {
		next, _ = m.Update(key(value))
		m = next.(Model)
	}
	next, _ = m.Update(key("enter"))
	m = next.(Model)
	next, _ = m.Update(key("ctrl+s"))
	m = next.(Model)

	if m.screen != ScreenHome {
		t.Fatalf("screen = %q, want home", m.screen)
	}
	if m.rawConfig.Projects[0].Name != "sample-api-saved" {
		t.Fatalf("raw project name = %q, want saved name", m.rawConfig.Projects[0].Name)
	}
	backups, err := store.ListBackups()
	if err != nil {
		t.Fatalf("ListBackups() error = %v", err)
	}
	if len(backups) != 1 {
		t.Fatalf("backups = %d, want 1", len(backups))
	}
	if _, err := os.Stat(configPath); err != nil {
		t.Fatalf("saved config missing: %v", err)
	}
}

func TestSettingsDiscardDropsStagedChanges(t *testing.T) {
	raw, resolved := testConfig(t, t.TempDir())
	m := NewModelWithServices(raw, resolved, tmux.State{}, &fakeExec{}, &fakeStore{cfg: raw}, nil)
	m.width = 120

	next, _ := m.Update(key("s"))
	m = next.(Model)
	next, _ = m.Update(key("enter"))
	m = next.(Model)
	next, _ = m.Update(key("-"))
	m = next.(Model)
	next, _ = m.Update(key("enter"))
	m = next.(Model)
	next, _ = m.Update(key("x"))
	m = next.(Model)

	if m.screen != ScreenHome {
		t.Fatalf("screen = %q, want home", m.screen)
	}
	if m.rawConfig.Projects[0].Name != "sample-api" {
		t.Fatalf("raw project name = %q, want unchanged sample-api", m.rawConfig.Projects[0].Name)
	}
}

func TestKeyBindingViewReadsTmuxListKeys(t *testing.T) {
	raw, resolved := testConfig(t, t.TempDir())
	keys := &fakeKeys{bindings: []tmux.KeyBinding{
		{Table: "prefix", Key: "F1", Command: "select-window -t :1"},
		{Table: "prefix", Key: "C-b", Command: "send-prefix"},
	}}
	m := NewModelWithServices(raw, resolved, tmux.State{}, &fakeExec{}, &fakeStore{cfg: raw}, keys)
	m.width = 120

	next, cmd := m.Update(key("b"))
	m = next.(Model)
	if cmd == nil {
		t.Fatal("key binding view should load bindings")
	}
	next, _ = m.Update(cmd())
	m = next.(Model)
	view := m.View()
	if !keys.called {
		t.Fatal("ListKeys was not called")
	}
	if !strings.Contains(view, "F1") || !strings.Contains(view, "send-prefix") {
		t.Fatalf("key binding view missing bindings:\n%s", view)
	}
}

func testModel(width int, state tmux.State) Model {
	return testModelWithExecutor(width, state, &fakeExec{})
}

func testModelWithExecutor(width int, state tmux.State, exec Executor) Model {
	raw := config.Config{}
	resolved := config.ResolvedConfig{Projects: []config.ResolvedProject{
		{
			Name:            "sample-api",
			Path:            "/Users/me/sample-api",
			Session:         "sample-api",
			DefaultWindow:   "codex",
			WindowSelection: config.WindowSelectionConfigured,
			OnExisting:      config.ExistingPolicyAttach,
			ConfirmKill:     true,
			FailurePolicy:   config.FailurePolicyStop,
			Windows: []config.ResolvedWindow{
				{ToolID: "yazi", Window: "yazi", CWD: "/Users/me/sample-api", ShellCommand: `zsh -lc "yazi; exec zsh"`},
				{ToolID: "codex", Window: "codex", CWD: "/Users/me/sample-api", ShellCommand: `zsh -lc "codex; exec zsh"`},
			},
		},
		{
			Name:            "notes",
			Path:            "/Users/me/notes",
			Session:         "notes",
			DefaultWindow:   "yazi",
			WindowSelection: config.WindowSelectionConfigured,
			OnExisting:      config.ExistingPolicyAttach,
			ConfirmKill:     true,
			FailurePolicy:   config.FailurePolicyStop,
			Windows: []config.ResolvedWindow{
				{ToolID: "yazi", Window: "yazi", CWD: "/Users/me/notes", ShellCommand: `zsh -lc "yazi; exec zsh"`},
			},
		},
	}}
	m := NewModel(raw, resolved, state, exec)
	m.width = width
	return m
}

func key(value string) tea.KeyMsg {
	switch value {
	case "enter":
		return tea.KeyMsg{Type: tea.KeyEnter}
	case "esc":
		return tea.KeyMsg{Type: tea.KeyEsc}
	case "up":
		return tea.KeyMsg{Type: tea.KeyUp}
	case "down":
		return tea.KeyMsg{Type: tea.KeyDown}
	case "backspace":
		return tea.KeyMsg{Type: tea.KeyBackspace}
	case "tab":
		return tea.KeyMsg{Type: tea.KeyTab}
	case "ctrl+s":
		return tea.KeyMsg{Type: tea.KeyCtrlS}
	default:
		return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(value)}
	}
}

type fakeExec struct {
	plans  []planner.ActionPlan
	result runner.Result
}

func (f *fakeExec) Run(_ context.Context, plan planner.ActionPlan) runner.Result {
	f.plans = append(f.plans, plan)
	return f.result
}

type fakeStore struct {
	cfg   config.Config
	saved bool
}

func (f *fakeStore) Load() (config.Config, error) {
	return f.cfg, nil
}

func (f *fakeStore) Save(cfg config.Config, opts config.ResolveOptions) (string, error) {
	if _, err := config.Resolve(cfg, opts); err != nil {
		return "", err
	}
	f.cfg = cfg
	f.saved = true
	return "/tmp/config.yaml.bak-test.yaml", nil
}

type fakeKeys struct {
	bindings []tmux.KeyBinding
	err      error
	called   bool
}

func (f *fakeKeys) ListKeys(context.Context) ([]tmux.KeyBinding, error) {
	f.called = true
	return f.bindings, f.err
}

func testConfig(t *testing.T, projectPath string) (config.Config, config.ResolvedConfig) {
	t.Helper()
	raw := config.Config{
		Tools: map[string]config.Tool{
			"codex": {Window: "codex", Command: "codex", AfterExit: config.AfterExitShell},
		},
		Projects: []config.Project{{
			Name:            "sample-api",
			Path:            projectPath,
			Session:         "sample-api",
			DefaultWindow:   "codex",
			WindowSelection: config.WindowSelectionConfigured,
			OnExisting:      config.ExistingPolicyAttach,
			ConfirmKill:     boolPtr(true),
			FailurePolicy:   config.FailurePolicyStop,
			Tools:           []config.ProjectTool{{Name: "codex"}},
		}},
	}
	resolved, err := config.Resolve(raw, config.ResolveOptions{})
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	return raw, resolved
}
