# tmux-manager

`tmux-manager` is a Go TUI for starting, re-entering, and safely rebuilding
project-specific tmux workspaces.

This project started as a personal tool and is published while it is still in
active development. Expect rough edges, small breaking changes, and documentation
that may lag behind the implementation. Feedback from similar day-to-day
development setups is welcome.

It is designed for development setups where one repository usually needs
several repeatable tmux windows: an editor, a shell, a test runner, logs, or
AI coding tools. Instead of keeping this layout in shell history or tmux muscle
memory, `tmux-manager` stores it as a small YAML config and gives you a focused
terminal UI for launching and attaching to those workspaces.

Japanese issues and discussions are welcome. / 日本語での Issue や相談も歓迎です。
See [README.ja.md](README.ja.md) for the Japanese README.

## Design Philosophy

- **Project first**: sessions are modeled as named projects, not as loose tmux
  commands.
- **Config over hidden state**: reusable tools and project layouts live in a
  readable YAML file.
- **Safe by default**: session recreation and killing can require confirmation.
- **Plain tmux underneath**: the app shells out to the installed `tmux` CLI and
  does not write `.tmux.conf`.
- **Single binary**: the app is distributed as a small Go command with no daemon
  or background service.

## Use Case: Next.js Project Workspaces

When you work across several projects, even a single Next.js app can require a
small stack of terminal processes before real work starts:

- a development server such as `pnpm dev` or `next dev`
- Codex, Claude Code, Hermes Agent, or another AI coding agent
- a normal shell for Git, package scripts, and one-off commands
- a file manager such as `yazi`, including when you inspect files from a phone
  or a remote terminal
- logs, tests, database consoles, or other project-specific tools

The value of `tmux-manager` is that this setup becomes a project entry instead
of a repeated manual ritual. You choose the project in the TUI, launch or attach
to its tmux session, and get the same named windows every time. If the session
already exists, the project can attach, prompt, or rebuild based on its policy.
If a process exits, the window can stay open in a shell so the failure is still
visible.

For a Next.js app, a config might look like this:

```yaml
tools:
  server:
    window: server
    command: pnpm dev
    after_exit: shell
  codex:
    window: codex
    command: codex
    after_exit: shell
  claude:
    window: claude
    command: claude
    after_exit: shell
  hermes:
    window: hermes
    command: hermes
    after_exit: shell
  files:
    window: files
    command: yazi
    after_exit: shell

projects:
  - name: sample-next-app
    path: ~/src/sample-next-app
    session: sample-next-app
    default_window: server
    window_selection: prompt
    on_existing: prompt
    confirm_kill: true
    failure_policy: continue
    tools:
      - server
      - codex
      - claude:
          enabled: false
      - hermes:
          enabled: false
      - files
```

The same reusable tools can then be shared by other projects while each project
keeps its own session name, default window, enabled tools, and safety policy.

## Terms

- **Project**: a workspace entry with a path, tmux session name, default window,
  and selected tools.
- **Tool**: a reusable window definition such as `editor`, `shell`, `tests`, or
  `logs`.
- **Window**: the tmux window created for a tool.
- **Session**: the tmux session owned by a project.
- **Default window**: the window selected before attaching to a session.
- **Existing session policy**: what to do when the target tmux session already
  exists: attach, prompt, or recreate.

## Requirements

- Go 1.26.3 or a newer compatible Go toolchain
- `tmux` on `PATH`
- a POSIX-style shell from `$SHELL`, or `sh` on `PATH`

The command wrapper runs tool commands through the resolved shell with `-lc`.

## Installation

### Go Install

Once the repository is published, install the latest release with:

```sh
go install github.com/okakoh/tmux-manager/cmd/tmux-manager@latest
```

Make sure Go's install directory is on your `PATH`. It is usually:

```sh
$(go env GOPATH)/bin
```

### From Source

```sh
git clone https://github.com/okakoh/tmux-manager.git
cd tmux-manager
go build -o ./tmux-manager ./cmd/tmux-manager
./tmux-manager
```

Check the installed version with:

```sh
tmux-manager -version
```

### Homebrew

Homebrew packaging is planned, but not available yet.

## Configuration

By default, `tmux-manager` reads:

```text
$XDG_CONFIG_HOME/tmux-manager/config.yaml
```

If `XDG_CONFIG_HOME` is not set, it reads:

```text
~/.config/tmux-manager/config.yaml
```

If the config file does not exist, the TUI starts with an empty project list.
Open settings with `s`, add projects/tools, and save with `Ctrl+S`.

Treat config files as trusted input. Tool commands are executable shell commands,
so do not run configs copied from untrusted sources without reviewing them.

To start from the sample config:

```sh
mkdir -p ~/.config/tmux-manager
cp examples/config.yaml ~/.config/tmux-manager/config.yaml
```

Then edit the sample project paths before launching.

You can also pass a config path explicitly:

```sh
tmux-manager -config examples/config.yaml
```

## Example

```yaml
tools:
  editor:
    window: editor
    command: nvim
    after_exit: shell
  shell:
    window: shell
    command: sh
    after_exit: shell

projects:
  - name: sample-api
    path: ~/src/sample-api
    session: sample-api
    default_window: editor
    window_selection: configured
    on_existing: attach
    confirm_kill: true
    failure_policy: stop
    tools:
      - editor
      - shell
```

Each tool command currently runs through the resolved shell:

```sh
sh -lc "<command>; exec sh"
```

so the shell remains open after the tool exits.

## TUI Usage

Home:

- `Enter`: launch a stopped project or attach to a running project
- `r`: recreate the selected session
- `k`: kill the selected session
- `w`: choose a target window before launching/attaching
- `/`: filter projects
- `s`: open settings
- `b`: view tmux key bindings
- `?`: help
- `q`: quit

Settings:

- `Tab`: switch between project and global tool editors
- `Up/Down` or `j/k`: move between fields
- `Left/Right` or `h/l`: switch selected project/tool
- `Enter`: edit or cycle the selected field
- `Enter` or `Space` on a project tool row: add or enable/disable a tool
- `d` on a project tool row: remove that project's tool reference
- `a`: add project/tool
- `d`: delete project/tool when an action row is selected
- `Ctrl+S`: validate and save
- `x`, `Esc`, or `q`: discard staged changes

Key binding view:

- `b`: reload bindings
- `q` or `Esc`: return home

The key binding view calls `tmux list-keys`; it does not modify tmux
configuration.

## Policies

`window_selection`:

- `configured`: use `default_window`
- `prompt`: ask for a window in the TUI before running the action

`on_existing`:

- `attach`: attach to the existing session
- `prompt`: ask whether to attach or recreate
- `recreate`: kill and rebuild the session

`failure_policy`:

- `stop`: stop at the first failed tmux step
- `continue`: keep going after non-final window creation failures and report
  partial success

## Development

```sh
go test ./...
go vet ./...
go build ./cmd/tmux-manager
```

Optional live tmux checks should use isolated session names and clean up after
themselves. Do not run them against important existing sessions.

## Contributing

Issues, bug reports, feature requests, config examples, and documentation fixes
are welcome. Please include:

- your OS and tmux version
- the `tmux-manager` version or commit
- a minimal config snippet when reporting config or launch behavior
- the expected behavior and actual behavior

日本語での報告も歓迎です。必要であれば、英語のタイトルだけ短く付けて本文は日本語で書いてください。

## Privacy

Do not commit personal project paths, private command arguments, API keys, or
local config backups. Use `~/.config/tmux-manager/config.yaml` for your real
configuration and keep `examples/` generic.

## License

MIT
