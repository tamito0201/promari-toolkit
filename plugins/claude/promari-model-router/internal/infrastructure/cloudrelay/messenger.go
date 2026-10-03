// Package cloudrelay reaches a Claude Code cloud session through the Claude
// Code CLI: `claude -p --cloud <id>` queues one message and exits. The CLI
// authenticates by itself; this package never reads, stores or forwards a
// Claude.ai credential, and the message travels on standard input, never on
// the command line (no length limit, nothing for `ps` to show).
package cloudrelay

import (
	"bytes"
	"context"
	"encoding/json/v2"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"strings"
	"time"

	"promari-model-router/internal/domain/model"
	"promari-model-router/internal/domain/repository"
)

// Runner starts a program with stdin and returns what it printed. It is a
// field so that a test can stand in for the CLI.
type Runner func(ctx context.Context, bin string, args []string, stdin io.Reader) (stdout, stderr []byte, err error)

// Messenger implements repository.CloudMessenger with the Claude Code CLI.
type Messenger struct {
	Bin     string
	Timeout time.Duration
	Run     Runner
}

var _ repository.CloudMessenger = Messenger{}

// sendResult is the CLI's `--output-format json` answer to a send.
type sendResult struct {
	OK        bool   `json:"ok"`
	SessionID string `json:"session_id"`
	URL       string `json:"url"`
	Error     string `json:"error"`
}

// ErrSendRefused is a send the CLI answered with ok=false, or that ended with
// no answer of its own (it could not start, or it printed an error).
var ErrSendRefused = errors.New("the Claude Code CLI did not queue the message")

// ErrSendUnknown is a send stopped before the CLI answered (the timeout, or
// Ctrl-C). The CLI may have queued the message already: sending it again can
// run the work, which is billed in the cloud, twice. The session's page says
// whether it arrived.
var ErrSendUnknown = errors.New("the send was stopped before the Claude Code CLI answered; the message may have been queued, so look at the session before sending it again")

// waitDelay bounds the wait for the CLI's output once it is stopped: a child it
// started (node) may hold its standard output open after it is killed.
const waitDelay = 2 * time.Second

// Send implements repository.CloudMessenger.
func (m Messenger) Send(ctx context.Context, session model.CloudSessionID, msg model.CloudMessage) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, m.Timeout)
	defer cancel()
	run := m.Run
	if run == nil {
		run = runCommand
	}
	args := []string{"-p", "--cloud", session.String(), "--output-format", "json"}
	stdout, stderr, err := run(ctx, m.Bin, args, strings.NewReader(msg.Text()))
	var res sendResult
	jerr := json.Unmarshal(bytes.TrimSpace(stdout), &res)
	if jerr != nil && ctx.Err() != nil {
		return "", fmt.Errorf("%w (%w)", ErrSendUnknown, context.Cause(ctx))
	}
	if jerr != nil {
		// Configuration errors (no Anthropic account, a disabled policy) come
		// on stderr without JSON.
		return "", fmt.Errorf("%w: %s", ErrSendRefused, firstLine(stderr, err))
	}
	if !res.OK {
		return "", fmt.Errorf("%w: %s", ErrSendRefused, orElse(res.Error, firstLine(stderr, err)))
	}
	return orElse(res.URL, session.URL()), nil
}

func runCommand(ctx context.Context, bin string, args []string, stdin io.Reader) ([]byte, []byte, error) {
	var stdout, stderr bytes.Buffer
	cmd := exec.CommandContext(ctx, bin, args...) // #nosec G204 -- bin comes from the user file only; args are fixed words and a validated ID
	cmd.Stdin, cmd.Stdout, cmd.Stderr = stdin, &stdout, &stderr
	cmd.WaitDelay = waitDelay
	err := cmd.Run()
	return stdout.Bytes(), stderr.Bytes(), err
}

func firstLine(stderr []byte, err error) string {
	if line, _, _ := strings.Cut(strings.TrimSpace(string(stderr)), "\n"); line != "" {
		return line
	}
	if err != nil {
		return err.Error()
	}
	return "no output"
}

func orElse(s, fallback string) string {
	if s == "" {
		return fallback
	}
	return s
}
