package usecase_test

import (
	"context"
	"errors"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"promari-statusline/internal/application/usecase"
	"promari-statusline/internal/domain/model"
	"promari-statusline/internal/domain/repository"
)

var t0 = time.Date(2026, 10, 3, 4, 9, 0, 0, time.UTC)

var errBroken = errors.New("broken")

type clock time.Time

func (c clock) Now() time.Time { return time.Time(c) }

// world answers every source port. Each answer is what the test set; err makes
// every reader fail. asked records the questions, guarded for the concurrent
// readers.
type world struct {
	mu    sync.Mutex
	asked []string
	err   error

	git      model.Git
	pull     model.PullRequest
	spend    model.Spend
	codex    model.CodexLimits
	tools    model.ToolStats
	todos    model.Todos
	track    model.Track
	incident model.Incident
	latest   string
	account  string
	machine  model.Machine
}

func (w *world) ask(question string) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.asked = append(w.asked, question)
	return w.err
}

func (w *world) Git(_ context.Context, dir string) (model.Git, error) {
	return w.git, w.ask("git " + dir)
}

func (w *world) PullRequest(_ context.Context, dir, branch string) (model.PullRequest, error) {
	return w.pull, w.ask("pull " + dir + " " + branch)
}

func (w *world) Spend(_ context.Context, input []byte) (model.Spend, error) {
	return w.spend, w.ask("spend " + string(input))
}

func (w *world) Codex(context.Context) (model.CodexLimits, error) { return w.codex, w.ask("codex") }

func (w *world) ToolStats(_ context.Context, transcript string) (model.ToolStats, error) {
	return w.tools, w.ask("tools " + transcript)
}

func (w *world) Todos(_ context.Context, key string) (model.Todos, error) {
	return w.todos, w.ask("todos " + key)
}

func (w *world) Track(context.Context) (model.Track, error) { return w.track, w.ask("track") }

func (w *world) Incident(context.Context) (model.Incident, error) {
	return w.incident, w.ask("incident")
}

func (w *world) Latest(context.Context) (string, error)  { return w.latest, w.ask("latest") }
func (w *world) Account(context.Context) (string, error) { return w.account, w.ask("account") }

func (w *world) Machine(_ context.Context, dir string) model.Machine {
	_ = w.ask("machine " + dir)
	return w.machine
}

func (w *world) sources() usecase.Sources {
	return usecase.Sources{
		Git: w, Pulls: w, Spend: w, Codex: w, Tools: w, Todos: w, Track: w, Incident: w, Release: w, Account: w, Machine: w,
	}
}

// memory is every store of a render, in memory. readOnly makes every write fail.
type memory struct {
	readOnly   bool
	activities map[string]model.Activity
	limits     model.RateLimits
	limitsAt   time.Time
	history    model.RateHistory
	input      []byte
	width      string
	demo       bool
	claude     []model.RateLimits
	codex      []model.CodexLimits
	cells      int
}

func (m *memory) write() error {
	if m.readOnly {
		return errBroken
	}
	return nil
}

func (m *memory) Load(key string) model.Activity { return m.activities[key] }

func (m *memory) Save(key string, a model.Activity) error {
	if err := m.write(); err != nil {
		return err
	}
	m.activities[key] = a
	return nil
}

func (m *memory) Last() (model.RateLimits, time.Time, error) {
	if m.limits.Empty() {
		return model.RateLimits{}, time.Time{}, repository.ErrNone
	}
	return m.limits, m.limitsAt, nil
}

func (m *memory) Remember(l model.RateLimits, at time.Time) error {
	if err := m.write(); err != nil {
		return err
	}
	m.limits, m.limitsAt = l, at
	return nil
}

func (m *memory) History() model.RateHistory { return m.history }

func (m *memory) SaveHistory(h model.RateHistory) error {
	if err := m.write(); err != nil {
		return err
	}
	m.history = h
	return nil
}

func (m *memory) PostClaude(l model.RateLimits, _ time.Time) error {
	if err := m.write(); err != nil {
		return err
	}
	m.claude = append(m.claude, l)
	return nil
}

func (m *memory) PostCodex(l model.CodexLimits, _ time.Time) error {
	if err := m.write(); err != nil {
		return err
	}
	m.codex = append(m.codex, l)
	return nil
}

func (m *memory) Width() (int, string) { return m.cells, "test" }
func (m *memory) Input(raw []byte)     { m.input = raw }
func (m *memory) BlinkDemo() bool      { return m.demo }

func (m *memory) recordWidth(source string, budget int) {
	m.width = source + " " + strings.Repeat("#", budget)
}

// recorder adapts memory to repository.Recorder, whose Width method would
// clash with repository.Terminal's.
type recorder struct{ m *memory }

func (r recorder) Input(raw []byte)                { r.m.Input(raw) }
func (r recorder) Width(source string, budget int) { r.m.recordWidth(source, budget) }

func newMemory() *memory {
	return &memory{activities: map[string]model.Activity{}, cells: 200}
}

func render(w *world, m *memory, at time.Time, s model.Session) usecase.Rendered {
	u := usecase.NewRenderStatusLine(usecase.RenderDeps{
		Clock: clock(at), Sources: w.sources(), Activities: m, Limits: m, Board: m, Terminal: m, Recorder: recorder{m}, Switches: m,
	})
	return u.Execute(context.Background(), usecase.RenderRequest{Session: s, Raw: []byte(`{"raw":true}`)})
}

// text flattens the rendered lines to one string per line.
func text(r usecase.Rendered) []string {
	out := make([]string, 0, len(r.Lines))
	for _, line := range r.Lines {
		var b strings.Builder
		for i, item := range line.Items {
			if i > 0 {
				b.WriteString(" | ")
			}
			b.WriteString(item.Chip.Text())
		}
		out = append(out, b.String())
	}
	return out
}

func contains(lines []string, part string) bool {
	return slices.ContainsFunc(lines, func(line string) bool { return strings.Contains(line, part) })
}

func session() model.Session {
	return model.Session{
		Reported: true, ID: "s1", PromptID: "p1", Dir: "/work", TranscriptPath: "/t.jsonl", Model: "Opus", Version: "2.1.34",
		Context: model.ContextWindow{Size: 200_000, UsedPct: model.Some(42.0), Current: 84_000},
	}
}

func TestRenderGathersEveryFact(t *testing.T) {
	t.Parallel()
	w := &world{
		git:      model.Git{Branch: "develop", Changed: 2},
		pull:     model.PullRequest{Number: 7, Passed: 3},
		spend:    model.Spend{Today: model.Some(model.Amount{Text: "45.67", Value: 45.67})},
		codex:    model.CodexLimits{Primary: model.Some(model.CodexWindow{UsedPct: 5, WindowMinutes: 300}), SeenAt: t0},
		tools:    model.ToolStats{Total: 3, Top: []model.ToolCount{{Name: "Read", Count: 3}}},
		todos:    model.Todos{Done: 1, Total: 2},
		track:    model.Track{Title: "Take Five"},
		incident: model.Incident{Indicator: "minor", Description: "Slow"},
		latest:   "2.1.287",
		account:  "someone@example.com",
		machine:  model.Machine{Sessions: 2},
	}
	m := newMemory()
	got := render(w, m, t0, session())
	lines := text(got)
	for _, want := range []string{
		"🌐 API minor: Slow", "42%", "🤖 Codex 5h", "Today $45.67", "Turns ×1", "✅ Todo 1/2", "Tools ×3 Read3",
		"develop", "📝 2 Files", "🔀 PR #7 CI ✅ 3", "Opus", "⛵ Proc ×2", "👤 someone", "🆙 Update v2.1.287", "Take Five",
	} {
		if !contains(lines, want) {
			t.Errorf("the status line lacks %q:\n  %s", want, strings.Join(lines, "\n  "))
		}
	}
	if !got.At.Equal(t0) {
		t.Errorf("At = %v, want the clock's %v", got.At, t0)
	}
	slices.Sort(w.asked)
	wantAsked := []string{
		"account", "codex", "git /work", "incident", "latest", "machine /work", "pull /work develop",
		`spend {"raw":true}`, "todos s1", "tools /t.jsonl", "track",
	}
	if !slices.Equal(w.asked, wantAsked) {
		t.Errorf("asked %q, want %q", w.asked, wantAsked)
	}
	if len(m.codex) != 1 || m.codex[0] != w.codex || len(m.claude) != 0 {
		t.Errorf("posted Codex %+v and Claude %+v; want the Codex usage that was read and, without rate limits, nothing for Claude", m.codex, m.claude)
	}
	if string(m.input) != `{"raw":true}` || m.width != "test "+strings.Repeat("#", 198) {
		t.Errorf("recorded input %q and width %q", m.input, m.width)
	}
}

func TestRenderWithoutFacts(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name  string
		world *world
	}{
		{"every source fails", &world{err: errBroken, git: model.Git{Branch: "develop"}}},
		{"every source has nothing", &world{err: repository.ErrNone}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			lines := text(render(tt.world, newMemory(), t0, session()))
			if !contains(lines, "🧠 Context") || !contains(lines, "🧭 Env | Opus") || !contains(lines, "🕐 04:09") {
				t.Errorf("what is known without the sources is missing:\n  %s", strings.Join(lines, "\n  "))
			}
			for _, absent := range []string{"🌿 Git", "🔀 PR", "🤖 Codex", "🎵 Music", "🚨 Alert", "Today"} {
				if contains(lines, absent) {
					t.Errorf("%q is shown although its source gave nothing", absent)
				}
			}
			if slices.ContainsFunc(tt.world.asked, func(q string) bool { return strings.HasPrefix(q, "pull") }) {
				t.Error("the pull request was asked for although there is no branch to ask about")
			}
		})
	}
}

func TestRenderAsksOnlyWhatItCan(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		session model.Session
		absent  []string
	}{
		{"no transcript: the tools are not counted", model.Session{Reported: true, ID: "s1"}, []string{"tools"}},
		{"an id that cannot name a file: no to-dos", model.Session{Reported: true, ID: "../x", TranscriptPath: "/t"}, []string{"todos"}},
		{"nothing reported", model.Session{}, []string{"tools", "todos"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			w := &world{}
			render(w, newMemory(), t0, tt.session)
			for _, prefix := range tt.absent {
				if slices.ContainsFunc(w.asked, func(q string) bool { return strings.HasPrefix(q, prefix) }) {
					t.Errorf("asked for %s: %q", prefix, w.asked)
				}
			}
			if !slices.Contains(w.asked, "machine .") && tt.session.Dir == "" {
				t.Errorf("the machine was not read for the default directory: %q", w.asked)
			}
		})
	}
}

func TestRenderTracksActivity(t *testing.T) {
	t.Parallel()
	t.Run("renders add up to turns and working time", func(t *testing.T) {
		t.Parallel()
		w, m := &world{err: repository.ErrNone}, newMemory()
		s := session()
		render(w, m, t0, s)
		s.PromptID = "p2"
		lines := text(render(w, m, t0.Add(2*time.Minute), s))
		if !contains(lines, "Turns ×2") || !contains(lines, "Active 2m") {
			t.Errorf("two prompts two minutes apart:\n  %s", strings.Join(lines, "\n  "))
		}
		if a := m.activities["s1"]; a.Turns != 2 || a.Worked() != 2*time.Minute {
			t.Errorf("stored activity = %+v", a)
		}
	})
	t.Run("a session without a usable id has no activity", func(t *testing.T) {
		t.Parallel()
		m := newMemory()
		s := session()
		s.ID = "a/b"
		lines := text(render(&world{err: repository.ErrNone}, m, t0, s))
		if contains(lines, "Active") || contains(lines, "Turns") || len(m.activities) != 0 {
			t.Errorf("activity shown or stored for an unusable id:\n  %s", strings.Join(lines, "\n  "))
		}
	})
	t.Run("without a context window nothing is observed", func(t *testing.T) {
		t.Parallel()
		m := newMemory()
		render(&world{err: repository.ErrNone}, m, t0, model.Session{Reported: true, ID: "s1"})
		if len(m.activities) != 0 {
			t.Errorf("stored %+v without a context window", m.activities)
		}
	})
	t.Run("an activity that cannot be saved is still shown", func(t *testing.T) {
		t.Parallel()
		m := newMemory()
		m.readOnly = true
		if lines := text(render(&world{err: repository.ErrNone}, m, t0, session())); !contains(lines, "Turns ×1") {
			t.Errorf("the activity of this render is missing:\n  %s", strings.Join(lines, "\n  "))
		}
	})
}

func TestRenderRateLimits(t *testing.T) {
	t.Parallel()
	limits := model.RateLimits{FiveHour: model.Some(model.RateWindow{UsedPct: 50, ResetsAt: t0.Add(2 * time.Hour)})}
	withLimits := session()
	withLimits.Limits = limits

	t.Run("reported limits are remembered and recorded", func(t *testing.T) {
		t.Parallel()
		m := newMemory()
		lines := text(render(&world{err: repository.ErrNone}, m, t0, withLimits))
		if !contains(lines, "⚡ Claude 5h") || contains(lines, "(10/") {
			t.Errorf("fresh limits:\n  %s", strings.Join(lines, "\n  "))
		}
		if m.limits != limits || !m.limitsAt.Equal(t0) || len(m.history) != 1 {
			t.Errorf("remembered %+v at %v with history %v", m.limits, m.limitsAt, m.history)
		}
		if len(m.claude) != 1 || m.claude[0] != limits || len(m.codex) != 0 {
			t.Errorf("posted Claude %+v and Codex %+v; want the reported limits and, without Codex usage, nothing for Codex", m.claude, m.codex)
		}
	})
	t.Run("a render without limits shows the remembered ones and records nothing", func(t *testing.T) {
		t.Parallel()
		m := newMemory()
		m.limits, m.limitsAt = limits, t0.Add(-7*time.Hour)
		lines := text(render(&world{err: repository.ErrNone}, m, t0, session()))
		if !contains(lines, "⚡ Claude 5h") || !contains(lines, "(10/2)") {
			t.Errorf("remembered limits with their date:\n  %s", strings.Join(lines, "\n  "))
		}
		if len(m.history) != 0 {
			t.Errorf("a remembered value was recorded as a new measurement: %v", m.history)
		}
		if len(m.claude) != 0 {
			t.Errorf("a remembered value was posted as a new measurement: %+v", m.claude)
		}
	})
	t.Run("limits remembered for a week are not shown", func(t *testing.T) {
		t.Parallel()
		m := newMemory()
		m.limits, m.limitsAt = limits, t0.Add(-7*24*time.Hour)
		if lines := text(render(&world{err: repository.ErrNone}, m, t0, session())); contains(lines, "⚡ Claude") {
			t.Errorf("week-old limits are shown:\n  %s", strings.Join(lines, "\n  "))
		}
	})
	t.Run("nothing remembered, nothing shown", func(t *testing.T) {
		t.Parallel()
		if lines := text(render(&world{err: repository.ErrNone}, newMemory(), t0, session())); contains(lines, "⚡ Claude") {
			t.Errorf("limits out of nowhere:\n  %s", strings.Join(lines, "\n  "))
		}
	})
	t.Run("usage that grows towards the reset is forecast", func(t *testing.T) {
		t.Parallel()
		m := newMemory()
		m.history = model.RateHistory{{At: t0.Add(-10 * time.Minute), FiveHour: model.Some(40.0)}}
		lines := text(render(&world{err: repository.ErrNone}, m, t0, withLimits))
		if !contains(lines, "5h 枯渇まで 50m (reset前)") {
			t.Errorf("no forecast:\n  %s", strings.Join(lines, "\n  "))
		}
	})
	t.Run("memory that cannot be written still shows this render's limits", func(t *testing.T) {
		t.Parallel()
		m := newMemory()
		m.readOnly = true
		if lines := text(render(&world{err: repository.ErrNone}, m, t0, withLimits)); !contains(lines, "⚡ Claude 5h") {
			t.Errorf("limits missing:\n  %s", strings.Join(lines, "\n  "))
		}
	})
}

func TestRenderFitsTheTerminal(t *testing.T) {
	t.Parallel()
	w := &world{git: model.Git{Branch: "develop", Changed: 2, Behind: 16}, machine: model.Machine{Sessions: 6}}
	wide, narrow := newMemory(), newMemory()
	narrow.cells = 50
	wideLines := render(w, wide, t0, session()).Lines
	narrowLines := render(&world{git: w.git, machine: w.machine}, narrow, t0, session()).Lines
	if len(narrowLines) <= len(wideLines) {
		t.Errorf("a terminal of 50 cells took %d lines, one of 200 cells %d", len(narrowLines), len(wideLines))
	}
	if narrow.width != "test "+strings.Repeat("#", 48) {
		t.Errorf("recorded width %q, want a budget of 48", narrow.width)
	}
}

func TestRenderBlinkDemo(t *testing.T) {
	t.Parallel()
	m := newMemory()
	m.demo = true
	alarmed := false
	for _, line := range render(&world{err: repository.ErrNone}, m, t0, session()).Lines {
		for _, item := range line.Items {
			for _, span := range item.Chip {
				alarmed = alarmed || span.Alarm
			}
		}
	}
	if !alarmed {
		t.Error("the blink demo raised no alarm at 42 % usage")
	}
}
