package model

import (
	"cmp"
	"path/filepath"
	"slices"
	"time"
)

// Peer is what one session's status line last showed about its session,
// posted for the status lines of the other sessions on this machine. Each
// render posts it, so every status line shows all the sessions that run.
type Peer struct {
	// Key is the session id; it names the peer's file.
	Key string `json:"key"`
	// Owner is the process id of the Claude Code that runs the session; zero
	// when unknown. A peer whose owner has exited is no longer running.
	Owner int       `json:"owner,omitzero"`
	At    time.Time `json:"at"`
	// Account is the account the session runs as; "" when unknown. Sessions
	// of different accounts do not show each other.
	Account string `json:"account,omitzero"`

	Name       string            `json:"name,omitzero"`
	Project    string            `json:"project,omitzero"`
	Branch     string            `json:"branch,omitzero"`
	Model      string            `json:"model,omitzero"`
	ContextPct Optional[float64] `json:"context_pct,omitzero"`
	CostUSD    Optional[float64] `json:"cost_usd,omitzero"`
}

// PeerOf summarises a session of an account for the other status lines. ok is
// false for a session without a usable id: it has no file of its own to post to.
func PeerOf(s *Session, account, branch string, at time.Time) (Peer, bool) {
	key, ok := s.Key()
	if !ok {
		return Peer{}, false
	}
	p := Peer{Key: key, At: at, Account: account, Name: s.Name, Branch: branch, Model: s.Model, CostUSD: s.Cost.TotalUSD}
	if s.Dir != "" {
		p.Project = filepath.Base(s.Dir)
	}
	if usage, ok := s.Context.Usage(); ok {
		p.ContextPct = Some(usage.Pct)
	}
	return p, true
}

// Label is how the peer is named on a status line: the session's name when it
// has one, otherwise its project.
func (p Peer) Label() string {
	if p.Name != "" {
		return p.Name
	}
	if p.Project != "" {
		return p.Project
	}
	return p.Key
}

// Roster is the running sessions, the caller's own included.
type Roster []Peer

// Of returns the sessions of an account. An unknown account ("") is one of its
// own: it is not taken for any known account.
func (r Roster) Of(account string) Roster {
	return slices.DeleteFunc(slices.Clone(r), func(p Peer) bool { return p.Account != account })
}

// Others returns the sessions other than self, in a stable order: by label,
// then by id, so that a session keeps its place from one render to the next.
func (r Roster) Others(self string) Roster {
	others := slices.DeleteFunc(slices.Clone(r), func(p Peer) bool { return p.Key == self })
	slices.SortFunc(others, func(a, b Peer) int {
		return cmp.Or(cmp.Compare(a.Label(), b.Label()), cmp.Compare(a.Key, b.Key))
	})
	return others
}
