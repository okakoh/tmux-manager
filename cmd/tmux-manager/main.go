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

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run() error {
	configPath := flag.String("config", "", "config file path")
	flag.Parse()

	store, err := storage.New(*configPath)
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
	resolved, err := config.Resolve(raw, config.ResolveOptions{})
	if err != nil {
		return err
	}

	tmuxClient := tmux.NewClient("")
	state, err := tmuxClient.Snapshot(context.Background())
	if err != nil {
		state = tmux.State{}
	}
	return tui.RunWithServices(raw, resolved, state, nil, store, tmuxClient)
}
