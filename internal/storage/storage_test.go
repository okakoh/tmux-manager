package storage

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/okakoh/tmux-manager/internal/config"
)

func TestDefaultConfigPathHonorsXDGConfigHome(t *testing.T) {
	configHome := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", configHome)
	got, err := DefaultConfigPath()
	if err != nil {
		t.Fatalf("DefaultConfigPath() error = %v", err)
	}
	want := filepath.Join(configHome, AppDirName, ConfigFileName)
	if got != want {
		t.Fatalf("DefaultConfigPath() = %q, want %q", got, want)
	}
}

func TestLoadMissingConfig(t *testing.T) {
	store, err := New(filepath.Join(t.TempDir(), "config.yaml"))
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	_, err = store.Load()
	if !errors.Is(err, ErrConfigNotFound) {
		t.Fatalf("Load() error = %v, want ErrConfigNotFound", err)
	}
}

func TestSaveCreatesConfigAndBackup(t *testing.T) {
	dir := t.TempDir()
	projectDir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	store, err := New(path)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	store.Now = func() time.Time {
		return time.Date(2026, 5, 24, 12, 34, 56, 0, time.UTC)
	}

	cfg := validConfig(projectDir, "codex")
	backupPath, err := store.Save(cfg, config.ResolveOptions{RequireExistingProjectPaths: true})
	if err != nil {
		t.Fatalf("first Save() error = %v", err)
	}
	if backupPath != "" {
		t.Fatalf("first Save() backupPath = %q, want empty", backupPath)
	}
	firstData, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	if !strings.Contains(string(firstData), "codex") {
		t.Fatalf("saved config missing codex:\n%s", firstData)
	}

	cfg.Tools["codex"] = config.Tool{Window: "codex", Command: "codex --dangerously-skip-approvals", AfterExit: config.AfterExitShell}
	backupPath, err = store.Save(cfg, config.ResolveOptions{RequireExistingProjectPaths: true})
	if err != nil {
		t.Fatalf("second Save() error = %v", err)
	}
	if backupPath == "" {
		t.Fatal("second Save() backupPath empty")
	}
	backupData, err := os.ReadFile(backupPath)
	if err != nil {
		t.Fatalf("ReadFile(backup) error = %v", err)
	}
	if string(backupData) != string(firstData) {
		t.Fatalf("backup content mismatch")
	}
	backups, err := store.ListBackups()
	if err != nil {
		t.Fatalf("ListBackups() error = %v", err)
	}
	if len(backups) != 1 || backups[0].Path != backupPath {
		t.Fatalf("ListBackups() = %#v, want %q", backups, backupPath)
	}
}

func TestSaveCreatesPrivateConfigDirectory(t *testing.T) {
	root := t.TempDir()
	projectDir := t.TempDir()
	path := filepath.Join(root, "nested", "config.yaml")
	store, err := New(path)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	if _, err := store.Save(validConfig(projectDir, "codex"), config.ResolveOptions{RequireExistingProjectPaths: true}); err != nil {
		t.Fatalf("Save() error = %v", err)
	}
	info, err := os.Stat(filepath.Dir(path))
	if err != nil {
		t.Fatalf("Stat(config dir) error = %v", err)
	}
	if got := info.Mode().Perm(); got != 0o700 {
		t.Fatalf("config dir mode = %v, want 0700", got)
	}
}

func TestSaveCreatesUniqueBackupNamesWithinSameSecond(t *testing.T) {
	dir := t.TempDir()
	projectDir := t.TempDir()
	store, err := New(filepath.Join(dir, "config.yaml"))
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	store.Now = func() time.Time {
		return time.Date(2026, 5, 24, 12, 34, 56, 0, time.UTC)
	}

	cfg := validConfig(projectDir, "codex")
	if _, err := store.Save(cfg, config.ResolveOptions{RequireExistingProjectPaths: true}); err != nil {
		t.Fatalf("initial Save() error = %v", err)
	}
	if _, err := store.Save(cfg, config.ResolveOptions{RequireExistingProjectPaths: true}); err != nil {
		t.Fatalf("second Save() error = %v", err)
	}
	if _, err := store.Save(cfg, config.ResolveOptions{RequireExistingProjectPaths: true}); err != nil {
		t.Fatalf("third Save() error = %v", err)
	}
	backups, err := store.ListBackups()
	if err != nil {
		t.Fatalf("ListBackups() error = %v", err)
	}
	if len(backups) != 2 {
		t.Fatalf("backup count = %d, want 2", len(backups))
	}
	if backups[0].Path == backups[1].Path {
		t.Fatalf("backup paths are not unique: %#v", backups)
	}
}

func TestSaveRejectsInvalidConfigWithoutReplacingExisting(t *testing.T) {
	dir := t.TempDir()
	projectDir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	store, err := New(path)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	good := validConfig(projectDir, "codex")
	if _, err := store.Save(good, config.ResolveOptions{RequireExistingProjectPaths: true}); err != nil {
		t.Fatalf("Save(good) error = %v", err)
	}
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}

	bad := good
	bad.Projects[0].Path = filepath.Join(projectDir, "missing")
	if _, err := store.Save(bad, config.ResolveOptions{RequireExistingProjectPaths: true}); err == nil {
		t.Fatal("Save(bad) expected error")
	}
	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile() after error = %v", err)
	}
	if string(after) != string(before) {
		t.Fatal("invalid save replaced existing config")
	}
}

func validConfig(projectDir, command string) config.Config {
	return config.Config{
		Tools: map[string]config.Tool{
			"codex": {Window: "codex", Command: command, AfterExit: config.AfterExitShell},
		},
		Projects: []config.Project{{
			Name:          "tmux-manager",
			Path:          projectDir,
			DefaultWindow: "codex",
			Tools:         []config.ProjectTool{{Name: "codex"}},
		}},
	}
}
