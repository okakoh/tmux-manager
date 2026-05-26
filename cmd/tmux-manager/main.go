package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"

	"github.com/okakoh/tmux-manager/internal/config"
	"github.com/okakoh/tmux-manager/internal/storage"
	"github.com/okakoh/tmux-manager/internal/tmux"
	"github.com/okakoh/tmux-manager/internal/tui"
)

var version = "0.1.1"

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run() error {
	configPath := flag.String("config", "", "config file path")
	showVersion := flag.Bool("version", false, "print version and exit")
	flag.Parse()
	if *showVersion {
		fmt.Fprintf(os.Stdout, "tmux-manager %s\n", version)
		return nil
	}

	store, err := storage.New(*configPath)
	if err != nil {
		return err
	}
	tmuxBinary, err := tmux.ResolveBinary("")
	if err != nil {
		return err
	}
	shell, err := config.ResolveShellPath()
	if err != nil {
		return err
	}
	raw, err := store.Load()
	if err != nil {
		if errors.Is(err, storage.ErrConfigNotFound) {
			raw = config.Config{}
		} else {
			return err
		}
	}
	resolved, err := config.Resolve(raw, config.ResolveOptions{Shell: shell})
	if err != nil {
		return err
	}

	tmuxClient := tmux.NewClient(tmuxBinary)
	state, err := tmuxClient.Snapshot(context.Background())
	if err != nil {
		state = tmux.State{}
	}
	return tui.RunWithServicesAndShell(raw, resolved, state, nil, store, tmuxClient, shell)
}
