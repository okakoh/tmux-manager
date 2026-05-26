package storage

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"time"

	"github.com/okakoh/tmux-manager/internal/config"
)

const (
	AppDirName     = "tmux-manager"
	ConfigFileName = "config.yaml"
)

var ErrConfigNotFound = errors.New("config not found")

type Store struct {
	ConfigPath string
	Now        func() time.Time
}

type Backup struct {
	Path    string
	ModTime time.Time
}

func DefaultConfigPath() (string, error) {
	if xdg := os.Getenv("XDG_CONFIG_HOME"); xdg != "" {
		return filepath.Join(xdg, AppDirName, ConfigFileName), nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".config", AppDirName, ConfigFileName), nil
}

func New(path string) (*Store, error) {
	if path == "" {
		var err error
		path, err = DefaultConfigPath()
		if err != nil {
			return nil, err
		}
	}
	return &Store{ConfigPath: path, Now: time.Now}, nil
}

func (s *Store) Load() (config.Config, error) {
	data, err := os.ReadFile(s.ConfigPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return config.Config{}, ErrConfigNotFound
		}
		return config.Config{}, err
	}
	return config.LoadYAML(data)
}

func (s *Store) Save(cfg config.Config, opts config.ResolveOptions) (string, error) {
	if _, err := config.Resolve(cfg, opts); err != nil {
		return "", err
	}
	data, err := config.MarshalYAML(cfg)
	if err != nil {
		return "", err
	}
	dir := filepath.Dir(s.ConfigPath)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", err
	}

	backupPath := ""
	if _, err := os.Stat(s.ConfigPath); err == nil {
		backupPath = s.backupPath()
		if err := copyFileInDir(dir, filepath.Base(s.ConfigPath), filepath.Base(backupPath)); err != nil {
			return "", err
		}
	} else if err != nil && !errors.Is(err, os.ErrNotExist) {
		return "", err
	}

	temp, err := os.CreateTemp(dir, ".config-*.yaml")
	if err != nil {
		return "", err
	}
	tempPath := temp.Name()
	cleanupTemp := true
	defer func() {
		if cleanupTemp {
			_ = os.Remove(tempPath)
		}
	}()

	if _, err := temp.Write(data); err != nil {
		_ = temp.Close()
		return "", err
	}
	if err := temp.Chmod(0o600); err != nil {
		_ = temp.Close()
		return "", err
	}
	if err := temp.Close(); err != nil {
		return "", err
	}
	if err := os.Rename(tempPath, s.ConfigPath); err != nil {
		return "", err
	}
	cleanupTemp = false
	return backupPath, nil
}

func (s *Store) ListBackups() ([]Backup, error) {
	pattern := filepath.Join(filepath.Dir(s.ConfigPath), backupPrefix()+"*.yaml")
	matches, err := filepath.Glob(pattern)
	if err != nil {
		return nil, err
	}
	backups := make([]Backup, 0, len(matches))
	for _, match := range matches {
		info, err := os.Stat(match)
		if err != nil {
			continue
		}
		backups = append(backups, Backup{Path: match, ModTime: info.ModTime()})
	}
	sort.Slice(backups, func(i, j int) bool {
		return backups[i].ModTime.After(backups[j].ModTime)
	})
	return backups, nil
}

func (s *Store) backupPath() string {
	now := time.Now
	if s.Now != nil {
		now = s.Now
	}
	stamp := now().Format("20060102-150405")
	dir := filepath.Dir(s.ConfigPath)
	candidate := filepath.Join(dir, fmt.Sprintf("%s%s.yaml", backupPrefix(), stamp))
	if _, err := os.Stat(candidate); errors.Is(err, os.ErrNotExist) {
		return candidate
	}
	for i := 1; ; i++ {
		candidate = filepath.Join(dir, fmt.Sprintf("%s%s-%02d.yaml", backupPrefix(), stamp, i))
		if _, err := os.Stat(candidate); errors.Is(err, os.ErrNotExist) {
			return candidate
		}
	}
}

func backupPrefix() string {
	return ConfigFileName + ".bak-"
}

func copyFileInDir(dir, srcName, dstName string) error {
	root, err := os.OpenRoot(dir)
	if err != nil {
		return err
	}
	defer root.Close()

	in, err := root.Open(srcName)
	if err != nil {
		return err
	}
	defer in.Close()

	out, err := root.OpenFile(dstName, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return err
	}
	defer out.Close()

	if _, err := io.Copy(out, in); err != nil {
		return err
	}
	return out.Sync()
}
