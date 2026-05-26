package tmux

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"strconv"
	"strings"
)

const DefaultBinary = "tmux"

type ErrorKind string

const (
	ErrorMissingExecutable ErrorKind = "missing-executable"
	ErrorMissingSession    ErrorKind = "missing-session"
	ErrorCommandFailed     ErrorKind = "command-failed"
	ErrorParseFailed       ErrorKind = "parse-failed"
)

type Error struct {
	Kind   ErrorKind
	Args   []string
	Output string
	Err    error
}

func (e *Error) Error() string {
	if e == nil {
		return "<nil>"
	}
	var details []string
	if len(e.Args) > 0 {
		details = append(details, strings.Join(e.Args, " "))
	}
	if output := strings.TrimSpace(e.Output); output != "" {
		details = append(details, output)
	}
	if len(details) > 0 {
		if e.Err != nil {
			return fmt.Sprintf("tmux %s: %v (%s)", e.Kind, e.Err, strings.Join(details, ": "))
		}
		return fmt.Sprintf("tmux %s: %s", e.Kind, strings.Join(details, ": "))
	}
	if e.Err != nil {
		return fmt.Sprintf("tmux %s: %v", e.Kind, e.Err)
	}
	return "tmux " + string(e.Kind)
}

func (e *Error) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Err
}

type State struct {
	Sessions []Session
}

type Session struct {
	Name        string
	WindowCount int
	Windows     []Window
}

type Window struct {
	Index  int
	Name   string
	Active bool
}

type KeyBinding struct {
	Table   string
	Key     string
	Command string
}

type VersionDiagnostic struct {
	Binary        string
	ClientVersion string
	ServerVersion string
}

type Client struct {
	Binary string
}

func NewClient(binary string) Client {
	if binary == "" {
		binary = DefaultBinary
	}
	return Client{Binary: binary}
}

func ResolveBinary(binary string) (string, error) {
	if binary == "" {
		binary = DefaultBinary
	}
	path, err := exec.LookPath(binary)
	if err != nil {
		return "", &Error{Kind: ErrorMissingExecutable, Args: []string{binary}, Err: err}
	}
	return path, nil
}

func (s State) HasSession(name string) bool {
	_, ok := s.FindSession(name)
	return ok
}

func (s State) FindSession(name string) (Session, bool) {
	for _, session := range s.Sessions {
		if session.Name == name {
			return session, true
		}
	}
	return Session{}, false
}

func (c Client) ListSessions(ctx context.Context) ([]Session, error) {
	output, err := c.run(ctx, ListSessionsArgs())
	if err != nil {
		return nil, err
	}
	return ParseSessions(output)
}

func (c Client) ListWindows(ctx context.Context, session string) ([]Window, error) {
	output, err := c.run(ctx, ListWindowsArgs(session))
	if err != nil {
		return nil, err
	}
	return ParseWindows(output)
}

func (c Client) Snapshot(ctx context.Context) (State, error) {
	sessions, err := c.ListSessions(ctx)
	if err != nil {
		return State{}, err
	}
	for i := range sessions {
		windows, err := c.ListWindows(ctx, sessions[i].Name)
		if err != nil {
			return State{}, err
		}
		sessions[i].Windows = windows
	}
	return State{Sessions: sessions}, nil
}

func (c Client) HasSession(ctx context.Context, session string) (bool, error) {
	_, err := c.run(ctx, HasSessionArgs(session))
	if err == nil {
		return true, nil
	}
	var tmuxErr *Error
	if errors.As(err, &tmuxErr) && tmuxErr.Kind == ErrorMissingSession {
		return false, nil
	}
	return false, err
}

func (c Client) NewSession(ctx context.Context, session, window, cwd, shellCommand string) error {
	_, err := c.run(ctx, NewSessionArgs(session, window, cwd, shellCommand))
	return err
}

func (c Client) NewWindow(ctx context.Context, session, window, cwd, shellCommand string) error {
	_, err := c.run(ctx, NewWindowArgs(session, window, cwd, shellCommand))
	return err
}

func (c Client) SelectWindow(ctx context.Context, session, window string) error {
	_, err := c.run(ctx, SelectWindowArgs(session, window))
	return err
}

func (c Client) AttachSession(ctx context.Context, session string) error {
	_, err := c.run(ctx, AttachSessionArgs(session))
	return err
}

func (c Client) KillSession(ctx context.Context, session string) error {
	_, err := c.run(ctx, KillSessionArgs(session))
	return err
}

func (c Client) ListKeys(ctx context.Context) ([]KeyBinding, error) {
	output, err := c.run(ctx, ListKeysArgs())
	if err != nil {
		return nil, err
	}
	return ParseKeyBindings(output), nil
}

func (c Client) VersionDiagnostic(ctx context.Context) (VersionDiagnostic, error) {
	binary := c.Binary
	if binary == "" {
		binary = DefaultBinary
	}
	resolved, err := ResolveBinary(binary)
	if err != nil {
		return VersionDiagnostic{}, err
	}
	clientOutput, err := runCommand(ctx, resolved, VersionArgs())
	if err != nil {
		return VersionDiagnostic{}, err
	}
	serverOutput, err := runCommand(ctx, resolved, ServerVersionArgs())
	if err != nil {
		return VersionDiagnostic{}, err
	}
	return VersionDiagnostic{
		Binary:        resolved,
		ClientVersion: ParseClientVersion(clientOutput),
		ServerVersion: strings.TrimSpace(serverOutput),
	}, nil
}

func (d VersionDiagnostic) Mismatch() bool {
	return d.ClientVersion != "" && d.ServerVersion != "" && d.ClientVersion != d.ServerVersion
}

func (d VersionDiagnostic) Message() string {
	return fmt.Sprintf(`tmux-manager does not require a specific tmux version.
However, it is currently using a tmux client that differs from the running tmux server:

  tmux binary: %s
  tmux client version: %s
  tmux server version: %s

tmux attach can fail when the selected client and the running server for the same socket differ.
Use a tmux binary matching the server, or restart the tmux server after intentionally upgrading tmux.`, d.Binary, d.ClientVersion, d.ServerVersion)
}

func (c Client) run(ctx context.Context, args []string) (string, error) {
	binary := c.Binary
	if binary == "" {
		binary = DefaultBinary
	}
	resolved, err := ResolveBinary(binary)
	if err != nil {
		return "", err
	}
	return runCommand(ctx, resolved, args)
}

func runCommand(ctx context.Context, binary string, args []string) (string, error) {
	cmd := exec.CommandContext(ctx, binary, args...) // #nosec G204 -- binary is resolved during startup and args are fixed tmux argv.
	output, err := cmd.CombinedOutput()
	if err == nil {
		return string(output), nil
	}
	kind := ErrorCommandFailed
	text := string(output)
	if strings.Contains(strings.ToLower(text), "can't find session") {
		kind = ErrorMissingSession
	}
	return text, &Error{Kind: kind, Args: append([]string{binary}, args...), Output: text, Err: err}
}

func VersionArgs() []string {
	return []string{"-V"}
}

func ServerVersionArgs() []string {
	return []string{"-u", "display-message", "-p", "#{version}"}
}

func HasSessionArgs(session string) []string {
	return []string{"-u", "has-session", "-t", session}
}

func ListSessionsArgs() []string {
	return []string{"-u", "list-sessions", "-F", "#{session_name}\t#{session_windows}"}
}

func ListWindowsArgs(session string) []string {
	return []string{"-u", "list-windows", "-t", session, "-F", "#{window_index}\t#{window_name}\t#{window_active}"}
}

func NewSessionArgs(session, window, cwd, shellCommand string) []string {
	return []string{"-u", "new-session", "-d", "-s", session, "-n", window, "-c", cwd, shellCommand}
}

func NewWindowArgs(session, window, cwd, shellCommand string) []string {
	return []string{"-u", "new-window", "-t", session + ":", "-n", window, "-c", cwd, shellCommand}
}

func SelectWindowArgs(session, window string) []string {
	return []string{"-u", "select-window", "-t", session + ":" + window}
}

func AttachSessionArgs(session string) []string {
	return []string{"-u", "attach-session", "-d", "-t", session}
}

func SwitchClientArgs(session string) []string {
	return []string{"-u", "switch-client", "-t", session}
}

func EnterSessionArgs(session string, insideTmux bool) []string {
	if insideTmux {
		return SwitchClientArgs(session)
	}
	return AttachSessionArgs(session)
}

func KillSessionArgs(session string) []string {
	return []string{"-u", "kill-session", "-t", session}
}

func ListKeysArgs() []string {
	return []string{"-u", "list-keys"}
}

func ParseSessions(output string) ([]Session, error) {
	lines := strings.Split(strings.TrimSpace(output), "\n")
	if len(lines) == 1 && lines[0] == "" {
		return nil, nil
	}
	sessions := make([]Session, 0, len(lines))
	for _, line := range lines {
		fields := strings.Split(line, "\t")
		if len(fields) != 2 {
			return nil, &Error{Kind: ErrorParseFailed, Output: output, Err: fmt.Errorf("session line has %d fields", len(fields))}
		}
		count, err := strconv.Atoi(fields[1])
		if err != nil {
			return nil, &Error{Kind: ErrorParseFailed, Output: output, Err: fmt.Errorf("invalid session window count %q", fields[1])}
		}
		sessions = append(sessions, Session{Name: fields[0], WindowCount: count})
	}
	return sessions, nil
}

func ParseClientVersion(output string) string {
	version := strings.TrimSpace(output)
	version = strings.TrimPrefix(version, "tmux ")
	return strings.TrimSpace(version)
}

func ParseWindows(output string) ([]Window, error) {
	lines := strings.Split(strings.TrimSpace(output), "\n")
	if len(lines) == 1 && lines[0] == "" {
		return nil, nil
	}
	windows := make([]Window, 0, len(lines))
	for _, line := range lines {
		fields := strings.Split(line, "\t")
		if len(fields) != 3 {
			return nil, &Error{Kind: ErrorParseFailed, Output: output, Err: fmt.Errorf("window line has %d fields", len(fields))}
		}
		index, err := strconv.Atoi(fields[0])
		if err != nil {
			return nil, &Error{Kind: ErrorParseFailed, Output: output, Err: fmt.Errorf("invalid window index %q", fields[0])}
		}
		windows = append(windows, Window{Index: index, Name: fields[1], Active: fields[2] == "1"})
	}
	return windows, nil
}

func ParseKeyBindings(output string) []KeyBinding {
	var bindings []KeyBinding
	for _, line := range strings.Split(output, "\n") {
		fields := strings.Fields(line)
		if len(fields) == 0 || fields[0] != "bind-key" {
			continue
		}
		table := "prefix"
		i := 1
		if len(fields) > 3 && fields[1] == "-T" {
			table = fields[2]
			i = 3
		}
		if i >= len(fields) {
			continue
		}
		key := fields[i]
		command := strings.Join(fields[i+1:], " ")
		bindings = append(bindings, KeyBinding{Table: table, Key: key, Command: command})
	}
	return bindings
}
