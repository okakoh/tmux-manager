package tui

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/okakoh/tmux-manager/internal/config"
	"github.com/okakoh/tmux-manager/internal/planner"
	"github.com/okakoh/tmux-manager/internal/runner"
	"github.com/okakoh/tmux-manager/internal/tmux"
)

const WideLayoutThreshold = 100

type Screen string

const (
	ScreenHome             Screen = "home"
	ScreenProjectDetail    Screen = "project-detail"
	ScreenWindowPicker     Screen = "window-picker"
	ScreenConfirm          Screen = "confirm"
	ScreenSettingsProjects Screen = "settings-projects"
	ScreenSettingsTools    Screen = "settings-tools"
	ScreenSettingsEdit     Screen = "settings-edit-field"
	ScreenKeybindings      Screen = "keybindings"
	ScreenError            Screen = "error"
	ScreenHelp             Screen = "help"
)

type Executor interface {
	Run(context.Context, planner.ActionPlan) runner.Result
}

type ConfigStore interface {
	Load() (config.Config, error)
	Save(config.Config, config.ResolveOptions) (string, error)
}

type KeyBindingProvider interface {
	ListKeys(context.Context) ([]tmux.KeyBinding, error)
}

type Model struct {
	rawConfig  config.Config
	resolved   config.ResolvedConfig
	state      tmux.State
	executor   Executor
	store      ConfigStore
	keySource  KeyBindingProvider
	tmuxBinary string
	shell      string

	screen         Screen
	previous       Screen
	selected       int
	selectedWin    int
	windowOverride string
	width          int
	height         int
	searching      bool
	search         string

	pendingAction planner.Action
	pendingPlan   planner.ActionPlan
	err           error

	staged          config.Config
	settingsCursor  int
	settingsProject int
	settingsTool    int
	settingsMessage string
	editKind        string
	editField       string
	editValue       string
	keyBindings     []tmux.KeyBinding
	keyStatus       string
}

type runResultMsg struct {
	result runner.Result
}

type attachReadyMsg struct {
	step planner.PlanStep
}

type attachFinishedMsg struct {
	args []string
	err  error
}

type keyBindingsMsg struct {
	bindings []tmux.KeyBinding
	err      error
}

type runnerAdapter struct {
	runner runner.Runner
}

func NewModel(raw config.Config, resolved config.ResolvedConfig, state tmux.State, executor Executor) Model {
	return NewModelWithServices(raw, resolved, state, executor, nil, nil)
}

func NewModelWithServices(raw config.Config, resolved config.ResolvedConfig, state tmux.State, executor Executor, store ConfigStore, keySource KeyBindingProvider) Model {
	return NewModelWithServicesAndShell(raw, resolved, state, executor, store, keySource, config.DefaultShell)
}

func NewModelWithServicesAndShell(raw config.Config, resolved config.ResolvedConfig, state tmux.State, executor Executor, store ConfigStore, keySource KeyBindingProvider, shell string) Model {
	if shell == "" {
		shell = config.DefaultShell
	}
	tmuxBinary := tmux.DefaultBinary
	if client, ok := keySource.(tmux.Client); ok && client.Binary != "" {
		tmuxBinary = client.Binary
	}
	if executor == nil {
		client := tmux.NewClient(tmuxBinary)
		executor = runnerAdapter{runner: runner.New(runner.TmuxExecutor{Client: client, Shell: shell})}
		if keySource == nil {
			keySource = client
		}
	}
	return Model{
		rawConfig:     raw,
		resolved:      resolved,
		state:         state,
		executor:      executor,
		store:         store,
		keySource:     keySource,
		tmuxBinary:    tmuxBinary,
		shell:         shell,
		screen:        ScreenHome,
		pendingAction: planner.ActionLaunchOrAttach,
	}
}

func Run(raw config.Config, resolved config.ResolvedConfig, state tmux.State, executor Executor) error {
	return RunWithServices(raw, resolved, state, executor, nil, nil)
}

func RunWithServices(raw config.Config, resolved config.ResolvedConfig, state tmux.State, executor Executor, store ConfigStore, keySource KeyBindingProvider) error {
	return RunWithServicesAndShell(raw, resolved, state, executor, store, keySource, config.DefaultShell)
}

func RunWithServicesAndShell(raw config.Config, resolved config.ResolvedConfig, state tmux.State, executor Executor, store ConfigStore, keySource KeyBindingProvider, shell string) error {
	program := tea.NewProgram(NewModelWithServicesAndShell(raw, resolved, state, executor, store, keySource, shell), tea.WithAltScreen())
	_, err := program.Run()
	return err
}

func (m Model) Init() tea.Cmd {
	return nil
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		return m, nil
	case runResultMsg:
		if msg.result.Err != nil {
			m.err = msg.result.Err
			m.screen = ScreenError
			return m, nil
		}
		return m, tea.Quit
	case attachReadyMsg:
		binary := m.tmuxBinary
		if binary == "" {
			binary = tmux.DefaultBinary
		}
		args := m.enterSessionArgs(msg.step.TargetSession)
		cmd := exec.Command(binary, args...) // #nosec G204 -- binary is the startup-resolved tmux path.
		cmd.Stdin = os.Stdin
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		return m, tea.ExecProcess(cmd, func(err error) tea.Msg {
			return attachFinishedMsg{args: append([]string{binary}, args...), err: err}
		})
	case attachFinishedMsg:
		if msg.err != nil {
			m.err = m.attachFailureError(msg)
			m.screen = ScreenError
			return m, nil
		}
		return m, tea.Quit
	case keyBindingsMsg:
		if msg.err != nil {
			m.keyStatus = msg.err.Error()
			m.keyBindings = nil
			return m, nil
		}
		m.keyBindings = msg.bindings
		m.keyStatus = ""
		return m, nil
	case tea.KeyMsg:
		return m.updateKey(msg)
	default:
		return m, nil
	}
}

func (m Model) enterSessionArgs(session string) []string {
	return tmux.EnterSessionArgs(session, os.Getenv("TMUX") != "")
}

func (m Model) attachFailureError(msg attachFinishedMsg) error {
	err := &tmux.Error{Kind: tmux.ErrorCommandFailed, Args: msg.args, Err: msg.err}
	diagnostic, diagnosticErr := tmux.NewClient(m.tmuxBinary).VersionDiagnostic(context.Background())
	if diagnosticErr != nil || !diagnostic.Mismatch() {
		return err
	}
	return fmt.Errorf("%w\n\n%s", err, diagnostic.Message())
}

func (m Model) updateKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	key := msg.String()
	if key == "ctrl+c" {
		return m, tea.Quit
	}
	if m.searching {
		return m.updateSearch(key, msg)
	}

	switch m.screen {
	case ScreenHome:
		return m.updateHome(key)
	case ScreenProjectDetail:
		return m.updateDetail(key)
	case ScreenWindowPicker:
		return m.updateWindowPicker(key)
	case ScreenConfirm:
		return m.updateConfirm(key)
	case ScreenSettingsProjects:
		return m.updateSettingsProjects(key)
	case ScreenSettingsTools:
		return m.updateSettingsTools(key)
	case ScreenSettingsEdit:
		return m.updateSettingsEdit(key, msg)
	case ScreenKeybindings:
		return m.updateKeybindings(key)
	case ScreenError:
		return m.updateError(key)
	case ScreenHelp:
		return m.updateHelp(key)
	default:
		return m, nil
	}
}

func (m Model) updateSearch(key string, msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch key {
	case "esc":
		m.searching = false
		m.search = ""
	case "enter":
		m.searching = false
	case "backspace":
		if len(m.search) > 0 {
			m.search = m.search[:len(m.search)-1]
			m.selected = m.firstFilteredIndex()
		}
	default:
		if msg.Type == tea.KeyRunes {
			m.search += string(msg.Runes)
			m.selected = m.firstFilteredIndex()
		}
	}
	return m, nil
}

func (m Model) updateHome(key string) (tea.Model, tea.Cmd) {
	switch key {
	case "q":
		return m, tea.Quit
	case "up":
		m.moveSelection(-1)
	case "down", "j":
		m.moveSelection(1)
	case "/":
		m.searching = true
		m.search = ""
	case "?":
		m.previous = m.screen
		m.screen = ScreenHelp
	case "s":
		return m.openSettingsProjects()
	case "b":
		return m.openKeybindings()
	case "enter":
		if m.isWide() {
			return m.startAction(planner.ActionLaunchOrAttach)
		}
		m.screen = ScreenProjectDetail
	case "r":
		return m.startAction(planner.ActionRecreate)
	case "k":
		return m.startAction(planner.ActionKill)
	case "w":
		return m.openWindowPicker(planner.ActionSelectWindow)
	}
	return m, nil
}

func (m Model) updateDetail(key string) (tea.Model, tea.Cmd) {
	switch key {
	case "esc", "q":
		m.screen = ScreenHome
	case "?":
		m.previous = m.screen
		m.screen = ScreenHelp
	case "s":
		return m.openSettingsProjects()
	case "b":
		return m.openKeybindings()
	case "enter":
		return m.startAction(planner.ActionLaunchOrAttach)
	case "r":
		return m.startAction(planner.ActionRecreate)
	case "k":
		return m.startAction(planner.ActionKill)
	case "w":
		return m.openWindowPicker(planner.ActionSelectWindow)
	}
	return m, nil
}

func (m Model) updateWindowPicker(key string) (tea.Model, tea.Cmd) {
	project, ok := m.currentProject()
	if !ok {
		m.screen = ScreenHome
		return m, nil
	}
	switch key {
	case "esc", "q":
		m.screen = m.backScreen()
	case "up", "k":
		if m.selectedWin > 0 {
			m.selectedWin--
		}
	case "down", "j":
		if m.selectedWin < len(project.Windows)-1 {
			m.selectedWin++
		}
	case "enter":
		if len(project.Windows) == 0 {
			return m, nil
		}
		m.windowOverride = project.Windows[m.selectedWin].Window
		if m.pendingAction != "" && m.pendingAction != planner.ActionSelectWindow {
			return m.startAction(m.pendingAction)
		}
		m.screen = m.backScreen()
	}
	return m, nil
}

func (m Model) updateConfirm(key string) (tea.Model, tea.Cmd) {
	switch key {
	case "esc", "q", "n":
		m.screen = m.backScreen()
	case "y", "enter":
		return m, m.executePlan(m.pendingPlan)
	case "a":
		if m.pendingPlan.Prompt == planner.PromptExisting {
			return m.startActionWithExistingPolicy(planner.ActionLaunchOrAttach, config.ExistingPolicyAttach)
		}
	case "r":
		if m.pendingPlan.Prompt == planner.PromptExisting {
			return m.startActionWithExistingPolicy(planner.ActionRecreate, config.ExistingPolicyRecreate)
		}
	}
	return m, nil
}

func (m Model) updateError(key string) (tea.Model, tea.Cmd) {
	switch key {
	case "q", "ctrl+c":
		return m, tea.Quit
	case "esc", "h":
		m.err = nil
		m.screen = ScreenHome
	}
	return m, nil
}

func (m Model) updateHelp(key string) (tea.Model, tea.Cmd) {
	switch key {
	case "q", "esc", "?":
		m.screen = m.backScreen()
	}
	return m, nil
}

func (m Model) startAction(action planner.Action) (tea.Model, tea.Cmd) {
	project, ok := m.currentProject()
	if !ok {
		return m, nil
	}
	plan, err := planner.Plan(project, m.state, planner.Request{Action: action, SelectedWindow: m.windowOverride})
	if err != nil {
		m.err = err
		m.screen = ScreenError
		return m, nil
	}
	if plan.Prompt == planner.PromptWindow {
		m.pendingAction = action
		return m.openWindowPicker(action)
	}
	if plan.Prompt == planner.PromptExisting {
		m.pendingPlan = plan
		m.previous = m.screen
		m.screen = ScreenConfirm
		return m, nil
	}
	if plan.RequiresConfirmation {
		m.pendingPlan = plan
		m.previous = m.screen
		m.screen = ScreenConfirm
		return m, nil
	}
	return m, m.executePlan(plan)
}

func (m Model) startActionWithExistingPolicy(action planner.Action, policy config.ExistingPolicy) (tea.Model, tea.Cmd) {
	project, ok := m.currentProject()
	if !ok {
		return m, nil
	}
	project.OnExisting = policy
	plan, err := planner.Plan(project, m.state, planner.Request{Action: action, SelectedWindow: m.windowOverride})
	if err != nil {
		m.err = err
		m.screen = ScreenError
		return m, nil
	}
	if plan.RequiresConfirmation {
		m.pendingPlan = plan
		m.screen = ScreenConfirm
		return m, nil
	}
	return m, m.executePlan(plan)
}

func (m Model) openWindowPicker(action planner.Action) (tea.Model, tea.Cmd) {
	m.pendingAction = action
	m.previous = m.screen
	m.screen = ScreenWindowPicker
	m.selectedWin = 0
	project, ok := m.currentProject()
	if ok && m.windowOverride != "" {
		for i, window := range project.Windows {
			if window.Window == m.windowOverride {
				m.selectedWin = i
				break
			}
		}
	}
	return m, nil
}

func (m Model) executePlan(plan planner.ActionPlan) tea.Cmd {
	if len(plan.Steps) > 0 {
		last := plan.Steps[len(plan.Steps)-1]
		if last.Kind == planner.StepAttach {
			preAttach := plan
			preAttach.Steps = append([]planner.PlanStep(nil), plan.Steps[:len(plan.Steps)-1]...)
			return func() tea.Msg {
				result := m.executor.Run(context.Background(), preAttach)
				if result.Err != nil && !result.PartialSuccess {
					return runResultMsg{result: result}
				}
				return attachReadyMsg{step: last}
			}
		}
	}
	return func() tea.Msg {
		return runResultMsg{result: m.executor.Run(context.Background(), plan)}
	}
}

func (a runnerAdapter) Run(ctx context.Context, plan planner.ActionPlan) runner.Result {
	return a.runner.Run(ctx, plan)
}

func (m Model) View() string {
	switch m.screen {
	case ScreenHome:
		return m.viewHome()
	case ScreenProjectDetail:
		return m.viewDetail()
	case ScreenWindowPicker:
		return m.viewWindowPicker()
	case ScreenConfirm:
		return m.viewConfirm()
	case ScreenSettingsProjects:
		return m.viewSettingsProjects()
	case ScreenSettingsTools:
		return m.viewSettingsTools()
	case ScreenSettingsEdit:
		return m.viewSettingsEdit()
	case ScreenKeybindings:
		return m.viewKeybindings()
	case ScreenError:
		return m.viewError()
	case ScreenHelp:
		return m.viewHelp()
	default:
		return m.viewHome()
	}
}

func (m Model) viewHome() string {
	if m.isWide() {
		left := panelStyle.Width(34).Render(m.viewProjectList())
		right := panelStyle.Width(max(40, m.width-42)).Render(m.detailText())
		return lipgloss.JoinHorizontal(lipgloss.Top, left, right)
	}
	return panelStyle.Width(m.contentWidth()).Render(m.viewProjectList())
}

func (m Model) viewDetail() string {
	return panelStyle.Width(m.contentWidth()).Render(m.detailText())
}

func (m Model) viewProjectList() string {
	var b strings.Builder
	b.WriteString(titleStyle.Render("tmux-manager"))
	b.WriteString("\n")
	if m.searching || m.search != "" {
		b.WriteString(mutedStyle.Render("search: " + m.search))
		b.WriteString("\n")
	}
	if len(m.resolved.Projects) == 0 {
		b.WriteString("\nNo projects configured.\n")
		return b.String()
	}
	for i, project := range m.resolved.Projects {
		if !m.matchesSearch(project) {
			continue
		}
		cursor := "  "
		style := normalStyle
		if i == m.selected {
			cursor = "> "
			style = selectedStyle
		}
		status := "stopped"
		if m.state.HasSession(project.Session) {
			status = "running"
		}
		b.WriteString(style.Render(fmt.Sprintf("%s%s [%s]", cursor, project.Name, status)))
		b.WriteString("\n")
	}
	b.WriteString("\n")
	if m.isWide() {
		b.WriteString(mutedStyle.Render("enter launch/attach  r recreate  k kill  w window  s settings  b keys  / search  ? help  q quit"))
	} else {
		b.WriteString(mutedStyle.Render("enter detail  r recreate  k kill  s settings  b keys  / search  ? help  q quit"))
	}
	return b.String()
}

func (m Model) detailText() string {
	project, ok := m.currentProject()
	if !ok {
		return titleStyle.Render("No Project") + "\n\nAdd projects in the config file."
	}
	var b strings.Builder
	b.WriteString(titleStyle.Render(project.Name))
	b.WriteString("\n")
	b.WriteString(fmt.Sprintf("path: %s\nsession: %s\n", project.Path, project.Session))
	b.WriteString(fmt.Sprintf("existing: %s  failure: %s\n", project.OnExisting, project.FailurePolicy))
	if m.windowOverride != "" {
		b.WriteString(accentStyle.Render("target window: " + m.windowOverride))
		b.WriteString("\n")
	}
	b.WriteString("\nwindows\n")
	for _, window := range project.Windows {
		active := ""
		if session, ok := m.state.FindSession(project.Session); ok {
			for _, live := range session.Windows {
				if live.Name == window.Window && live.Active {
					active = " active"
				}
			}
		}
		b.WriteString(fmt.Sprintf("  %s  %s%s\n", window.Window, window.CWD, active))
	}
	b.WriteString("\n")
	b.WriteString(mutedStyle.Render("enter launch/attach  r recreate  k kill  w choose window  s settings  b keys  esc back"))
	return b.String()
}

func (m Model) viewWindowPicker() string {
	project, ok := m.currentProject()
	if !ok {
		return m.viewHome()
	}
	var b strings.Builder
	b.WriteString(titleStyle.Render("Choose Window"))
	b.WriteString("\n")
	for i, window := range project.Windows {
		cursor := "  "
		style := normalStyle
		if i == m.selectedWin {
			cursor = "> "
			style = selectedStyle
		}
		b.WriteString(style.Render(cursor + window.Window))
		b.WriteString("\n")
	}
	b.WriteString("\n")
	b.WriteString(mutedStyle.Render("enter select  esc cancel"))
	return panelStyle.Width(m.contentWidth()).Render(b.String())
}

func (m Model) viewConfirm() string {
	var b strings.Builder
	if m.pendingPlan.Prompt == planner.PromptExisting {
		b.WriteString(titleStyle.Render("Session Already Running"))
		b.WriteString("\n")
		b.WriteString("a attach  r recreate  esc cancel\n")
		return panelStyle.Width(m.contentWidth()).Render(b.String())
	}
	b.WriteString(titleStyle.Render("Confirm"))
	b.WriteString("\n")
	for _, step := range m.pendingPlan.Steps {
		prefix := "  "
		if step.Destructive {
			prefix = "! "
		}
		b.WriteString(prefix + step.Description + "\n")
	}
	b.WriteString("\n")
	b.WriteString(mutedStyle.Render("enter/y confirm  esc/n cancel"))
	return panelStyle.Width(m.contentWidth()).Render(b.String())
}

func (m Model) viewError() string {
	message := "unknown error"
	if m.err != nil {
		message = m.err.Error()
	}
	return panelStyle.Width(m.contentWidth()).Render(titleStyle.Render("Error") + "\n" + message + "\n\n" + mutedStyle.Render("esc/h home  q quit"))
}

func (m Model) viewHelp() string {
	text := strings.Join([]string{
		titleStyle.Render("Help"),
		"up/down or j: move",
		"enter: open detail on narrow terminals, launch/attach on wide terminals",
		"r: recreate session",
		"k: kill session",
		"w: choose target window",
		"s: settings",
		"b: tmux key bindings",
		"/: search projects",
		"?: toggle help",
		"q or esc: back/quit",
	}, "\n")
	return panelStyle.Width(m.contentWidth()).Render(text)
}

func (m Model) currentProject() (config.ResolvedProject, bool) {
	if len(m.resolved.Projects) == 0 || m.selected < 0 || m.selected >= len(m.resolved.Projects) {
		return config.ResolvedProject{}, false
	}
	return m.resolved.Projects[m.selected], true
}

func (m *Model) moveSelection(delta int) {
	if len(m.resolved.Projects) == 0 {
		return
	}
	next := m.selected
	for {
		next += delta
		if next < 0 || next >= len(m.resolved.Projects) {
			return
		}
		if m.matchesSearch(m.resolved.Projects[next]) {
			m.selected = next
			return
		}
	}
}

func (m Model) firstFilteredIndex() int {
	for i, project := range m.resolved.Projects {
		if m.matchesSearch(project) {
			return i
		}
	}
	return m.selected
}

func (m Model) matchesSearch(project config.ResolvedProject) bool {
	if m.search == "" {
		return true
	}
	query := strings.ToLower(m.search)
	return strings.Contains(strings.ToLower(project.Name), query) || strings.Contains(strings.ToLower(project.Path), query)
}

func (m Model) backScreen() Screen {
	if m.previous != "" && m.previous != ScreenConfirm && m.previous != ScreenWindowPicker {
		return m.previous
	}
	return ScreenHome
}

func (m Model) isWide() bool {
	return m.width >= WideLayoutThreshold
}

func (m Model) contentWidth() int {
	if m.width <= 0 {
		return 80
	}
	return max(40, m.width-4)
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

var (
	panelStyle    = lipgloss.NewStyle().Padding(1, 2)
	titleStyle    = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("212"))
	selectedStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("230")).Background(lipgloss.Color("57"))
	normalStyle   = lipgloss.NewStyle()
	mutedStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("244"))
	accentStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("86"))
)
