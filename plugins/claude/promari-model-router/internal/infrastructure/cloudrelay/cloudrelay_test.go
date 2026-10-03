package cloudrelay_test

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"

	"promari-model-router/internal/domain/model"
	"promari-model-router/internal/infrastructure/cloudrelay"
)

const cloudID = "session_017cB1aBqhcEMKwqXEK2d4A9"

func ids(t *testing.T) (model.CloudSessionID, model.CloudMessage) {
	t.Helper()
	id, err := model.ParseCloudSessionID(cloudID)
	if err != nil {
		t.Fatal(err)
	}
	msg, err := model.NewCloudMessage("line one\n$(touch /h/pwned) `id`")
	if err != nil {
		t.Fatal(err)
	}
	return id, msg
}

func TestMessengerSend(t *testing.T) {
	t.Parallel()
	errExit := errors.New("exit status 1")
	tests := []struct {
		name    string
		stdout  string
		stderr  string
		runErr  error
		wantURL string
		wantErr string
	}{
		{name: "queued", stdout: `{"ok":true,"session_id":"` + cloudID + `","url":"https://claude.ai/code/` + cloudID + `?from=cli"}`, wantURL: "https://claude.ai/code/" + cloudID + "?from=cli"},
		{name: "queued without a URL", stdout: `{"ok":true}`, wantURL: "https://claude.ai/code/" + cloudID},
		{name: "archived session", stdout: `{"ok":false,"session_id":"` + cloudID + `","error":"cloud session is archived"}`, runErr: errExit, wantErr: "cloud session is archived"},
		{name: "configuration error on stderr", stderr: "Error: Cloud sessions are disabled by your organization's policy.\nmore", runErr: errExit, wantErr: "disabled by your organization's policy."},
		{name: "no output at all", runErr: errExit, wantErr: "exit status 1"},
		{name: "unreadable output and nothing else", stdout: "not json", wantErr: "no output"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			id, msg := ids(t)
			var gotArgs []string
			var gotStdin string
			m := cloudrelay.Messenger{
				Bin: "claude", Timeout: time.Second,
				Run: func(_ context.Context, bin string, args []string, stdin io.Reader) ([]byte, []byte, error) {
					raw, _ := io.ReadAll(stdin)
					gotArgs, gotStdin = append([]string{bin}, args...), string(raw)
					return []byte(tt.stdout), []byte(tt.stderr), tt.runErr
				},
			}
			url, err := m.Send(context.Background(), id, msg)
			// The message never reaches the command line, only standard input.
			if diff := cmp.Diff([]string{"claude", "-p", "--cloud", cloudID, "--output-format", "json"}, gotArgs); diff != "" {
				t.Errorf("args (-want +got):\n%s", diff)
			}
			if gotStdin != msg.Text() {
				t.Errorf("stdin = %q", gotStdin)
			}
			if tt.wantErr != "" {
				if !errors.Is(err, cloudrelay.ErrSendRefused) || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("err = %v, want ErrSendRefused with %q", err, tt.wantErr)
				}
				return
			}
			if err != nil || url != tt.wantURL {
				t.Fatalf("url, err = %q, %v; want %q", url, err, tt.wantURL)
			}
		})
	}
}

// A send stopped before the CLI answered may have queued the message: it is
// not reported as refused (it used to be, inviting a second, billed send).
func TestMessengerSendStopped(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		stop    func(cancel context.CancelFunc) // stops the caller's context, or not
		stdout  string
		wantErr error
		wantURL string
	}{
		{name: "the timeout", stop: func(context.CancelFunc) {}, wantErr: cloudrelay.ErrSendUnknown},
		{name: "Ctrl-C", stop: func(cancel context.CancelFunc) { cancel() }, wantErr: cloudrelay.ErrSendUnknown},
		{
			name: "an answer that came before the stop still counts", stop: func(cancel context.CancelFunc) { cancel() },
			stdout: `{"ok":true,"url":"https://claude.ai/code/x"}`, wantURL: "https://claude.ai/code/x",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			id, msg := ids(t)
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			tt.stop(cancel)
			m := cloudrelay.Messenger{
				Bin: "claude", Timeout: 10 * time.Millisecond,
				Run: func(ctx context.Context, _ string, _ []string, _ io.Reader) ([]byte, []byte, error) {
					<-ctx.Done() // the CLI is killed without answering
					return []byte(tt.stdout), nil, ctx.Err()
				},
			}
			url, err := m.Send(ctx, id, msg)
			if !errors.Is(err, tt.wantErr) || url != tt.wantURL {
				t.Fatalf("url, err = %q, %v; want %q, %v", url, err, tt.wantURL, tt.wantErr)
			}
			if errors.Is(err, cloudrelay.ErrSendRefused) {
				t.Errorf("a stopped send was reported as refused: %v", err)
			}
		})
	}
}

func TestMessengerRunsTheProgram(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	bin := filepath.Join(dir, "fake-claude")
	// The fake CLI echoes its stdin back inside the URL, so the test sees that
	// the real process got the message on standard input.
	script := "#!/bin/sh\nprintf '{\"ok\":true,\"url\":\"https://claude.ai/code/%s\"}' \"$(cat)\"\n"
	if err := os.WriteFile(bin, []byte(script), 0o700); err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name    string
		bin     string
		wantURL string
		wantErr error
	}{
		{name: "the program gets the message on stdin", bin: bin, wantURL: "https://claude.ai/code/hello"},
		{name: "a missing program is a refused send", bin: filepath.Join(dir, "missing"), wantErr: cloudrelay.ErrSendRefused},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			id, err := model.ParseCloudSessionID(cloudID)
			if err != nil {
				t.Fatal(err)
			}
			msg, err := model.NewCloudMessage("hello")
			if err != nil {
				t.Fatal(err)
			}
			url, err := cloudrelay.Messenger{Bin: tt.bin, Timeout: 10 * time.Second}.Send(context.Background(), id, msg)
			if !errors.Is(err, tt.wantErr) || url != tt.wantURL {
				t.Fatalf("url, err = %q, %v; want %q, %v", url, err, tt.wantURL, tt.wantErr)
			}
		})
	}
}

func TestStore(t *testing.T) {
	t.Parallel()
	jst := time.FixedZone("JST", 9*3600)
	sentAt := time.Date(2026, 10, 3, 8, 54, 52, 0, jst)
	tests := []struct {
		name    string
		file    string // written before Load; "" = no file, "save" = written by Save
		wantOK  bool
		wantErr bool
		wantIs  error
	}{
		{name: "no file yet", file: ""},
		{name: "what Save wrote", file: "save", wantOK: true},
		// A hand-edited file cannot smuggle a non-ID into the command line.
		{name: "an option word for a session", file: `{"session":"--help"}`, wantErr: true, wantIs: model.ErrInvalidCloudSession},
		{name: "broken JSON", file: `{not json`, wantErr: true},
		{name: "the path is a directory", file: "dir", wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			s := cloudrelay.Store{Path: filepath.Join(t.TempDir(), "sub", "cloud.json")}
			ctx := context.Background()
			id, _ := ids(t)
			switch tt.file {
			case "":
			case "dir":
				if err := os.MkdirAll(s.Path, 0o700); err != nil {
					t.Fatal(err)
				}
			case "save":
				if err := s.Save(ctx, model.CloudLink{Session: id, SentAt: sentAt}); err != nil {
					t.Fatal(err)
				}
				if info, err := os.Stat(s.Path); err != nil || info.Mode().Perm() != 0o600 {
					t.Fatalf("mode = %v, err = %v; want 0600", info.Mode().Perm(), err)
				}
			default:
				if err := os.MkdirAll(filepath.Dir(s.Path), 0o700); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(s.Path, []byte(tt.file), 0o600); err != nil {
					t.Fatal(err)
				}
			}
			got, ok, err := s.Load(ctx)
			if (err != nil) != tt.wantErr || (tt.wantIs != nil && !errors.Is(err, tt.wantIs)) {
				t.Fatalf("err = %v, want error %v (%v)", err, tt.wantErr, tt.wantIs)
			}
			if ok != tt.wantOK {
				t.Fatalf("ok = %v, want %v", ok, tt.wantOK)
			}
			if tt.wantOK && (got.Session != id || !got.SentAt.Equal(sentAt)) {
				t.Errorf("got %+v", got)
			}
		})
	}
}

func TestStoreSaveFailures(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name   string
		sentAt time.Time
		block  bool // a file stands where the directory should be
	}{
		{name: "a time JSON cannot hold", sentAt: time.Date(10000, 1, 1, 0, 0, 0, 0, time.UTC)},
		{name: "the directory cannot be made", block: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			dir := t.TempDir()
			if tt.block {
				if err := os.WriteFile(filepath.Join(dir, "sub"), nil, 0o600); err != nil {
					t.Fatal(err)
				}
			}
			id, _ := ids(t)
			s := cloudrelay.Store{Path: filepath.Join(dir, "sub", "cloud.json")}
			if err := s.Save(context.Background(), model.CloudLink{Session: id, SentAt: tt.sentAt}); err == nil {
				t.Fatal("no error")
			}
		})
	}
}
