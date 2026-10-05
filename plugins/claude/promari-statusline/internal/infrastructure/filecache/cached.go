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
	return Memo(ctx, c.Store, "pull-request", dir+":"+branch, pullTTL, func() (model.PullRequest, error) {
		return c.Next.PullRequest(ctx, dir, branch)
	})
}

// historyTTL is how long the history of a branch is remembered: it changes
// only when commits are made or fetched, and its slowest parts (today's lines,
// the merged branches, the days with commits) would cost every render.
const historyTTL = time.Minute

// Histories remembers what the commits of a branch say.
type Histories struct {
	Store *Store
	Next  repository.HistoryReader
}

// History implements repository.HistoryReader.
func (c Histories) History(ctx context.Context, dir, branch string) (model.History, error) {
	return Memo(ctx, c.Store, "history", dir+":"+branch, historyTTL, func() (model.History, error) {
		return c.Next.History(ctx, dir, branch)
	})
}

// ReviewQueues remembers the pull requests that wait for the user's review.
type ReviewQueues struct {
	Store *Store
	Next  repository.ReviewQueueReader
}

// ReviewQueue implements repository.ReviewQueueReader.
func (c ReviewQueues) ReviewQueue(ctx context.Context, dir string) (model.ReviewQueue, error) {
	return Memo(ctx, c.Store, "review-queue", dir, pullTTL, func() (model.ReviewQueue, error) {
		return c.Next.ReviewQueue(ctx, dir)
	})
}

// Workloads remembers the work the user owes. Issues and pull requests
// change over minutes, and five questions to GitHub are too many for every
// render.
type Workloads struct {
	Store *Store
	Next  repository.WorkloadReader
}

// Workload implements repository.WorkloadReader.
func (c Workloads) Workload(ctx context.Context, dir string) (model.Workload, error) {
	return Memo(ctx, c.Store, "workload", dir, pullTTL, func() (model.Workload, error) {
		return c.Next.Workload(ctx, dir)
	})
}

// Tracks remembers the song that is playing.
type Tracks struct {
	Store *Store
	Next  repository.TrackReader
}

// Track implements repository.TrackReader.
func (c Tracks) Track(ctx context.Context) (model.Track, error) {
	return Memo(ctx, c.Store, "track", "", trackTTL, func() (model.Track, error) { return c.Next.Track(ctx) })
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
		c.Store.Prune(transcriptDir, transcriptsKept)
	}
	read, err := c.Next.Transcript(ctx, path, since)
	// A read cut short by the end of the render is not where the next one continues.
	if err != nil && !errors.Is(err, repository.ErrNone) || ctx.Err() != nil {
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
	return Memo(ctx, c.Store, "incident", "", incidentTTL, func() (model.Incident, error) { return c.Next.Incident(ctx) })
}

// Releases remembers the newest released version.
type Releases struct {
	Store *Store
	Next  repository.ReleaseReader
}

// Latest implements repository.ReleaseReader.
func (c Releases) Latest(ctx context.Context) (string, error) {
	return Memo(ctx, c.Store, "latest", "", releaseTTL, func() (string, error) { return c.Next.Latest(ctx) })
}

// Codex remembers the usage windows Codex last reported.
type Codex struct {
	Store *Store
	Next  repository.CodexReader
}

// Codex implements repository.CodexReader.
func (c Codex) Codex(ctx context.Context) (model.CodexLimits, error) {
	return Memo(ctx, c.Store, "codex", "", codexTTL, func() (model.CodexLimits, error) { return c.Next.Codex(ctx) })
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
	return Memo(ctx, c.Store, "account", key, accountTTL, func() (string, error) { return c.Next.Account(ctx) })
}
