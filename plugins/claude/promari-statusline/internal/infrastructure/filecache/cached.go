package filecache

import (
	"context"
	"time"

	"promari-statusline/internal/domain/model"
	"promari-statusline/internal/domain/repository"
)

// How long each source's answer is remembered: as long as the answer stays
// true enough, and no longer than a reader would notice.
const (
	pullTTL     = 5 * time.Minute
	trackTTL    = 15 * time.Second
	toolsTTL    = 30 * time.Second
	incidentTTL = 5 * time.Minute
	releaseTTL  = 6 * time.Hour
	codexTTL    = time.Minute
	accountTTL  = time.Hour
)

// The decorators below add remembering to a reader without the reader knowing:
// each implements the port it wraps and asks the wrapped reader only when the
// remembered answer is too old.

// PullRequests remembers the pull request of a branch.
type PullRequests struct {
	Store *Store
	Next  repository.PullRequestReader
}

// PullRequest implements repository.PullRequestReader.
func (c PullRequests) PullRequest(ctx context.Context, dir, branch string) (model.PullRequest, error) {
	return Memo(c.Store, "pull-request.json", dir+":"+branch, pullTTL, func() (model.PullRequest, error) {
		return c.Next.PullRequest(ctx, dir, branch)
	})
}

// Tracks remembers the song that is playing.
type Tracks struct {
	Store *Store
	Next  repository.TrackReader
}

// Track implements repository.TrackReader.
func (c Tracks) Track(ctx context.Context) (model.Track, error) {
	return Memo(c.Store, "track.json", "", trackTTL, func() (model.Track, error) { return c.Next.Track(ctx) })
}

// ToolStats remembers the tool calls counted in a transcript.
type ToolStats struct {
	Store *Store
	Next  repository.ToolStatsReader
}

// ToolStats implements repository.ToolStatsReader.
func (c ToolStats) ToolStats(ctx context.Context, transcript string) (model.ToolStats, error) {
	return Memo(c.Store, "tools.json", transcript, toolsTTL, func() (model.ToolStats, error) {
		return c.Next.ToolStats(ctx, transcript)
	})
}

// Incidents remembers the state of the API's status page.
type Incidents struct {
	Store *Store
	Next  repository.IncidentReader
}

// Incident implements repository.IncidentReader.
func (c Incidents) Incident(ctx context.Context) (model.Incident, error) {
	return Memo(c.Store, "incident.json", "", incidentTTL, func() (model.Incident, error) { return c.Next.Incident(ctx) })
}

// Releases remembers the newest released version.
type Releases struct {
	Store *Store
	Next  repository.ReleaseReader
}

// Latest implements repository.ReleaseReader.
func (c Releases) Latest(ctx context.Context) (string, error) {
	return Memo(c.Store, "latest.json", "", releaseTTL, func() (string, error) { return c.Next.Latest(ctx) })
}

// Codex remembers the usage windows Codex last reported.
type Codex struct {
	Store *Store
	Next  repository.CodexReader
}

// Codex implements repository.CodexReader.
func (c Codex) Codex(ctx context.Context) (model.CodexLimits, error) {
	return Memo(c.Store, "codex.json", "", codexTTL, func() (model.CodexLimits, error) { return c.Next.Codex(ctx) })
}

// AccountFile is an account reader that names the file it reads.
type AccountFile interface {
	repository.AccountReader
	File() string
}

// Accounts remembers the signed-in account. The answer is kept for the file it
// was read from as it was then: a /login rewrites the file and is seen on the
// next render, and a session of another configuration directory is never
// answered with this one's account.
type Accounts struct {
	Store *Store
	Next  AccountFile
}

// Account implements repository.AccountReader.
func (c Accounts) Account(ctx context.Context) (string, error) {
	file := c.Next.File()
	key := file
	if at, err := c.Store.sys.ModTime(file); err == nil {
		key += "@" + at.UTC().Format(time.RFC3339Nano)
	}
	return Memo(c.Store, "account.json", key, accountTTL, func() (string, error) { return c.Next.Account(ctx) })
}
