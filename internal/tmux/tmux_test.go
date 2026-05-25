package tmux

import (
	"errors"
	"reflect"
	"strings"
	"testing"
)

func TestCommandBuilders(t *testing.T) {
	tests := []struct {
		name string
		got  []string
		want []string
	}{
		{
			name: "new session",
			got:  NewSessionArgs("sample-api", "yazi", "/Users/me/sample-api", `zsh -lc "yazi; exec zsh"`),
			want: []string{"-u", "new-session", "-d", "-s", "sample-api", "-n", "yazi", "-c", "/Users/me/sample-api", `zsh -lc "yazi; exec zsh"`},
		},
		{
			name: "new window",
			got:  NewWindowArgs("sample-api", "codex", "/Users/me/sample-api", `zsh -lc "codex; exec zsh"`),
			want: []string{"-u", "new-window", "-t", "sample-api:", "-n", "codex", "-c", "/Users/me/sample-api", `zsh -lc "codex; exec zsh"`},
		},
		{
			name: "select window",
			got:  SelectWindowArgs("sample-api", "codex"),
			want: []string{"-u", "select-window", "-t", "sample-api:codex"},
		},
		{
			name: "attach",
			got:  AttachSessionArgs("sample-api"),
			want: []string{"-u", "attach-session", "-d", "-t", "sample-api"},
		},
		{
			name: "kill",
			got:  KillSessionArgs("sample-api"),
			want: []string{"-u", "kill-session", "-t", "sample-api"},
		},
		{
			name: "list sessions",
			got:  ListSessionsArgs(),
			want: []string{"-u", "list-sessions", "-F", "#{session_name}\t#{session_windows}"},
		},
		{
			name: "list windows",
			got:  ListWindowsArgs("sample-api"),
			want: []string{"-u", "list-windows", "-t", "sample-api", "-F", "#{window_index}\t#{window_name}\t#{window_active}"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if !reflect.DeepEqual(tt.got, tt.want) {
				t.Fatalf("args = %#v, want %#v", tt.got, tt.want)
			}
		})
	}
}

func TestParseSessions(t *testing.T) {
	got, err := ParseSessions("sample-api\t4\nnotes\t1\n")
	if err != nil {
		t.Fatalf("ParseSessions() error = %v", err)
	}
	want := []Session{{Name: "sample-api", WindowCount: 4}, {Name: "notes", WindowCount: 1}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("ParseSessions() = %#v, want %#v", got, want)
	}
}

func TestParseSessionsRejectsBadOutput(t *testing.T) {
	_, err := ParseSessions("sample-api\tmany\n")
	var tmuxErr *Error
	if !errors.As(err, &tmuxErr) || tmuxErr.Kind != ErrorParseFailed {
		t.Fatalf("ParseSessions() error = %T %[1]v, want parse error", err)
	}
}

func TestErrorIncludesCommandAndOutput(t *testing.T) {
	err := (&Error{
		Kind:   ErrorCommandFailed,
		Args:   []string{"tmux", "-u", "attach-session", "-t", "sample-api"},
		Output: "open terminal failed: not a terminal\n",
		Err:    errors.New("exit status 1"),
	}).Error()
	for _, want := range []string{"command-failed", "exit status 1", "attach-session", "open terminal failed"} {
		if !strings.Contains(err, want) {
			t.Fatalf("Error() = %q, want %q", err, want)
		}
	}
}

func TestParseWindows(t *testing.T) {
	got, err := ParseWindows("0\tyazi\t0\n1\tcodex\t1\n")
	if err != nil {
		t.Fatalf("ParseWindows() error = %v", err)
	}
	want := []Window{{Index: 0, Name: "yazi", Active: false}, {Index: 1, Name: "codex", Active: true}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("ParseWindows() = %#v, want %#v", got, want)
	}
}

func TestParseKeyBindingsToleratesUnknownLines(t *testing.T) {
	got := ParseKeyBindings("not-a-binding\nbind-key -T prefix F1 select-window -t :1\nbind-key C-b send-prefix\n")
	want := []KeyBinding{
		{Table: "prefix", Key: "F1", Command: "select-window -t :1"},
		{Table: "prefix", Key: "C-b", Command: "send-prefix"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("ParseKeyBindings() = %#v, want %#v", got, want)
	}
}
