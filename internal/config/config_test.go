package config

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadResolveDefaultsAndOverrides(t *testing.T) {
	projectDir := t.TempDir()
	raw := []byte(`
tools:
  codex:
    window: codex
    command: codex
    after_exit: shell
  yazi:
    window: yazi
    command: yazi
    default_for_new_projects: true
projects:
  - name: tmux-manager
    path: ` + projectDir + `
    default_window: assistant
    tools:
      - yazi
      - codex:
          window: assistant
          command: codex --ask-for-approval never
          env:
            CODEX_HOME: /tmp/codex
`)

	cfg, err := LoadYAML(raw)
	if err != nil {
		t.Fatalf("LoadYAML() error = %v", err)
	}
	cfg.TmuxBinary = "/opt/homebrew/Cellar/tmux/3.5a/bin/tmux"
	data, err := MarshalYAML(cfg)
	if err != nil {
		t.Fatalf("MarshalYAML() error = %v", err)
	}
	if !strings.Contains(string(data), "tmux_binary: /opt/homebrew/Cellar/tmux/3.5a/bin/tmux") {
		t.Fatalf("marshaled config missing tmux_binary:\n%s", data)
	}
	resolved, err := Resolve(cfg, ResolveOptions{RequireExistingProjectPaths: true})
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	project := resolved.Projects[0]
	if project.Session != "tmux-manager" {
		t.Fatalf("Session default = %q", project.Session)
	}
	if project.ConfirmKill != true {
		t.Fatal("ConfirmKill should default true")
	}
	if project.OnExisting != ExistingPolicyAttach {
		t.Fatalf("OnExisting default = %q", project.OnExisting)
	}
	if got := project.Windows[1].Command; got != "codex --ask-for-approval never" {
		t.Fatalf("override command = %q", got)
	}
	if got := project.Windows[1].Window; got != "assistant" {
		t.Fatalf("override window = %q", got)
	}
	if got := project.Windows[1].Env["CODEX_HOME"]; got != "/tmp/codex" {
		t.Fatalf("override env = %q", got)
	}
	if !resolved.Tools["yazi"].DefaultForNewProjects {
		t.Fatal("resolved yazi should keep default_for_new_projects")
	}
}

func TestResolveRejectsInvalidConfig(t *testing.T) {
	raw := []byte(`
tools:
  codex:
    command: ""
projects:
  - name: duplicate
    path: /tmp
    default_window: missing
    on_existing: invalid
    tools:
      - unknown
  - name: duplicate
    path: /tmp
    tools: []
`)
	cfg, err := LoadYAML(raw)
	if err != nil {
		t.Fatalf("LoadYAML() error = %v", err)
	}
	_, err = Resolve(cfg, ResolveOptions{})
	var validationErr *ValidationError
	if !errors.As(err, &validationErr) {
		t.Fatalf("Resolve() error = %T %v, want ValidationError", err, err)
	}
	joined := strings.Join(validationErr.Problems, "\n")
	for _, want := range []string{"command must not be empty", "invalid on_existing", "must reference a global tool", "duplicated"} {
		if !strings.Contains(joined, want) {
			t.Fatalf("validation problems missing %q in:\n%s", want, joined)
		}
	}
}

func TestResolveRejectsInvalidEnvNames(t *testing.T) {
	cfg := Config{
		Tools: map[string]Tool{
			"codex": {
				Command: "codex",
				Env:     map[string]string{"BAD-NAME": "value"},
			},
		},
		Projects: []Project{{
			Name:  "sample",
			Path:  "/tmp",
			Tools: []ProjectTool{{Name: "codex"}},
		}},
	}
	_, err := Resolve(cfg, ResolveOptions{})
	var validationErr *ValidationError
	if !errors.As(err, &validationErr) {
		t.Fatalf("Resolve() error = %T %v, want ValidationError", err, err)
	}
	joined := strings.Join(validationErr.Problems, "\n")
	if !strings.Contains(joined, "BAD-NAME") {
		t.Fatalf("validation problems missing env name:\n%s", joined)
	}
}

func TestResolveRejectsUnsafeTmuxTargetNames(t *testing.T) {
	cfg := Config{
		Tools: map[string]Tool{
			"bad:tool": {Command: "codex"},
		},
		Projects: []Project{{
			Name:    "sample",
			Path:    "/tmp",
			Session: "sample:api",
			Tools: []ProjectTool{{
				Name: "bad:tool",
				Override: ToolOverride{
					Window: "bad:window",
				},
			}},
		}},
	}
	_, err := Resolve(cfg, ResolveOptions{})
	var validationErr *ValidationError
	if !errors.As(err, &validationErr) {
		t.Fatalf("Resolve() error = %T %v, want ValidationError", err, err)
	}
	joined := strings.Join(validationErr.Problems, "\n")
	for _, want := range []string{"session", "window", "must not contain ':'"} {
		if !strings.Contains(joined, want) {
			t.Fatalf("validation problems missing %q in:\n%s", want, joined)
		}
	}
}

func TestResolveCanRequireExistingProjectPaths(t *testing.T) {
	cfg := Config{
		Tools: map[string]Tool{"codex": {Command: "codex"}},
		Projects: []Project{{
			Name:  "missing",
			Path:  filepath.Join(t.TempDir(), "does-not-exist"),
			Tools: []ProjectTool{{Name: "codex"}},
		}},
	}
	if _, err := Resolve(cfg, ResolveOptions{}); err != nil {
		t.Fatalf("Resolve without path requirement error = %v", err)
	}
	_, err := Resolve(cfg, ResolveOptions{RequireExistingProjectPaths: true})
	if err == nil {
		t.Fatal("Resolve with path requirement expected error")
	}
}

func TestExpandPath(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatalf("UserHomeDir() error = %v", err)
	}
	got, err := ExpandPath("~/project")
	if err != nil {
		t.Fatalf("ExpandPath() error = %v", err)
	}
	want := filepath.Join(home, "project")
	if got != want {
		t.Fatalf("ExpandPath() = %q, want %q", got, want)
	}
}

func TestBuildShellCommand(t *testing.T) {
	got := BuildShellCommand("codex --ask-for-approval never", AfterExitShell)
	want := `sh -lc "codex --ask-for-approval never; exec sh"`
	if got != want {
		t.Fatalf("BuildShellCommand() = %q, want %q", got, want)
	}
}

func TestBuildShellCommandWithShell(t *testing.T) {
	got := BuildShellCommandWithShell("codex", AfterExitShell, "/bin/zsh")
	want := `/bin/zsh -lc "codex; exec /bin/zsh"`
	if got != want {
		t.Fatalf("BuildShellCommandWithShell() = %q, want %q", got, want)
	}
}

func TestBuildShellCommandWithShellAndEnv(t *testing.T) {
	t.Setenv("HOME", "/Users/me")
	t.Setenv("PATH", "/usr/bin:/bin")
	env := buildWindowEnv(map[string]string{"PATH": "$HOME/.opencode/bin:$PATH"})
	got := BuildShellCommandWithShellAndEnv("opencode", AfterExitShell, "/bin/zsh", env)
	want := `/bin/zsh -lc "export PATH='/Users/me/.opencode/bin:/usr/bin:/bin'; opencode; exec /bin/zsh"`
	if got != want {
		t.Fatalf("BuildShellCommandWithShellAndEnv() = %q, want %q", got, want)
	}
}

func TestResolveAddsProcessPathToWindowEnv(t *testing.T) {
	t.Setenv("PATH", "/custom/bin:/usr/bin")
	cfg := Config{
		Tools: map[string]Tool{"codex": {Command: "codex"}},
		Projects: []Project{{
			Name:  "sample",
			Path:  "/tmp",
			Tools: []ProjectTool{{Name: "codex"}},
		}},
	}
	resolved, err := Resolve(cfg, ResolveOptions{})
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	window := resolved.Projects[0].Windows[0]
	if got := window.Env["PATH"]; got != "/custom/bin:/usr/bin" {
		t.Fatalf("window PATH = %q", got)
	}
	if !strings.Contains(window.ShellCommand, "export PATH='/custom/bin:/usr/bin'") {
		t.Fatalf("ShellCommand missing PATH export: %q", window.ShellCommand)
	}
}

func TestResolveShellPathFallsBackToSh(t *testing.T) {
	got, err := ResolveShellPathFromEnv(filepath.Join(t.TempDir(), "missing-shell"))
	if err != nil {
		t.Fatalf("ResolveShellPathFromEnv() error = %v", err)
	}
	if filepath.Base(got) != DefaultShell {
		t.Fatalf("ResolveShellPathFromEnv() = %q, want fallback shell %q", got, DefaultShell)
	}
}
