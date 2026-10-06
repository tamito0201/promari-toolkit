package dashboard

import (
	"bytes"
	"context"
	"encoding/json/v2"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"promari-statusline/internal/application/usecase"
	"promari-statusline/internal/domain/model"
	"promari-statusline/internal/domain/service"
)

var t0 = time.Date(2026, 10, 6, 2, 0, 0, 0, time.UTC)

type fixed usecase.Snapshot

func (f fixed) Execute(context.Context) usecase.Snapshot { return usecase.Snapshot(f) }

func sample() usecase.Snapshot {
	return usecase.Snapshot{
		At:      t0,
		InputAt: t0.Add(-time.Minute),
		Live:    true,
		Session: model.Session{Name: "review", Model: "Opus 5.5", ProjectDir: "/work/promari", Version: "2.1.289"},
		Headline: usecase.Headline{
			ContextPct: model.Some(31.0),
			FiveHour:   model.Some(model.RateWindow{UsedPct: 23, ResetsAt: t0.Add(time.Hour)}),
			TodayUSD:   model.Some(142.04),
			Running:    4,
		},
		Bands: bands(),
		RateHistory: model.RateHistory{
			{At: t0.Add(-time.Hour), FiveHour: model.Some(0.0)},
			{At: t0.Add(-time.Minute), SevenDay: model.Some(83.0)},
		},
		ContextHistory: []model.Sample{{At: t0.Add(-time.Minute), Tokens: 62000}},
		CodexHistory: []model.CodexRatePoint{
			{At: t0.Add(-time.Minute), Primary: model.Some(model.CodexWindow{UsedPct: 0, WindowMinutes: 10080})},
			{At: t0, Secondary: model.Some(model.CodexWindow{UsedPct: 5, WindowMinutes: 300})},
		},
		LimitsAt: t0.Add(-time.Minute),
		Measurements: map[string]usecase.Measurement{
			"changed":   {Value: model.Some(0.0)},
			"blockCost": {Basis: model.Basis{Kind: model.BasisNoBlock}},
		},
		Groups: []model.Group{
			{Band: model.BandLimits, Title: "🧠 Context", Tone: model.ToneAccent, Chips: []model.Chip{{
				{Text: "██", Tone: model.ToneGood}, {Text: "░░░", Tone: model.ToneMuted}, {Text: " 31%", Bold: true},
			}}},
			{Band: model.BandLimits, Chips: []model.Chip{{{Text: "🤖 Codex", Tone: model.ToneBrand}, {Text: "100%", Tone: model.ToneDanger, Alarm: true}}}},
		},
	}
}

// bands are the bands of the domain in the use case's terms, as the snapshot carries them.
func bands() []usecase.Band {
	infos := service.Bands()
	out := make([]usecase.Band, 0, len(infos))
	for _, b := range infos {
		out = append(out, usecase.Band{Band: b.Band, Name: b.Name, Question: b.Question})
	}
	return out
}

func get(t *testing.T, h http.Handler, host, path string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, path, nil)
	req.Host = host
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func TestSnapshotJSON(t *testing.T) {
	t.Parallel()
	rec := get(t, Server{Snapshot: fixed(sample()), Version: "1.13.0"}.Handler(), "localhost:4646", "/api/snapshot")
	if rec.Code != http.StatusOK || !strings.HasPrefix(rec.Header().Get("Content-Type"), "application/json") || rec.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("status %d, headers %v", rec.Code, rec.Header())
	}
	var got map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	want := map[string]any{
		"version":  "1.13.0",
		"at":       "2026-10-06T02:00:00Z",
		"inputAt":  "2026-10-06T01:59:00Z",
		"live":     true,
		"limitsAt": "2026-10-06T01:59:00Z",
		"measurements": map[string]any{
			"changed":   map[string]any{"value": 0.0},
			"blockCost": map[string]any{"note": "稼働枠なし"},
		},
		"rateHistory": []any{
			map[string]any{"at": "2026-10-06T01:00:00Z", "fiveHour": 0.0},
			map[string]any{"at": "2026-10-06T01:59:00Z", "sevenDay": 83.0},
		},
		"contextHistory": []any{map[string]any{"at": "2026-10-06T01:59:00Z", "tokens": 62000.0}},
		"codexHistory": []any{
			map[string]any{"at": "2026-10-06T01:59:00Z", "primary": map[string]any{"usedPct": 0.0, "windowMinutes": 10080.0}},
			map[string]any{"at": "2026-10-06T02:00:00Z", "secondary": map[string]any{"usedPct": 5.0, "windowMinutes": 300.0}},
		},
		"session": map[string]any{"name": "review", "model": "Opus 5.5", "project": "promari", "version": "2.1.289"},
		// Absent values are left out: no sevenDay and no sessionUsd, not zeros.
		"headline": map[string]any{
			"contextPct": 31.0,
			"fiveHour":   map[string]any{"usedPct": 23.0, "resetsAt": "2026-10-06T03:00:00Z"},
			"todayUsd":   142.04,
			"running":    4.0,
		},
	}
	for key, w := range want {
		g, _ := json.Marshal(got[key], json.Deterministic(true))
		e, _ := json.Marshal(w, json.Deterministic(true))
		if !bytes.Equal(g, e) {
			t.Errorf("%s = %s, want %s", key, g, e)
		}
	}
	bands, _ := got["bands"].([]any)
	if len(bands) != 6 {
		t.Errorf("bands %v", got["bands"])
	}
	// Members are compared in sorted order: maps have no order of their own.
	groups, _ := json.Marshal(got["groups"], json.Deterministic(true))
	wantGroups := `[{"band":2,"chips":[{"spans":[{"text":"██","tone":"good"},{"text":"░░░","tone":"muted"},{"bold":true,"text":" 31%"}]}],"title":"🧠 Context","tone":"accent"},` +
		`{"band":2,"chips":[{"spans":[{"text":"🤖 Codex","tone":"brand"},{"alarm":true,"text":"100%","tone":"danger"}]}],"title":"","tone":""}]`
	if string(groups) != wantGroups {
		t.Errorf("groups\n %s\nwant\n %s", groups, wantGroups)
	}
}

func TestSnapshotOfNothing(t *testing.T) {
	t.Parallel()
	rec := get(t, Server{Snapshot: fixed(usecase.Snapshot{At: t0})}.Handler(), "127.0.0.1:4646", "/api/snapshot")
	body := rec.Body.String()
	// Lists are empty lists, never null: the page maps over them.
	for _, part := range []string{`"bands":[]`, `"groups":[]`, `"session":{}`, `"headline":{"running":0}`, `"rateHistory":[]`, `"contextHistory":[]`} {
		if !strings.Contains(body, part) {
			t.Errorf("%s is missing from %s", part, body)
		}
	}
	if strings.Contains(body, "inputAt") {
		t.Errorf("an unknown time is written: %s", body)
	}
}

func TestThePageIsServed(t *testing.T) {
	t.Parallel()
	h := Server{Snapshot: fixed(sample())}.Handler()
	for path, kind := range map[string]string{"/": "text/html", "/app.js": "text/javascript", "/app.css": "text/css"} {
		rec := get(t, h, "localhost:4646", path)
		if rec.Code != http.StatusOK || !strings.HasPrefix(rec.Header().Get("Content-Type"), kind) {
			t.Errorf("%s: status %d, type %q", path, rec.Code, rec.Header().Get("Content-Type"))
		}
	}
	if rec := get(t, h, "localhost:4646", "/"); !strings.Contains(rec.Body.String(), `<script type="module" src="app.js">`) {
		t.Error("the page does not load its script")
	}
	if rec := get(t, h, "localhost:4646", "/missing.js"); rec.Code != http.StatusNotFound {
		t.Errorf("a missing file: %d", rec.Code)
	}
}

// A page on another site that points its own name at 127.0.0.1 still sends
// that name; only names of this machine are answered.
func TestOnlyThisMachineIsAnswered(t *testing.T) {
	t.Parallel()
	h := Server{Snapshot: fixed(sample())}.Handler()
	for _, host := range []string{"evil.example:4646", "evil.example", "192.168.1.10:4646", "localhost.evil.example:4646", ""} {
		if rec := get(t, h, host, "/api/snapshot"); rec.Code != http.StatusMisdirectedRequest || strings.Contains(rec.Body.String(), "review") {
			t.Errorf("Host %q: status %d, body %q", host, rec.Code, rec.Body.String())
		}
	}
	for _, host := range []string{"localhost:4646", "LOCALHOST:4646", "127.0.0.1:4646", "127.0.0.1", "[::1]:4646", "localhost"} {
		rec := get(t, h, host, "/api/snapshot")
		if rec.Code != http.StatusOK {
			t.Errorf("Host %q: status %d", host, rec.Code)
		}
		csp := rec.Header().Get("Content-Security-Policy")
		if !strings.Contains(csp, "default-src 'self'") || !strings.Contains(csp, "frame-ancestors 'none'") ||
			rec.Header().Get("X-Content-Type-Options") != "nosniff" || rec.Header().Get("Referrer-Policy") != "no-referrer" {
			t.Errorf("Host %q: headers %v", host, rec.Header())
		}
	}
}

func TestOnlyGetIsAnswered(t *testing.T) {
	t.Parallel()
	req := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/api/snapshot", nil)
	req.Host = "localhost:4646"
	rec := httptest.NewRecorder()
	Server{Snapshot: fixed(sample())}.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("POST: %d", rec.Code)
	}
}

func TestServeUntilTheContextEnds(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithCancel(t.Context())
	out := &syncBuffer{}
	done := make(chan error, 1)
	go func() { done <- Server{Snapshot: fixed(sample())}.Serve(ctx, "127.0.0.1:0", out) }()

	addr := waitForAddr(t, out)
	if !strings.HasPrefix(addr, "localhost:") {
		t.Errorf("the address is shown as %q, want localhost", addr)
	}
	req, _ := http.NewRequestWithContext(t.Context(), http.MethodGet, "http://"+addr+"/api/snapshot", http.NoBody)
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(res.Body)
	_ = res.Body.Close()
	if res.StatusCode != http.StatusOK || !strings.Contains(string(body), `"live":true`) {
		t.Errorf("status %d, body %s", res.StatusCode, body)
	}
	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Errorf("Serve returned %v after the context ended", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Serve did not return after the context ended")
	}
}

func TestServeOnAnAddressInUse(t *testing.T) {
	t.Parallel()
	var lc net.ListenConfig
	ln, err := lc.Listen(t.Context(), "tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = ln.Close() }()
	err = Server{Snapshot: fixed(sample())}.Serve(t.Context(), ln.Addr().String(), io.Discard)
	if err == nil || !strings.Contains(err.Error(), "listen on "+ln.Addr().String()) {
		t.Errorf("err = %v, want a listen error naming the address", err)
	}
}

func TestHelpers(t *testing.T) {
	t.Parallel()
	if got := toneName(model.Tone(200)); got != "" {
		t.Errorf("an unknown tone is named %q", got)
	}
	for in, want := range map[string]string{"127.0.0.1:4646": "localhost:4646", "[::1]:4646": "[::1]:4646", "nonsense": "nonsense"} {
		if got := displayAddr(in); got != want {
			t.Errorf("displayAddr(%q) = %q, want %q", in, got, want)
		}
	}
	// The project falls back to the working directory.
	s := sample()
	s.Session.ProjectDir, s.Session.Dir = "", "/elsewhere/tool"
	if got := toDTO(&s, "").Session.Project; got != "tool" {
		t.Errorf("project %q", got)
	}
}

// syncBuffer is a buffer the server writes while the test reads.
type syncBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *syncBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *syncBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

// waitForAddr waits for the line Serve prints and returns the address in it.
func waitForAddr(t *testing.T, out *syncBuffer) string {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if _, rest, ok := strings.Cut(out.String(), "http://"); ok {
			if addr, _, ok := strings.Cut(rest, "/"); ok {
				return addr
			}
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("Serve printed no address")
	return ""
}

func TestServeOnAClosedListener(t *testing.T) {
	t.Parallel()
	var lc net.ListenConfig
	ln, err := lc.Listen(t.Context(), "tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	_ = ln.Close()
	if err := (Server{Snapshot: fixed(sample())}).serve(t.Context(), ln); err == nil || !strings.HasPrefix(err.Error(), "serve: ") {
		t.Errorf("err = %v, want the server's own error", err)
	}
}

// blocking is a snapshot that does not answer until it is released, whatever
// its request's context says: a request still in flight at Ctrl-C.
type blocking struct {
	entered chan struct{}
	release chan struct{}
}

func (b blocking) Execute(context.Context) usecase.Snapshot {
	close(b.entered)
	<-b.release
	return usecase.Snapshot{}
}

func TestShutdownThatOutlivesItsGrace(t *testing.T) {
	t.Parallel()
	var lc net.ListenConfig
	ln, err := lc.Listen(t.Context(), "tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	b := blocking{entered: make(chan struct{}), release: make(chan struct{})}
	ctx, cancel := context.WithCancel(t.Context())
	done := make(chan error, 1)
	go func() { done <- Server{Snapshot: b, Grace: 20 * time.Millisecond}.serve(ctx, ln) }()

	answered := make(chan struct{})
	go func() {
		defer close(answered)
		req, _ := http.NewRequestWithContext(context.WithoutCancel(t.Context()), http.MethodGet, "http://"+ln.Addr().String()+"/api/snapshot", http.NoBody)
		if res, err := http.DefaultClient.Do(req); err == nil {
			_ = res.Body.Close()
		}
	}()
	<-b.entered
	cancel()
	select {
	case err := <-done:
		if err == nil || !strings.Contains(err.Error(), "shut down: ") {
			t.Errorf("err = %v, want the shutdown's deadline", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("serve did not give up after its grace")
	}
	close(b.release)
	<-answered
}
