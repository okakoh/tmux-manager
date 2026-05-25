package tui

import (
	"context"
	"fmt"
	"sort"
	"strconv"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/okakoh/tmux-manager/internal/config"
	"github.com/okakoh/tmux-manager/internal/tmux"
)

const (
	projectFieldCount = 8
	toolFieldCount    = 5
)

type projectToolRow struct {
	name  string
	index int
	tool  config.ProjectTool
	state string
}

func (m Model) openSettingsProjects() (tea.Model, tea.Cmd) {
	staged, err := cloneConfig(m.rawConfig)
	if err != nil {
		m.err = err
		m.screen = ScreenError
		return m, nil
	}
	m.staged = staged
	m.settingsCursor = 0
	m.settingsProject = clampIndex(m.settingsProject, len(m.staged.Projects))
	m.settingsMessage = ""
	m.previous = m.screen
	m.screen = ScreenSettingsProjects
	return m, nil
}

func (m Model) updateSettingsProjects(key string) (tea.Model, tea.Cmd) {
	switch key {
	case "esc", "q", "x":
		m.staged = config.Config{}
		m.screen = ScreenHome
	case "tab", "t":
		m.settingsCursor = 0
		m.screen = ScreenSettingsTools
	case "ctrl+s":
		return m.saveSettings()
	case "up", "k":
		if m.settingsCursor > 0 {
			m.settingsCursor--
		}
	case "down", "j":
		if m.settingsCursor < m.projectRowCount()-1 {
			m.settingsCursor++
		}
	case "left", "h":
		if m.settingsProject > 0 {
			m.settingsProject--
			m.settingsCursor = min(m.settingsCursor, m.projectRowCount()-1)
		}
	case "right", "l":
		if m.settingsProject < len(m.staged.Projects)-1 {
			m.settingsProject++
			m.settingsCursor = min(m.settingsCursor, m.projectRowCount()-1)
		}
	case "a":
		m.addProject()
	case "d":
		if m.removeProjectToolAtCursor() {
			return m, nil
		}
		m.deleteProject()
	case " ":
		if m.toggleProjectToolAtCursor() {
			return m, nil
		}
	case "enter":
		return m.activateProjectSetting()
	}
	return m, nil
}

func (m Model) updateSettingsTools(key string) (tea.Model, tea.Cmd) {
	switch key {
	case "esc", "q", "x":
		m.staged = config.Config{}
		m.screen = ScreenHome
	case "tab", "p":
		m.settingsCursor = 0
		m.screen = ScreenSettingsProjects
	case "ctrl+s":
		return m.saveSettings()
	case "up", "k":
		if m.settingsCursor > 0 {
			m.settingsCursor--
		}
	case "down", "j":
		if m.settingsCursor < m.toolRowCount()-1 {
			m.settingsCursor++
		}
	case "left", "h":
		if m.settingsTool > 0 {
			m.settingsTool--
		}
	case "right", "l":
		if m.settingsTool < len(m.staged.Tools)-1 {
			m.settingsTool++
		}
	case "a":
		m.addTool()
	case "d":
		m.deleteTool()
	case " ":
		names := sortedToolNames(m.staged.Tools)
		if len(names) > 0 && m.settingsCursor < toolFieldCount && toolFieldName(m.settingsCursor) == "default_for_new_projects" {
			m.cycleToolDefaultForNewProjects(names[m.settingsTool])
		}
	case "enter":
		return m.activateToolSetting()
	}
	return m, nil
}

func (m Model) updateSettingsEdit(key string, msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch key {
	case "esc":
		m.screen = m.backScreen()
	case "enter":
		m.commitEditValue()
		m.screen = m.backScreen()
	case "backspace":
		if len(m.editValue) > 0 {
			m.editValue = m.editValue[:len(m.editValue)-1]
		}
	default:
		if msg.Type == tea.KeyRunes {
			m.editValue += string(msg.Runes)
		}
	}
	return m, nil
}

func (m Model) activateProjectSetting() (tea.Model, tea.Cmd) {
	if len(m.staged.Projects) == 0 {
		switch m.settingsCursor {
		case 0:
			m.addProject()
		case 1:
			m.screen = ScreenSettingsTools
		case 2:
			return m.saveSettings()
		case 3:
			m.staged = config.Config{}
			m.screen = ScreenHome
		}
		return m, nil
	}
	if m.settingsCursor < projectFieldCount {
		field := projectFieldName(m.settingsCursor)
		if cycleProjectField(&m, field) {
			return m, nil
		}
		m.openEdit("project", field, projectFieldValue(m.staged.Projects[m.settingsProject], field))
		return m, nil
	}
	if m.toggleProjectToolAtCursor() {
		return m, nil
	}
	switch m.settingsCursor - m.projectActionOffset(m.staged.Projects[m.settingsProject]) {
	case 0:
		m.addProject()
	case 1:
		m.deleteProject()
	case 2:
		m.settingsCursor = 0
		m.screen = ScreenSettingsTools
	case 3:
		return m.saveSettings()
	case 4:
		m.staged = config.Config{}
		m.screen = ScreenHome
	}
	return m, nil
}

func (m Model) activateToolSetting() (tea.Model, tea.Cmd) {
	names := sortedToolNames(m.staged.Tools)
	if len(names) == 0 {
		switch m.settingsCursor {
		case 0:
			m.addTool()
		case 1:
			m.screen = ScreenSettingsProjects
		case 2:
			return m.saveSettings()
		case 3:
			m.staged = config.Config{}
			m.screen = ScreenHome
		}
		return m, nil
	}
	if m.settingsCursor < toolFieldCount {
		field := toolFieldName(m.settingsCursor)
		if field == "after_exit" {
			m.cycleToolAfterExit(names[m.settingsTool])
			return m, nil
		}
		if field == "default_for_new_projects" {
			m.cycleToolDefaultForNewProjects(names[m.settingsTool])
			return m, nil
		}
		m.openEdit("tool", field, toolFieldValue(names[m.settingsTool], m.staged.Tools[names[m.settingsTool]], field))
		return m, nil
	}
	switch m.settingsCursor - toolFieldCount {
	case 0:
		m.addTool()
	case 1:
		m.deleteTool()
	case 2:
		m.settingsCursor = 0
		m.screen = ScreenSettingsProjects
	case 3:
		return m.saveSettings()
	case 4:
		m.staged = config.Config{}
		m.screen = ScreenHome
	}
	return m, nil
}

func (m *Model) openEdit(kind, field, value string) {
	m.editKind = kind
	m.editField = field
	m.editValue = value
	m.previous = m.screen
	m.screen = ScreenSettingsEdit
}

func (m *Model) commitEditValue() {
	switch m.editKind {
	case "project":
		if len(m.staged.Projects) == 0 {
			return
		}
		project := &m.staged.Projects[m.settingsProject]
		switch m.editField {
		case "name":
			project.Name = strings.TrimSpace(m.editValue)
		case "path":
			project.Path = strings.TrimSpace(m.editValue)
		case "session":
			project.Session = strings.TrimSpace(m.editValue)
		case "default_window":
			project.DefaultWindow = strings.TrimSpace(m.editValue)
		}
	case "tool":
		names := sortedToolNames(m.staged.Tools)
		if len(names) == 0 {
			return
		}
		oldName := names[m.settingsTool]
		tool := m.staged.Tools[oldName]
		switch m.editField {
		case "name":
			m.renameTool(oldName, strings.TrimSpace(m.editValue))
			return
		case "window":
			tool.Window = strings.TrimSpace(m.editValue)
		case "command":
			tool.Command = strings.TrimSpace(m.editValue)
		}
		m.staged.Tools[oldName] = tool
	}
}

func cycleProjectField(m *Model, field string) bool {
	if len(m.staged.Projects) == 0 {
		return false
	}
	project := &m.staged.Projects[m.settingsProject]
	switch field {
	case "window_selection":
		if project.WindowSelection == config.WindowSelectionPrompt {
			project.WindowSelection = config.WindowSelectionConfigured
		} else {
			project.WindowSelection = config.WindowSelectionPrompt
		}
		return true
	case "on_existing":
		switch project.OnExisting {
		case "", config.ExistingPolicyAttach:
			project.OnExisting = config.ExistingPolicyPrompt
		case config.ExistingPolicyPrompt:
			project.OnExisting = config.ExistingPolicyRecreate
		default:
			project.OnExisting = config.ExistingPolicyAttach
		}
		return true
	case "confirm_kill":
		value := true
		if project.ConfirmKill != nil {
			value = !*project.ConfirmKill
		} else {
			value = false
		}
		project.ConfirmKill = &value
		return true
	case "failure_policy":
		if project.FailurePolicy == config.FailurePolicyContinue {
			project.FailurePolicy = config.FailurePolicyStop
		} else {
			project.FailurePolicy = config.FailurePolicyContinue
		}
		return true
	default:
		return false
	}
}

func (m *Model) cycleToolAfterExit(name string) {
	tool := m.staged.Tools[name]
	tool.AfterExit = config.AfterExitShell
	m.staged.Tools[name] = tool
}

func (m *Model) cycleToolDefaultForNewProjects(name string) {
	tool := m.staged.Tools[name]
	tool.DefaultForNewProjects = !tool.DefaultForNewProjects
	m.staged.Tools[name] = tool
	if tool.DefaultForNewProjects {
		m.settingsMessage = fmt.Sprintf("%s will be enabled on new projects", name)
	} else {
		m.settingsMessage = fmt.Sprintf("%s will not be enabled on new projects", name)
	}
}

func (m *Model) addProject() {
	name := uniqueProjectName(m.staged.Projects, "new-project")
	projectTools := defaultProjectTools(m.staged.Tools)
	defaultWindow := ""
	if len(projectTools) > 0 {
		defaultWindow = projectToolWindowName(m.staged.Tools, projectTools[0])
	}
	m.staged.Projects = append(m.staged.Projects, config.Project{
		Name:            name,
		Path:            ".",
		Session:         name,
		DefaultWindow:   defaultWindow,
		WindowSelection: config.WindowSelectionConfigured,
		OnExisting:      config.ExistingPolicyAttach,
		ConfirmKill:     boolPtr(true),
		FailurePolicy:   config.FailurePolicyStop,
		Tools:           projectTools,
	})
	m.settingsProject = len(m.staged.Projects) - 1
	m.settingsCursor = 0
	m.settingsMessage = "project added"
}

func (m *Model) deleteProject() {
	if len(m.staged.Projects) == 0 {
		return
	}
	m.staged.Projects = append(m.staged.Projects[:m.settingsProject], m.staged.Projects[m.settingsProject+1:]...)
	m.settingsProject = clampIndex(m.settingsProject, len(m.staged.Projects))
	m.settingsCursor = min(m.settingsCursor, m.projectRowCount()-1)
	m.settingsMessage = "project deleted"
}

func (m *Model) addTool() {
	if m.staged.Tools == nil {
		m.staged.Tools = map[string]config.Tool{}
	}
	name := uniqueToolName(m.staged.Tools, "new-tool")
	m.staged.Tools[name] = config.Tool{Window: name, Command: "zsh", AfterExit: config.AfterExitShell}
	m.settingsTool = indexOf(sortedToolNames(m.staged.Tools), name)
	m.settingsCursor = 0
	m.settingsMessage = "tool added"
}

func (m *Model) deleteTool() {
	names := sortedToolNames(m.staged.Tools)
	if len(names) == 0 {
		return
	}
	name := names[m.settingsTool]
	if toolIsReferenced(m.staged.Projects, name) {
		m.settingsMessage = "tool is still used by a project"
		return
	}
	delete(m.staged.Tools, name)
	m.settingsTool = clampIndex(m.settingsTool, len(m.staged.Tools))
	m.settingsCursor = min(m.settingsCursor, m.toolRowCount()-1)
	m.settingsMessage = "tool deleted"
}

func (m *Model) toggleProjectToolAtCursor() bool {
	if len(m.staged.Projects) == 0 {
		return false
	}
	project := &m.staged.Projects[m.settingsProject]
	rows := projectToolRows(*project, m.staged.Tools)
	rowIndex := m.settingsCursor - projectFieldCount
	if rowIndex < 0 || rowIndex >= len(rows) {
		return false
	}
	row := rows[rowIndex]
	if row.index == -1 {
		project.Tools = append(project.Tools, config.ProjectTool{Name: row.name})
		if project.DefaultWindow == "" {
			project.DefaultWindow = projectToolWindowName(m.staged.Tools, config.ProjectTool{Name: row.name})
		}
		m.settingsMessage = fmt.Sprintf("%s added to %s", row.name, project.Name)
		return true
	}
	tool := &project.Tools[row.index]
	if projectToolEnabled(*tool) {
		disabled := false
		tool.Override.Enabled = &disabled
		m.retargetDefaultWindowAfterDisable(project, *tool)
		m.settingsMessage = fmt.Sprintf("%s disabled for %s", tool.Name, project.Name)
	} else {
		tool.Override.Enabled = nil
		m.settingsMessage = fmt.Sprintf("%s enabled for %s", tool.Name, project.Name)
	}
	return true
}

func (m *Model) removeProjectToolAtCursor() bool {
	if len(m.staged.Projects) == 0 {
		return false
	}
	project := &m.staged.Projects[m.settingsProject]
	rows := projectToolRows(*project, m.staged.Tools)
	rowIndex := m.settingsCursor - projectFieldCount
	if rowIndex < 0 || rowIndex >= len(rows) {
		return false
	}
	row := rows[rowIndex]
	if row.index == -1 {
		m.settingsMessage = fmt.Sprintf("%s is not used by %s", row.name, project.Name)
		return true
	}
	removed := project.Tools[row.index]
	project.Tools = append(project.Tools[:row.index], project.Tools[row.index+1:]...)
	m.retargetDefaultWindowAfterDisable(project, removed)
	m.settingsCursor = min(m.settingsCursor, m.projectRowCount()-1)
	m.settingsMessage = fmt.Sprintf("%s removed from %s", removed.Name, project.Name)
	return true
}

func (m *Model) retargetDefaultWindowAfterDisable(project *config.Project, disabled config.ProjectTool) {
	if project.DefaultWindow == "" || project.DefaultWindow != projectToolWindowName(m.staged.Tools, disabled) {
		return
	}
	for _, candidate := range project.Tools {
		if candidate.Name == disabled.Name || !projectToolEnabled(candidate) {
			continue
		}
		project.DefaultWindow = projectToolWindowName(m.staged.Tools, candidate)
		return
	}
	project.DefaultWindow = ""
}

func (m *Model) renameTool(oldName, newName string) {
	if newName == "" {
		m.settingsMessage = "tool name must not be empty"
		return
	}
	if oldName != newName {
		if _, exists := m.staged.Tools[newName]; exists {
			m.settingsMessage = "tool name already exists"
			return
		}
	}
	tool := m.staged.Tools[oldName]
	delete(m.staged.Tools, oldName)
	m.staged.Tools[newName] = tool
	for i := range m.staged.Projects {
		for j := range m.staged.Projects[i].Tools {
			if m.staged.Projects[i].Tools[j].Name == oldName {
				m.staged.Projects[i].Tools[j].Name = newName
			}
		}
	}
	m.settingsTool = indexOf(sortedToolNames(m.staged.Tools), newName)
}

func (m Model) saveSettings() (tea.Model, tea.Cmd) {
	if m.store == nil {
		m.settingsMessage = "no config store is available"
		return m, nil
	}
	backup, err := m.store.Save(m.staged, config.ResolveOptions{RequireExistingProjectPaths: true})
	if err != nil {
		m.settingsMessage = err.Error()
		return m, nil
	}
	raw, err := m.store.Load()
	if err != nil {
		m.settingsMessage = err.Error()
		return m, nil
	}
	resolved, err := config.Resolve(raw, config.ResolveOptions{})
	if err != nil {
		m.settingsMessage = err.Error()
		return m, nil
	}
	m.rawConfig = raw
	m.resolved = resolved
	m.staged = config.Config{}
	m.selected = clampIndex(m.selected, len(m.resolved.Projects))
	m.settingsMessage = ""
	if backup != "" {
		m.settingsMessage = "saved with backup " + backup
	}
	m.screen = ScreenHome
	return m, nil
}

func (m Model) openKeybindings() (tea.Model, tea.Cmd) {
	m.previous = m.screen
	m.screen = ScreenKeybindings
	m.keyBindings = nil
	m.keyStatus = "loading..."
	return m, func() tea.Msg {
		if m.keySource == nil {
			return keyBindingsMsg{err: fmt.Errorf("no key binding provider is available")}
		}
		bindings, err := m.keySource.ListKeys(context.Background())
		return keyBindingsMsg{bindings: bindings, err: err}
	}
}

func (m Model) updateKeybindings(key string) (tea.Model, tea.Cmd) {
	switch key {
	case "esc", "q":
		m.screen = m.backScreen()
	case "r":
		return m.openKeybindings()
	}
	return m, nil
}

func (m Model) viewSettingsProjects() string {
	var b strings.Builder
	b.WriteString(titleStyle.Render("Settings Projects"))
	b.WriteString("\n")
	if m.settingsMessage != "" {
		b.WriteString(accentStyle.Render(m.settingsMessage))
		b.WriteString("\n")
	}
	if len(m.staged.Projects) == 0 {
		writeSettingRow(&b, m.settingsCursor, 0, "add project")
		writeSettingRow(&b, m.settingsCursor, 1, "tools")
		writeSettingRow(&b, m.settingsCursor, 2, "save")
		writeSettingRow(&b, m.settingsCursor, 3, "discard")
		return panelStyle.Width(m.contentWidth()).Render(b.String())
	}
	project := m.staged.Projects[m.settingsProject]
	b.WriteString(fmt.Sprintf("project %d/%d\n\n", m.settingsProject+1, len(m.staged.Projects)))
	fields := []string{
		"name: " + project.Name,
		"path: " + project.Path,
		"session: " + project.Session,
		"default_window: " + project.DefaultWindow,
		"window_selection: " + string(project.WindowSelection),
		"on_existing: " + string(project.OnExisting),
		"confirm_kill: " + boolField(project.ConfirmKill),
		"failure_policy: " + string(project.FailurePolicy),
	}
	for i, row := range fields {
		writeSettingRow(&b, m.settingsCursor, i, row)
	}
	rows := projectToolRows(project, m.staged.Tools)
	if len(rows) > 0 {
		b.WriteString("\ntools\n")
	}
	for i, row := range rows {
		writeProjectToolRow(&b, m.settingsCursor, projectFieldCount+i, m.staged.Tools, row)
	}
	actionOffset := m.projectActionOffset(project)
	for i, row := range []string{"add project", "delete project", "tools", "save", "discard"} {
		writeSettingRow(&b, m.settingsCursor, actionOffset+i, row)
	}
	b.WriteString("\n")
	b.WriteString(mutedStyle.Render("enter edit/cycle/toggle  space toggle tool  h/l project  a add  d delete  tab tools  ctrl+s save  x discard"))
	return panelStyle.Width(m.contentWidth()).Render(b.String())
}

func (m Model) viewSettingsTools() string {
	var b strings.Builder
	b.WriteString(titleStyle.Render("Settings Tools"))
	b.WriteString("\n")
	if m.settingsMessage != "" {
		b.WriteString(accentStyle.Render(m.settingsMessage))
		b.WriteString("\n")
	}
	names := sortedToolNames(m.staged.Tools)
	if len(names) == 0 {
		writeSettingRow(&b, m.settingsCursor, 0, "add tool")
		writeSettingRow(&b, m.settingsCursor, 1, "projects")
		writeSettingRow(&b, m.settingsCursor, 2, "save")
		writeSettingRow(&b, m.settingsCursor, 3, "discard")
		return panelStyle.Width(m.contentWidth()).Render(b.String())
	}
	m.settingsTool = clampIndex(m.settingsTool, len(names))
	name := names[m.settingsTool]
	tool := m.staged.Tools[name]
	b.WriteString(fmt.Sprintf("tool %d/%d\n\n", m.settingsTool+1, len(names)))
	fields := []string{
		"name: " + name,
		"window: " + tool.Window,
		"command: " + tool.Command,
		"after_exit: " + string(tool.AfterExit),
		"default_for_new_projects: " + strconv.FormatBool(tool.DefaultForNewProjects),
		"add tool",
		"delete tool",
		"projects",
		"save",
		"discard",
	}
	for i, row := range fields {
		writeSettingRow(&b, m.settingsCursor, i, row)
	}
	b.WriteString("\n")
	b.WriteString(mutedStyle.Render("enter edit/cycle  h/l tool  a add  d delete  tab projects  ctrl+s save  x discard"))
	return panelStyle.Width(m.contentWidth()).Render(b.String())
}

func (m Model) viewSettingsEdit() string {
	text := strings.Join([]string{
		titleStyle.Render("Edit " + m.editField),
		m.editValue,
		"",
		mutedStyle.Render("enter save field  esc cancel"),
	}, "\n")
	return panelStyle.Width(m.contentWidth()).Render(text)
}

func (m Model) viewKeybindings() string {
	var b strings.Builder
	b.WriteString(titleStyle.Render("tmux Key Bindings"))
	b.WriteString("\n")
	if m.keyStatus != "" {
		b.WriteString(m.keyStatus)
		b.WriteString("\n")
	}
	if len(m.keyBindings) > 0 {
		b.WriteString("F keys: ")
		var present []string
		for i := 1; i <= 12; i++ {
			key := "F" + strconv.Itoa(i)
			if hasKeyBinding(m.keyBindings, key) {
				present = append(present, key)
			}
		}
		if len(present) == 0 {
			b.WriteString("none")
		} else {
			b.WriteString(strings.Join(present, ", "))
		}
		b.WriteString("\n\n")
		for _, binding := range m.keyBindings {
			b.WriteString(fmt.Sprintf("%s  %s  %s\n", binding.Table, binding.Key, binding.Command))
		}
	}
	b.WriteString("\n")
	b.WriteString(mutedStyle.Render("r reload  esc back"))
	return panelStyle.Width(m.contentWidth()).Render(b.String())
}

func (m Model) projectRowCount() int {
	if len(m.staged.Projects) == 0 {
		return 4
	}
	return m.projectActionOffset(m.staged.Projects[m.settingsProject]) + 5
}

func (m Model) toolRowCount() int {
	if len(m.staged.Tools) == 0 {
		return 4
	}
	return toolFieldCount + 5
}

func projectFieldName(index int) string {
	return []string{"name", "path", "session", "default_window", "window_selection", "on_existing", "confirm_kill", "failure_policy"}[index]
}

func toolFieldName(index int) string {
	return []string{"name", "window", "command", "after_exit", "default_for_new_projects"}[index]
}

func projectFieldValue(project config.Project, field string) string {
	switch field {
	case "name":
		return project.Name
	case "path":
		return project.Path
	case "session":
		return project.Session
	case "default_window":
		return project.DefaultWindow
	default:
		return ""
	}
}

func toolFieldValue(name string, tool config.Tool, field string) string {
	switch field {
	case "name":
		return name
	case "window":
		return tool.Window
	case "command":
		return tool.Command
	default:
		return ""
	}
}

func writeSettingRow(b *strings.Builder, selected, index int, text string) {
	cursor := "  "
	style := normalStyle
	if selected == index {
		cursor = "> "
		style = selectedStyle
	}
	b.WriteString(style.Render(cursor + text))
	b.WriteString("\n")
}

func writeProjectToolRow(b *strings.Builder, selected, index int, tools map[string]config.Tool, row projectToolRow) {
	status := row.state
	style := accentStyle
	if row.state != "enabled" {
		style = mutedStyle
	}
	text := fmt.Sprintf("tool: %s  %s  window: %s", row.name, status, projectToolWindowName(tools, row.tool))
	if selected == index {
		style = selectedStyle
		text = "> " + text
	} else {
		text = "  " + text
	}
	b.WriteString(style.Render(text))
	b.WriteString("\n")
}

func (m Model) projectActionOffset(project config.Project) int {
	return projectFieldCount + len(projectToolRows(project, m.staged.Tools))
}

func projectToolEnabled(tool config.ProjectTool) bool {
	return tool.Override.Enabled == nil || *tool.Override.Enabled
}

func projectToolWindowName(tools map[string]config.Tool, projectTool config.ProjectTool) string {
	if projectTool.Override.Window != "" {
		return projectTool.Override.Window
	}
	if tool, ok := tools[projectTool.Name]; ok && tool.Window != "" {
		return tool.Window
	}
	return projectTool.Name
}

func projectToolRows(project config.Project, tools map[string]config.Tool) []projectToolRow {
	rows := make([]projectToolRow, 0, len(project.Tools)+len(tools))
	used := map[string]struct{}{}
	for i, tool := range project.Tools {
		state := "enabled"
		if !projectToolEnabled(tool) {
			state = "disabled"
		}
		rows = append(rows, projectToolRow{
			name:  tool.Name,
			index: i,
			tool:  tool,
			state: state,
		})
		used[tool.Name] = struct{}{}
	}
	for _, name := range sortedToolNames(tools) {
		if _, ok := used[name]; ok {
			continue
		}
		rows = append(rows, projectToolRow{
			name:  name,
			index: -1,
			tool:  config.ProjectTool{Name: name},
			state: "available",
		})
	}
	return rows
}

func defaultProjectTools(tools map[string]config.Tool) []config.ProjectTool {
	var defaults []config.ProjectTool
	for _, name := range sortedToolNames(tools) {
		if tools[name].DefaultForNewProjects {
			defaults = append(defaults, config.ProjectTool{Name: name})
		}
	}
	return defaults
}

func cloneConfig(in config.Config) (config.Config, error) {
	data, err := config.MarshalYAML(in)
	if err != nil {
		return config.Config{}, err
	}
	out, err := config.LoadYAML(data)
	if err != nil {
		return config.Config{}, err
	}
	return out, nil
}

func sortedToolNames(tools map[string]config.Tool) []string {
	names := make([]string, 0, len(tools))
	for name := range tools {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func uniqueProjectName(projects []config.Project, base string) string {
	used := map[string]struct{}{}
	for _, project := range projects {
		used[project.Name] = struct{}{}
	}
	return uniqueName(base, used)
}

func uniqueToolName(tools map[string]config.Tool, base string) string {
	used := map[string]struct{}{}
	for name := range tools {
		used[name] = struct{}{}
	}
	return uniqueName(base, used)
}

func uniqueName(base string, used map[string]struct{}) string {
	if _, exists := used[base]; !exists {
		return base
	}
	for i := 2; ; i++ {
		name := fmt.Sprintf("%s-%d", base, i)
		if _, exists := used[name]; !exists {
			return name
		}
	}
}

func toolIsReferenced(projects []config.Project, name string) bool {
	for _, project := range projects {
		for _, tool := range project.Tools {
			if tool.Name == name {
				return true
			}
		}
	}
	return false
}

func boolField(value *bool) string {
	if value == nil {
		return "default"
	}
	if *value {
		return "true"
	}
	return "false"
}

func hasKeyBinding(bindings []tmux.KeyBinding, key string) bool {
	for _, binding := range bindings {
		if binding.Key == key {
			return true
		}
	}
	return false
}

func indexOf(values []string, target string) int {
	for i, value := range values {
		if value == target {
			return i
		}
	}
	return 0
}

func clampIndex(index, length int) int {
	if length <= 0 {
		return 0
	}
	if index < 0 {
		return 0
	}
	if index >= length {
		return length - 1
	}
	return index
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func boolPtr(value bool) *bool {
	return &value
}
