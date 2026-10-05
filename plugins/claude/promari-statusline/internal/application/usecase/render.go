// Package usecase holds what the plugin does: render the status line, install
// it into Claude Code's settings, keep the installed copy current and
// diagnose the installation. A use case knows the domain and the ports in
// internal/domain/repository; it never names an adapter.
package usecase

import (
	"context"
	"errors"
	"slices"
	"strings"
	"sync"
	"time"

	"promari-statusline/internal/domain/model"
	"promari-statusline/internal/domain/repository"
	"promari-statusline/internal/domain/service"
)

// limitsKept is how long remembered rate limits are still worth showing.
const limitsKept = 7 * 24 * time.Hour

// Sources are the readers a render asks for facts.
type Sources struct {
	Git        repository.GitReader
	Pulls      repository.PullRequestReader
	Reviews    repository.ReviewQueueReader
	Workload   repository.WorkloadReader
	Spend      repository.SpendReader
	Codex      repository.CodexReader
	Transcript repository.TranscriptReader
	Todos      repository.TodoReader
	Track      repository.TrackReader
	Incident   repository.IncidentReader
	Release    repository.ReleaseReader
	Account    repository.AccountReader
	Machine    repository.MachineReader
}

// RenderDeps is what RenderStatusLine depends on.
type RenderDeps struct {
	Clock      repository.Clock
	Sources    Sources
	Activities repository.ActivityStore
	Limits     repository.RateLimitMemory
	Board      repository.UsageBoard
	Terminal   repository.Terminal
	Recorder   repository.Recorder
	Switches   repository.Switches
	Peers      repository.PeerBoard
}

// RenderStatusLine turns one session report into the lines of the status line.
type RenderStatusLine struct {
	deps RenderDeps
}

// NewRenderStatusLine returns the use case. It panics when a dependency is
// missing: a wiring mistake is found when the program starts, not as a chip
// that never shows.
func NewRenderStatusLine(deps RenderDeps) *RenderStatusLine {
	mustBeWired("RenderDeps", deps)
	return &RenderStatusLine{deps: deps}
}

// RenderRequest is one render: the session as Claude Code reported it, and the
// report as it arrived.
type RenderRequest struct {
	Session model.Session
	Raw     []byte
}

// Rendered is the status line of one render.
type Rendered struct {
	Lines []model.Line
	// At is the moment the render describes; the blink of an alarm follows it.
	At time.Time
}

// Execute renders the status line. It never fails: a fact that cannot be read
// is left out, and what is known is shown.
func (u *RenderStatusLine) Execute(ctx context.Context, req RenderRequest) Rendered {
	d := u.deps
	now := d.Clock.Now()
	d.Recorder.Input(req.Raw)

	view := service.View{Now: now, Session: req.Session, AlarmAll: d.Switches.BlinkDemo()}
	if usage, ok := req.Session.Context.Usage(); ok {
		view.Usage = model.Some(usage)
		view.Activity = u.observe(&req.Session, usage.Used, now)
	}
	facts, runs := u.gather(ctx, req)
	view.Facts = facts
	d.Recorder.Sources(runs)
	// The account the limits, the forecasts and the other sessions belong to.
	account := view.Facts.Account.Or("")
	view.Limits, view.LimitsSeen, view.Forecasts = u.limits(account, req.Session.Limits, now)
	view.Peers, view.Running = u.share(&req.Session, account, view.Facts, now)
	if codex, ok := view.Facts.Codex.Get(); ok {
		// A board that cannot be written costs another tool its advice, not this render.
		_ = d.Board.PostCodex(codex, now)
	}

	cells, source := d.Terminal.Width()
	budget := service.Budget(cells)
	d.Recorder.Width(source, budget)
	return Rendered{Lines: service.Layout(service.Compose(&view), budget), At: now}
}

// limits returns the rate limits to show. Limits that came with this render
// are remembered, posted on the board for other tools and added to the
// history the forecasts are made from. A render without limits (the first of a
// session) shows the remembered ones with the time they were seen, and neither
// posts them nor makes a forecast from them: a stale value is not a new
// measurement.
func (u *RenderStatusLine) limits(account string, reported model.RateLimits, now time.Time) (model.RateLimits, time.Time, []model.Forecast) {
	memory := u.deps.Limits
	if reported.Empty() {
		last, seen, err := memory.Last(account)
		if err != nil || now.Sub(seen) >= limitsKept {
			return model.RateLimits{}, time.Time{}, nil
		}
		return last, seen, nil
	}
	// Memory that cannot be written costs the next session its first chip and
	// the forecast its history; this render shows what it has.
	_ = memory.Remember(account, reported, now)
	_ = u.deps.Board.PostClaude(reported, now)
	history := memory.History(account).Record(reported, now)
	_ = memory.SaveHistory(account, history)
	return reported, time.Time{}, history.Forecasts(reported, now)
}

// observe records this render in the session's activity. A session without a
// usable id has no activity: its counters are unknown, not zero.
func (u *RenderStatusLine) observe(s *model.Session, used float64, now time.Time) model.Optional[model.Activity] {
	key, ok := s.Key()
	if !ok {
		return model.Optional[model.Activity]{}
	}
	activity := u.deps.Activities.Load(key)
	activity.Observe(used, s.PromptID, now)
	// An activity that cannot be saved is still right for this render.
	_ = u.deps.Activities.Save(key, activity)
	return model.Some(activity)
}

// share posts this session for the status lines of the other sessions and
// returns the sessions of the same account with the number running, this one
// included. Sessions of another account are neither shown nor counted. A
// session without a usable id is not posted and sees the others all the same.
func (u *RenderStatusLine) share(s *model.Session, account string, facts model.Facts, now time.Time) (model.Roster, int) {
	key, ok := s.Key()
	if peer, ok := model.PeerOf(s, account, branchOf(facts), now); ok {
		// A post that cannot be written leaves this session out of the others'
		// lines; this render is still right.
		_ = u.deps.Peers.Post(peer)
	}
	roster := u.deps.Peers.Roster().Of(account)
	if !ok {
		key = ""
	}
	return roster.Others(key), len(roster)
}

// branchOf returns the branch of the working tree, or "" outside a repository.
func branchOf(facts model.Facts) string {
	if git, ok := facts.Git.Get(); ok {
		return git.Branch
	}
	return ""
}

// gather asks every source at once (fan-out) and waits for all (fan-in). Each
// question runs in its own goroutine, which ends when its reader returns;
// every reader is bounded by the timeout of the command or request behind it.
// The number of goroutines is the number of sources, fixed in this function.
// Each writes one field of its own. It returns the facts and how each question
// went.
func (u *RenderStatusLine) gather(ctx context.Context, req RenderRequest) (model.Facts, []model.SourceRun) {
	src := u.deps.Sources
	s := &req.Session
	r := &runs{clock: u.deps.Clock}
	var facts model.Facts
	var wg sync.WaitGroup
	wg.Go(func() { facts.Git, facts.Pull = u.gitAndPull(ctx, r, s.WorkDir()) })
	wg.Go(func() {
		facts.Reviews = read(r, "reviews", func() (model.ReviewQueue, error) { return src.Reviews.ReviewQueue(ctx, s.WorkDir()) })
	})
	wg.Go(func() {
		facts.Workload = read(r, "workload", func() (model.Workload, error) { return src.Workload.Workload(ctx, s.WorkDir()) })
	})
	wg.Go(func() {
		facts.Spend = read(r, "spend", func() (model.Spend, error) { return src.Spend.Spend(ctx, req.Raw) })
	})
	wg.Go(func() {
		facts.Codex = read(r, "codex", func() (model.CodexLimits, error) { return src.Codex.Codex(ctx) })
	})
	wg.Go(func() { facts.Track = read(r, "track", func() (model.Track, error) { return src.Track.Track(ctx) }) })
	wg.Go(func() {
		facts.Incident = read(r, "incident", func() (model.Incident, error) { return src.Incident.Incident(ctx) })
	})
	wg.Go(func() { facts.Latest = read(r, "release", func() (string, error) { return src.Release.Latest(ctx) }) })
	wg.Go(func() { facts.Account = read(r, "account", func() (string, error) { return src.Account.Account(ctx) }) })
	wg.Go(func() {
		// The machine is always answered, with what could be measured.
		machine := read(r, "machine", func() (model.Machine, error) { return src.Machine.Machine(ctx, s.WorkDir()), nil })
		facts.Machine = machine.Or(model.Machine{})
	})
	if s.TranscriptPath != "" {
		wg.Go(func() {
			facts.Transcript = read(r, "transcript", func() (model.Transcript, error) {
				return src.Transcript.Transcript(ctx, s.TranscriptPath, model.Transcript{})
			})
		})
	}
	if key, ok := s.Key(); ok {
		wg.Go(func() {
			facts.Todos = read(r, "todos", func() (model.Todos, error) { return src.Todos.Todos(ctx, key) })
		})
	}
	wg.Wait()
	return facts, r.sorted()
}

// gitAndPull reads the working tree and then the pull request of its branch.
func (u *RenderStatusLine) gitAndPull(ctx context.Context, r *runs, dir string) (model.Optional[model.Git], model.Optional[model.PullRequest]) {
	git := read(r, "git", func() (model.Git, error) { return u.deps.Sources.Git.Git(ctx, dir) })
	g, ok := git.Get()
	if !ok || g.Branch == "" {
		return model.Optional[model.Git]{}, model.Optional[model.PullRequest]{}
	}
	pull := read(r, "pull", func() (model.PullRequest, error) { return u.deps.Sources.Pulls.PullRequest(ctx, dir, g.Branch) })
	return git, pull
}

// runs collects how the questions of one render went. The goroutines of a
// render add to it at once.
type runs struct {
	clock repository.Clock
	mu    sync.Mutex
	list  []model.SourceRun
}

func (r *runs) add(run model.SourceRun) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.list = append(r.list, run)
}

// sorted returns the runs in the order of their sources' names, so that two
// renders are compared line by line.
func (r *runs) sorted() []model.SourceRun {
	r.mu.Lock()
	defer r.mu.Unlock()
	return slices.SortedFunc(slices.Values(r.list), func(a, b model.SourceRun) int { return strings.Compare(a.Source, b.Source) })
}

// errPanicked is the answer of a reader that panicked.
var errPanicked = errors.New("the reader panicked")

// read asks one source and turns its answer into an optional fact: any error,
// a failure as much as "nothing to report", leaves the fact absent. Each
// question is a bulkhead: a reader that panics loses its own fact, not the
// render, and the panic is kept with the question's time for diagnosis.
func read[T any](r *runs, source string, ask func() (T, error)) model.Optional[T] {
	start := r.clock.Now()
	v, err := guarded(ask)
	run := model.SourceRun{Source: source, Took: r.clock.Now().Sub(start)}
	switch {
	case errors.Is(err, errPanicked):
		run.Outcome = model.SourcePanicked
	case errors.Is(err, repository.ErrNone):
		run.Outcome = model.SourceNone
	case err != nil:
		run.Outcome = model.SourceFailed
	}
	r.add(run)
	if err != nil {
		return model.Optional[T]{}
	}
	return model.Some(v)
}

// guarded runs ask, turning a panic into errPanicked.
func guarded[T any](ask func() (T, error)) (v T, err error) {
	defer func() {
		if recover() != nil {
			var zero T
			v, err = zero, errPanicked
		}
	}()
	return ask()
}
