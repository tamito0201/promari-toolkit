package filecache

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"path/filepath"
	"time"

	"promari-statusline/internal/domain/model"
	"promari-statusline/internal/domain/repository"
)

// How long each source's answer is remembered: as long as the answer stays
// true enough, and no longer than a reader would notice.
const (
	pullTTL  = 5 * time.Minute
	trackTTL = 15 * time.Second
	// transcriptTTL is short: a read only adds what was written since the last.
	transcriptTTL = 10 * time.Second
	incidentTTL   = 5 * time.Minute
	releaseTTL    = 6 * time.Hour
	codexTTL      = time.Minute
	accountTTL    = time.Hour

	// transcriptDir holds what each transcript recorded.
	transcriptDir = "transcripts"
	// transcriptsKept is how long the record of a transcript that is no longer
	// read is kept.
	transcriptsKept = 7 * 24 * time.Hour
	// digestBytes is the part of a SHA-256 that names a file: 64 bits.
	digestBytes = 8
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

// Transcripts remembers what each transcript recorded and where its reading
// stopped, in a file of its own per transcript: sessions running side by side
// would otherwise replace each other's progress and read their transcripts
// from the start again and again.
type Transcripts struct {
	Store *Store
	Next  repository.TranscriptReader
}

// Transcript implements repository.TranscriptReader. A remembered answer
// younger than transcriptTTL is returned as it is; an older one is where the
// next read continues. A failed read keeps the progress already remembered.
func (c Transcripts) Transcript(ctx context.Context, path string, since model.Transcript) (model.Transcript, error) {
	now := c.Store.sys.Now()
	name := filepath.Join(transcriptDir, digest(path)+".json")
	last, known := Load[entry[model.Transcript]](c.Store, name)
	known = known && last.Key == path
	if known && fresh(last.At, now, transcriptTTL) {
		if !last.Found {
			return last.Value, repository.ErrNone
		}
		return last.Value, nil
	}
	if known {
		since = last.Value
	} else {
		c.Store.prune(transcriptDir, transcriptsKept)
	}
	read, err := c.Next.Transcript(ctx, path, since)
	if err != nil && !errors.Is(err, repository.ErrNone) {
		return read, err
	}
	// A cache that cannot be written costs reading the transcript from the start next time.
	_ = Save(c.Store, entry[model.Transcript]{At: now, Key: path, Found: err == nil, Value: read}, name)
	return read, err
}

// digest names a file after a path without spelling the path out.
func digest(path string) string {
	sum := sha256.Sum256([]byte(path))
	return hex.EncodeToString(sum[:digestBytes])
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
