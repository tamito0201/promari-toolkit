package model

import (
	"errors"
	"fmt"
	"net/url"
	"regexp"
	"strings"
	"time"
)

// CloudSessionID names a Claude Code cloud session (session_… or cse_…), the
// session `pmr cloud` relays messages to. It is a value object: the only way
// to get one is ParseCloudSessionID, so a held value is always well formed.
type CloudSessionID struct{ id string }

// cloudSessionPattern is the shape of a cloud session ID as claude.ai prints
// it. Anything else (a path, an option, a shell word) is refused before it can
// reach a command line.
var cloudSessionPattern = regexp.MustCompile(`^(?:session|cse)_[A-Za-z0-9]{8,64}$`)

// cloudSessionHost is the host of a cloud session's page.
const cloudSessionHost = "claude.ai"

// cloudSessionPathPrefix is the path of a cloud session's page before its ID.
const cloudSessionPathPrefix = "/code/"

// Errors of the cloud relay.
var (
	ErrInvalidCloudSession = errors.New("not a cloud session ID or claude.ai/code URL")
	ErrNoCloudSession      = errors.New("no cloud session is set: run `pmr cloud use <session-id|url>`")
	ErrEmptyCloudMessage   = errors.New("the message is empty")
)

// ParseCloudSessionID accepts what the user copies: a bare ID, or the
// session's claude.ai/code URL with or without the scheme and query string.
func ParseCloudSessionID(raw string) (CloudSessionID, error) {
	s := strings.TrimSpace(raw)
	if cloudSessionPattern.MatchString(s) {
		return CloudSessionID{id: s}, nil
	}
	if !strings.Contains(s, "://") {
		s = "https://" + s
	}
	u, err := url.Parse(s)
	if err != nil || u.Hostname() != cloudSessionHost {
		return CloudSessionID{}, fmt.Errorf("%w: %q", ErrInvalidCloudSession, raw)
	}
	id, ok := strings.CutPrefix(u.Path, cloudSessionPathPrefix)
	if !ok || !cloudSessionPattern.MatchString(id) {
		return CloudSessionID{}, fmt.Errorf("%w: %q", ErrInvalidCloudSession, raw)
	}
	return CloudSessionID{id: id}, nil
}

// String returns the bare ID.
func (c CloudSessionID) String() string { return c.id }

// IsZero reports whether no session is named.
func (c CloudSessionID) IsZero() bool { return c.id == "" }

// URL is the session's page on claude.ai.
func (c CloudSessionID) URL() string {
	return "https://" + cloudSessionHost + cloudSessionPathPrefix + c.id
}

// CloudLink is the relay's state: the session messages go to, and when the
// last one was sent. A reply is what the session wrote after SentAt.
type CloudLink struct {
	Session CloudSessionID
	SentAt  time.Time
}

// CloudMessage is one message for the session. It is never empty, and it
// travels on standard input, never on a command line.
type CloudMessage struct{ text string }

// NewCloudMessage trims the text and refuses an empty one.
func NewCloudMessage(text string) (CloudMessage, error) {
	t := strings.TrimSpace(text)
	if t == "" {
		return CloudMessage{}, ErrEmptyCloudMessage
	}
	return CloudMessage{text: t}, nil
}

// Text returns the message.
func (m CloudMessage) Text() string { return m.text }

// CloudReceipt is what a successful send leaves: where the message went and
// from when to read the reply.
type CloudReceipt struct {
	Session CloudSessionID
	URL     string
	SentAt  time.Time
}
