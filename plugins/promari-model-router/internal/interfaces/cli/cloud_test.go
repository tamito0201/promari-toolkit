package cli_test

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"promari-model-router/internal/domain/model"
)

// fakeClaude writes a stand-in for the Claude Code CLI that records its
// arguments and standard input next to itself and answers like `claude -p
// --cloud <id> --output-format json`.
func fakeClaude(t *testing.T, dir, name string) (bin, record string) {
	t.Helper()
	bin, record = filepath.Join(dir, name), filepath.Join(dir, name+".log")
	script := "#!/bin/sh\n{ echo \"$@\"; cat; } > '" + record + "'\nprintf '{\"ok\":true,\"url\":\"https://claude.ai/code/%s\"}' \"$3\"\n"
	if err := os.WriteFile(bin, []byte(script), 0o700); err != nil {
		t.Fatal(err)
	}
	return bin, record
}

func TestCloudCommands(t *testing.T) {
	const id = "session_017cB1aBqhcEMKwqXEK2d4A9"
	tmp := isolate(t, "")
	bin, record := fakeClaude(t, tmp, "claude-user")
	// A project file arrives with a cloned repository: it must not name the
	// program pmr runs. Its [cloud] table is refused; the user file's is used.
	evil, evilRecord := fakeClaude(t, tmp, "claude-project")
	write(t, filepath.Join(tmp, "project", ".claude", "promari-model-router.toml"), "[cloud]\nclaude_bin = \""+evil+"\"\n")
	// poll_interval_seconds = 0 lets the test see `wait` finish.
	write(t, filepath.Join(tmp, "home", ".claude", "promari-model-router.toml"), "[cloud]\nclaude_bin = \""+bin+"\"\npoll_interval_seconds = 0\n")

	var sendOut string
	// The steps share one data directory and run in order.
	steps := []struct {
		name      string
		stdin     string
		cancelled bool
		args      []string
		wantIs    error
		wantErr   bool
		wantOut   string
	}{
		{name: "status before use", args: []string{"cloud", "status"}, wantIs: model.ErrNoCloudSession},
		{name: "send before use", args: []string{"cloud", "send", "hi"}, wantIs: model.ErrNoCloudSession},
		{name: "use with an option word", args: []string{"cloud", "use", "--", "--help"}, wantIs: model.ErrInvalidCloudSession},
		{name: "use a URL", args: []string{"cloud", "use", "https://claude.ai/code/" + id + "?from=cli"}, wantOut: id},
		{name: "empty send", stdin: " \n", args: []string{"cloud", "send"}, wantIs: model.ErrEmptyCloudMessage},
		{name: "send from stdin", stdin: "first line\nsecond line\n", args: []string{"cloud", "send", "--json"}, wantOut: `"sent_at"`},
		{name: "status after send", args: []string{"cloud", "status"}, wantOut: "sent at"},
		{name: "wait one interval", args: []string{"cloud", "wait"}},
		{name: "wait under a cancelled context", cancelled: true, args: []string{"cloud", "wait"}, wantErr: true},
	}
	for _, st := range steps {
		t.Run(st.name, func(t *testing.T) {
			out, err := execute(t, nil, st.cancelled, st.stdin, st.args...)
			switch {
			case st.wantIs != nil && !errors.Is(err, st.wantIs):
				t.Fatalf("err = %v, want %v", err, st.wantIs)
			case st.wantIs == nil && (err != nil) != st.wantErr:
				t.Fatalf("err = %v, want error %v", err, st.wantErr)
			case !strings.Contains(out, st.wantOut):
				t.Fatalf("out = %q, want it to contain %q", out, st.wantOut)
			}
			if st.name == "send from stdin" {
				sendOut = out
			}
		})
	}

	var v struct {
		Session  string `json:"session"`
		URL      string `json:"url"`
		SentAt   string `json:"sent_at"`
		MaxPolls int    `json:"max_polls"`
	}
	if err := json.Unmarshal([]byte(sendOut), &v); err != nil {
		t.Fatalf("send --json: %v\n%s", err, sendOut)
	}
	if v.Session != id || v.URL != "https://claude.ai/code/"+id || v.SentAt == "" || v.MaxPolls == 0 {
		t.Errorf("send --json = %+v", v)
	}
	got, err := os.ReadFile(record)
	if err != nil {
		t.Fatalf("the user's CLI did not run: %v", err)
	}
	if want := "-p --cloud " + id + " --output-format json\nfirst line\nsecond line"; string(got) != want {
		t.Errorf("CLI saw %q, want %q", got, want)
	}
	if _, err := os.Stat(evilRecord); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("the project file's program ran (err = %v)", err)
	}
}

// failingReader stands for a standard input that cannot be read.
type failingReader struct{}

func (failingReader) Read([]byte) (int, error) { return 0, errStdin }

var errStdin = errors.New("stdin closed")

func TestCloudSendUnreadableStdin(t *testing.T) {
	tests := []struct {
		name string
		args []string
	}{
		{name: "send with no argument reads stdin", args: []string{"cloud", "send"}},
		{name: "send --json with no argument reads stdin too", args: []string{"cloud", "send", "--json"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmp := isolate(t, "")
			bin, _ := fakeClaude(t, tmp, "claude-user")
			write(t, filepath.Join(tmp, "home", ".claude", "promari-model-router.toml"), "[cloud]\nclaude_bin = \""+bin+"\"\n")
			if _, err := execute(t, nil, false, "", "cloud", "use", "session_017cB1aBqhcEMKwqXEK2d4A9"); err != nil {
				t.Fatal(err)
			}
			root := newRoot(nil)
			root.SetArgs(tt.args)
			root.SetIn(failingReader{})
			root.SetOut(io.Discard)
			root.SetErr(io.Discard)
			if err := root.ExecuteContext(t.Context()); !errors.Is(err, errStdin) {
				t.Fatalf("err = %v, want %v", err, errStdin)
			}
		})
	}
}

func TestCloudWait(t *testing.T) {
	tests := []struct {
		name     string
		interval string
		wantErr  error
	}{
		{name: "an interval passes", interval: "0"},
		{name: "cancelled while waiting", interval: "3600", wantErr: context.DeadlineExceeded},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmp := isolate(t, "")
			write(t, filepath.Join(tmp, "home", ".claude", "promari-model-router.toml"), "[cloud]\npoll_interval_seconds = "+tt.interval+"\n")
			ctx, cancel := context.WithTimeout(t.Context(), 200*time.Millisecond)
			defer cancel()
			root := newRoot(nil)
			root.SetArgs([]string{"cloud", "wait"})
			root.SetOut(io.Discard)
			root.SetErr(io.Discard)
			if err := root.ExecuteContext(ctx); !errors.Is(err, tt.wantErr) {
				t.Fatalf("err = %v, want %v", err, tt.wantErr)
			}
		})
	}
}
