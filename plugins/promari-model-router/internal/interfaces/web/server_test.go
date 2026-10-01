package web

import (
	"bytes"
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"io"
	"iter"
	"net"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/getkin/kin-openapi/openapi3"
	gocmp "github.com/google/go-cmp/cmp"
	"github.com/samber/do/v2"

	"github.com/tamito0201/promari-toolkit/plugins/promari-model-router/internal/application/usecase"
	"github.com/tamito0201/promari-toolkit/plugins/promari-model-router/internal/di"
	"github.com/tamito0201/promari-toolkit/plugins/promari-model-router/internal/domain/model"
	"github.com/tamito0201/promari-toolkit/plugins/promari-model-router/internal/domain/repository"
	"github.com/tamito0201/promari-toolkit/plugins/promari-model-router/internal/infrastructure/clock"
	"github.com/tamito0201/promari-toolkit/plugins/promari-model-router/internal/interfaces/web/api"
)

// fakeLedger yields fixed entries, then err (if any), and records the lower bound.
type fakeLedger struct {
	repository.LedgerReader
	entries []model.Entry
	err     error
	from    *[]time.Time
}

func (l fakeLedger) Since(_ context.Context, from time.Time) iter.Seq2[model.Entry, error] {
	if l.from != nil {
		*l.from = append(*l.from, from)
	}
	return func(yield func(model.Entry, error) bool) {
		for i := range l.entries {
			if e := l.entries[i]; !e.At.Before(from) && !yield(e, nil) {
				return
			}
		}
		if l.err != nil {
			yield(model.Entry{}, l.err)
		}
	}
}

func testDeps(t *testing.T) Deps {
	t.Helper()
	tmp := t.TempDir()
	for k, v := range map[string]string{
		"HOME": filepath.Join(tmp, "home"), "CLAUDE_PLUGIN_DATA": filepath.Join(tmp, "data"), "CLAUDE_PROJECT_DIR": filepath.Join(tmp, "project"),
	} {
		t.Setenv(k, v)
	}
	c := di.New()
	t.Cleanup(func() { _ = c.Shutdown() })
	return Deps{
		Report: do.MustInvoke[usecase.ReportUseCase](c), Explain: do.MustInvoke[usecase.ExplainUseCase](c),
		Feed: do.MustInvoke[usecase.FeedUseCase](c), Serve: do.MustInvoke[model.Settings](c).Serve,
	}
}

func decode(t *testing.T, raw []byte) any {
	t.Helper()
	var v any
	if err := json.Unmarshal(raw, &v); err != nil {
		t.Fatalf("not JSON: %v\n%s", err, raw)
	}
	return v
}

func asJSON(t *testing.T, v any) any {
	t.Helper()
	raw, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return decode(t, raw)
}

func TestHandler(t *testing.T) {
	now := time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC)
	d7 := testDeps(t).Report.DefaultDays
	explain := func(d Deps, in usecase.ExplainInput) func(*testing.T) any {
		return func(t *testing.T) any {
			t.Helper()
			out, err := convert[api.Explanation](d.Explain.Execute(in))
			if err != nil {
				t.Fatal(err)
			}
			return asJSON(t, out)
		}
	}
	tests := []struct {
		name       string
		edit       func(d *Deps, from *[]time.Time)
		host       string // "" = 127.0.0.1
		method     string
		target     string
		body       string
		wantStatus int
		want       func(d Deps) func(*testing.T) any // JSON body; nil = not checked
		wantFrom   []time.Time
	}{
		{
			name: "healthz", method: http.MethodGet, target: "/healthz", wantStatus: http.StatusOK,
			want: func(Deps) func(*testing.T) any {
				return func(*testing.T) any { return map[string]any{"status": "ok", "version": Version} }
			},
		},
		{
			name: "report with the default window",
			edit: func(d *Deps, from *[]time.Time) {
				d.Report.Ledger, d.Report.Clock = fakeLedger{from: from}, clock.Fixed{At: now}
			},
			method: http.MethodGet, target: "/v1/report", wantStatus: http.StatusOK,
			want: func(d Deps) func(*testing.T) any {
				return func(t *testing.T) any {
					t.Helper()
					out, err := convert[api.Report](usecase.BuildReport(nil, d.Report.Prices))
					if err != nil {
						t.Fatal(err)
					}
					return asJSON(t, out)
				}
			},
			wantFrom: []time.Time{now.Add(-time.Duration(d7) * 24 * time.Hour)},
		},
		{
			name: "report with days",
			edit: func(d *Deps, from *[]time.Time) {
				d.Report.Ledger, d.Report.Clock = fakeLedger{from: from}, clock.Fixed{At: now}
			},
			method: http.MethodGet, target: "/v1/report?days=0.5", wantStatus: http.StatusOK,
			wantFrom: []time.Time{now.Add(-12 * time.Hour)},
		},
		{name: "report rejects days below the minimum", method: http.MethodGet, target: "/v1/report?days=0", wantStatus: http.StatusBadRequest},
		{name: "report rejects days that are not numbers", method: http.MethodGet, target: "/v1/report?days=x", wantStatus: http.StatusBadRequest},
		{
			name:   "report fails with 500 when the ledger fails",
			edit:   func(d *Deps, _ *[]time.Time) { d.Report.Ledger = fakeLedger{err: errors.New("ledger unreadable")} },
			method: http.MethodGet, target: "/v1/report?days=1", wantStatus: http.StatusInternalServerError,
		},
		{
			name: "explain with the defaults", method: http.MethodPost, target: "/v1/explain",
			body: `{"prompt":"この関数の単体テストを書いて"}`, wantStatus: http.StatusOK,
			want: func(d Deps) func(*testing.T) any {
				return explain(d, usecase.ExplainInput{Prompt: "この関数の単体テストを書いて", SubagentType: "general-purpose"})
			},
		},
		{
			name: "explain honours the subagent type and the session model", method: http.MethodPost, target: "/v1/explain",
			body: `{"prompt":"探して","subagent_type":"Explore","session_model":"claude-sonnet-4-6"}`, wantStatus: http.StatusOK,
			want: func(d Deps) func(*testing.T) any {
				return explain(d, usecase.ExplainInput{Prompt: "探して", SubagentType: "Explore", SessionModel: "claude-sonnet-4-6"})
			},
		},
		{name: "explain rejects unknown fields", method: http.MethodPost, target: "/v1/explain", body: `{"prompt":"x","extra":1}`, wantStatus: http.StatusBadRequest},
		{name: "explain rejects an empty prompt", method: http.MethodPost, target: "/v1/explain", body: `{"prompt":""}`, wantStatus: http.StatusBadRequest},
		{name: "explain rejects a missing body", method: http.MethodPost, target: "/v1/explain", wantStatus: http.StatusBadRequest},
		{name: "unknown path", method: http.MethodGet, target: "/v1/nope", wantStatus: http.StatusNotFound},
		{name: "wrong method", method: http.MethodDelete, target: "/v1/report", wantStatus: http.StatusMethodNotAllowed},
		// A DNS-rebinding page reaches 127.0.0.1 under its own host name.
		{name: "another host is misdirected", host: "evil.example:7457", method: http.MethodGet, target: "/healthz", wantStatus: http.StatusMisdirectedRequest},
		{name: "another host without a port is misdirected", host: "evil.example", method: http.MethodGet, target: "/v1/report", wantStatus: http.StatusMisdirectedRequest},
		{name: "localhost is served", host: "localhost:7457", method: http.MethodGet, target: "/healthz", wantStatus: http.StatusOK},
		{name: "IPv6 loopback is served", host: "[::1]:7457", method: http.MethodGet, target: "/healthz", wantStatus: http.StatusOK},
		{name: "IPv6 loopback without a port is served", host: "[::1]", method: http.MethodGet, target: "/healthz", wantStatus: http.StatusOK},
		{name: "the event stream is guarded too", host: "evil.example", method: http.MethodGet, target: "/v1/events", wantStatus: http.StatusMisdirectedRequest},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d := testDeps(t)
			var from []time.Time
			if tt.edit != nil {
				tt.edit(&d, &from)
			}
			h, err := Handler(d)
			if err != nil {
				t.Fatal(err)
			}
			// Bounded, so a stream that should have been refused cannot hang the test.
			ctx, cancel := context.WithTimeout(t.Context(), 2*time.Second)
			defer cancel()
			req := httptest.NewRequestWithContext(ctx, tt.method, tt.target, strings.NewReader(tt.body))
			req.Host = cmp.Or(tt.host, "127.0.0.1:7457")
			if tt.body != "" {
				req.Header.Set("Content-Type", "application/json")
			}
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, req)
			if rec.Code != tt.wantStatus {
				t.Fatalf("status = %d, want %d\n%s", rec.Code, tt.wantStatus, rec.Body.String())
			}
			if tt.want != nil {
				if diff := gocmp.Diff(tt.want(d)(t), decode(t, rec.Body.Bytes())); diff != "" {
					t.Errorf("body (-want +got):\n%s", diff)
				}
			}
			if diff := gocmp.Diff(tt.wantFrom, from); tt.wantFrom != nil && diff != "" {
				t.Errorf("ledger window (-want +got):\n%s", diff)
			}
		})
	}
}

// TestStrictDirect covers the paths the request validator keeps a real
// request from reaching.
func TestStrictDirect(t *testing.T) {
	tests := []struct {
		name string
		call func(s strict) (any, error)
		want any
		err  bool
	}{
		{
			name: "PostExplain without a body is a 400",
			call: func(s strict) (any, error) { return s.PostExplain(t.Context(), api.PostExplainRequestObject{}) },
			want: api.PostExplain400JSONResponse{Message: "missing body"},
		},
		{
			name: "GetEvents is served by the streaming handler",
			call: func(s strict) (any, error) { return s.GetEvents(t.Context(), api.GetEventsRequestObject{}) },
			want: api.GetEventsResponseObject(nil),
			err:  true,
		},
		{
			name: "writeOK reports a value that cannot be encoded",
			call: func(strict) (any, error) { return nil, writeOK(httptest.NewRecorder(), make(chan int)) },
			err:  true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := tt.call(strict{d: testDeps(t)})
			if (err != nil) != tt.err {
				t.Fatalf("err = %v, want error %v", err, tt.err)
			}
			if diff := gocmp.Diff(tt.want, got); diff != "" {
				t.Errorf("(-want +got):\n%s", diff)
			}
		})
	}
}

// plainWriter is a ResponseWriter without http.Flusher.
type plainWriter struct {
	header http.Header
	code   int
	body   bytes.Buffer
}

func (w *plainWriter) Header() http.Header         { return w.header }
func (w *plainWriter) Write(b []byte) (int, error) { return w.body.Write(b) }
func (w *plainWriter) WriteHeader(code int)        { w.code = code }

// failingFlusher accepts headers but fails every body write.
type failingFlusher struct{ *httptest.ResponseRecorder }

func (failingFlusher) Write([]byte) (int, error) { return 0, errors.New("client gone") }

// feedLedger serves entries by position (entries[i] is at position i+1).
type feedLedger struct {
	repository.LedgerFeed
	entries []model.Entry
	headErr error
	err     error
}

func (l feedLedger) Head(context.Context) (uint, error) { return uint(len(l.entries)), l.headErr }

func (l feedLedger) After(_ context.Context, pos uint, from time.Time) iter.Seq2[repository.Positioned, error] {
	return func(yield func(repository.Positioned, error) bool) {
		for i := int(pos); i < len(l.entries); i++ {
			if !l.entries[i].At.Before(from) && !yield(repository.Positioned{Pos: uint(i + 1), Entry: l.entries[i]}, nil) {
				return
			}
		}
		if l.err != nil {
			yield(repository.Positioned{}, l.err)
		}
	}
}

func TestEvents(t *testing.T) {
	now := time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC)
	entry := model.Entry{At: now.Add(-30 * time.Second), Event: model.EventSubagent, Action: model.ActionInject, Reason: "rule", Target: "haiku", Class: "lookup"}
	line := func(e model.Entry) string {
		raw, err := json.Marshal(ledgerEvent{At: e.At, Event: string(e.Event), Action: string(e.Action), Reason: e.Reason, Target: string(e.Target), Class: string(e.Class)})
		if err != nil {
			t.Fatal(err)
		}
		return "event: ledger\ndata: " + string(raw) + "\n\n"
	}
	bad := `{"message":"since_seconds must be an integer from 0 to 86400"}` + "\n"
	tests := []struct {
		name     string
		ledger   feedLedger
		query    string
		timeout  time.Duration // 0 = the request is already cancelled
		flusher  bool
		failBody bool
		wantCode int
		wantBody string
	}{
		{name: "a writer without Flush is refused", flusher: false, wantCode: http.StatusInternalServerError, wantBody: "streaming unsupported\n"},
		{
			name: "entries since since_seconds are streamed", ledger: feedLedger{entries: []model.Entry{entry}}, query: "?since_seconds=60",
			flusher: true, wantCode: http.StatusOK, wantBody: line(entry),
		},
		{
			name: "without since_seconds only new entries are streamed", ledger: feedLedger{entries: []model.Entry{entry}},
			flusher: true, wantCode: http.StatusOK, wantBody: "",
		},
		// These used to fall back to "now" silently (and "10abc" read as 10).
		{name: "an out-of-range since_seconds is a 400", query: "?since_seconds=86401", flusher: true, wantCode: http.StatusBadRequest, wantBody: bad},
		{name: "a negative since_seconds is a 400", query: "?since_seconds=-1", flusher: true, wantCode: http.StatusBadRequest, wantBody: bad},
		{name: "a malformed since_seconds is a 400", query: "?since_seconds=x", flusher: true, wantCode: http.StatusBadRequest, wantBody: bad},
		{name: "a number with a tail is a 400", query: "?since_seconds=10abc", flusher: true, wantCode: http.StatusBadRequest, wantBody: bad},
		{
			name: "a ledger that cannot be read is a 500", ledger: feedLedger{headErr: errors.New("ledger unreadable")},
			flusher: true, wantCode: http.StatusInternalServerError, wantBody: "ledger unreadable\n",
		},
		{
			name: "a ledger error ends the stream", ledger: feedLedger{err: errors.New("ledger unreadable")}, query: "?since_seconds=60",
			flusher: true, wantCode: http.StatusOK, wantBody: "",
		},
		{
			name:   "an entry that cannot be encoded ends the stream",
			ledger: feedLedger{entries: []model.Entry{{At: time.Date(10000, 1, 1, 0, 0, 0, 0, time.UTC)}}},
			query:  "?since_seconds=60", flusher: true, wantCode: http.StatusOK, wantBody: "",
		},
		{
			name: "a failed write ends the stream", ledger: feedLedger{entries: []model.Entry{entry}}, query: "?since_seconds=60",
			flusher: true, failBody: true, wantCode: http.StatusOK, wantBody: "",
		},
		{
			// The feed used to resume from "the last timestamp + 1ns", which the
			// stored text compares above: the same entry came back every poll.
			name: "polling sends every entry once until the client leaves", ledger: feedLedger{entries: []model.Entry{entry}}, query: "?since_seconds=60",
			timeout: 50 * time.Millisecond, flusher: true, wantCode: http.StatusOK, wantBody: line(entry),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d := Deps{Feed: usecase.FeedUseCase{Ledger: tt.ledger, Clock: clock.Fixed{At: now}}, Serve: model.ServeSettings{SSEPollMS: 1}}
			ctx, cancel := context.WithCancel(t.Context())
			if tt.timeout > 0 {
				ctx, cancel = context.WithTimeout(t.Context(), tt.timeout)
			} else {
				cancel()
			}
			defer cancel()
			req := httptest.NewRequestWithContext(ctx, http.MethodGet, "/v1/events"+tt.query, nil)
			var code int
			var body string
			switch {
			case !tt.flusher:
				w := &plainWriter{header: http.Header{}, code: http.StatusOK}
				strict{d: d}.events(w, req)
				code, body = w.code, w.body.String()
			case tt.failBody:
				rec := httptest.NewRecorder()
				strict{d: d}.events(failingFlusher{rec}, req)
				code, body = rec.Code, rec.Body.String()
			default:
				rec := httptest.NewRecorder()
				strict{d: d}.events(rec, req)
				code, body = rec.Code, rec.Body.String()
				if want := map[bool]string{true: "text/event-stream", false: rec.Header().Get("Content-Type")}[code == http.StatusOK]; rec.Header().Get("Content-Type") != want {
					t.Errorf("Content-Type = %q", rec.Header().Get("Content-Type"))
				}
			}
			if code != tt.wantCode {
				t.Errorf("status = %d, want %d", code, tt.wantCode)
			}
			if diff := gocmp.Diff(tt.wantBody, body); diff != "" {
				t.Errorf("body (-want +got):\n%s", diff)
			}
		})
	}
}

// TestEventsOverHTTP streams through the real mux, a real connection and the
// real ledger: a row whose timestamp ends in zeros arrives exactly once.
func TestEventsOverHTTP(t *testing.T) {
	tests := []struct {
		name string
		at   time.Time
	}{
		{name: "a whole second", at: time.Now().Truncate(time.Second)},
		{name: "a fraction with trailing zeros", at: time.Now().Truncate(10 * time.Microsecond)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmp := t.TempDir()
			for k, v := range map[string]string{"HOME": filepath.Join(tmp, "home"), "CLAUDE_PLUGIN_DATA": filepath.Join(tmp, "data"), "CLAUDE_PROJECT_DIR": filepath.Join(tmp, "project")} {
				t.Setenv(k, v)
			}
			c := di.New()
			t.Cleanup(func() { _ = c.Shutdown() })
			d := testDeps(t)
			d.Feed = do.MustInvoke[usecase.FeedUseCase](c)
			d.Serve.SSEPollMS = 5
			if err := do.MustInvoke[repository.LedgerRepository](c).Append(t.Context(), model.NewEntry(tt.at, model.EventPrompt)); err != nil {
				t.Fatal(err)
			}
			h, err := Handler(d)
			if err != nil {
				t.Fatal(err)
			}
			ts := httptest.NewServer(h)
			t.Cleanup(ts.Close)
			ctx, cancel := context.WithTimeout(t.Context(), 200*time.Millisecond)
			defer cancel()
			req, err := http.NewRequestWithContext(ctx, http.MethodGet, ts.URL+"/v1/events?since_seconds=60", http.NoBody)
			if err != nil {
				t.Fatal(err)
			}
			res, err := ts.Client().Do(req)
			if err != nil {
				t.Fatal(err)
			}
			defer func() { _ = res.Body.Close() }()
			raw, _ := io.ReadAll(res.Body) // ends when the context does (dozens of polls)
			if diff := gocmp.Diff(1, strings.Count(string(raw), "event: ledger")); diff != "" {
				t.Errorf("frames (-want +got):\n%s\n%s", diff, raw)
			}
		})
	}
}

func TestServe(t *testing.T) {
	busy, err := (&net.ListenConfig{}).Listen(t.Context(), "tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = busy.Close() })
	tests := []struct {
		name    string
		addr    string
		want    error // compared with errors.Is; nil with wantAny=false means success
		wantAny bool  // any error
		wantLog string
	}{
		{name: "a public address is refused", addr: "0.0.0.0:7457", want: ErrNotLoopback},
		{name: "a host name other than localhost is refused", addr: "example.com:7457", want: ErrNotLoopback},
		{name: "an address without a port is refused", addr: "127.0.0.1", wantAny: true},
		{name: "a busy port is an error", addr: busy.Addr().String(), wantAny: true, wantLog: "pmr serve: http://" + busy.Addr().String() + " (OpenAPI: openapi/openapi.yaml)\n"},
		{name: "loopback serves until cancelled", addr: "127.0.0.1:0", wantLog: "pmr serve: http://127.0.0.1:0 (OpenAPI: openapi/openapi.yaml)\n"},
		{name: "localhost serves until cancelled", addr: "localhost:0", wantLog: "pmr serve: http://localhost:0 (OpenAPI: openapi/openapi.yaml)\n"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d := testDeps(t)
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			var log bytes.Buffer // read only after Serve returned
			done := make(chan error, 1)
			go func() { done <- Serve(ctx, tt.addr, d, &log) }()
			var err error
			select {
			case err = <-done:
			case <-time.After(200 * time.Millisecond):
				cancel() // still serving: stop it
				err = <-done
			}
			switch {
			case tt.wantAny:
				if err == nil {
					t.Error("Serve() = nil, want an error")
				}
			case !errors.Is(err, tt.want):
				t.Errorf("Serve() = %v, want %v", err, tt.want)
			}
			if diff := gocmp.Diff(tt.wantLog, log.String()); diff != "" {
				t.Errorf("log (-want +got):\n%s", diff)
			}
		})
	}
}

// TestSpecFailure covers a contract that fails to load (a build defect in
// practice: the spec is embedded).
func TestSpecFailure(t *testing.T) {
	errSpec := errors.New("spec is corrupt")
	tests := []struct {
		name string
		run  func(d Deps) error
	}{
		{name: "Handler", run: func(d Deps) error { _, err := Handler(d); return err }},
		{name: "Serve", run: func(d Deps) error { return Serve(t.Context(), "127.0.0.1:0", d, io.Discard) }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			old := loadSpec
			loadSpec = func() (*openapi3.T, error) { return nil, errSpec }
			t.Cleanup(func() { loadSpec = old })
			if err := tt.run(testDeps(t)); !errors.Is(err, errSpec) {
				t.Errorf("err = %v, want %v", err, errSpec)
			}
		})
	}
}

// TestServeStopsStreams: Serve used to return as soon as the listener closed,
// while Shutdown ran in the background and an open event stream held the
// connection until the shutdown deadline. Now requests run under ctx, and
// Serve returns after the shutdown has finished.
func TestServeStopsStreams(t *testing.T) {
	tests := []struct {
		name   string
		stream bool // a client holds /v1/events open when Serve is cancelled
	}{
		{name: "an open event stream ends before Serve returns", stream: true},
		{name: "without a client Serve returns at once"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			l, err := (&net.ListenConfig{}).Listen(t.Context(), "tcp", "127.0.0.1:0")
			if err != nil {
				t.Fatal(err)
			}
			addr := l.Addr().String()
			_ = l.Close()
			d := testDeps(t)
			d.Serve.SSEPollMS, d.Serve.ShutdownTimeoutMS = 5, 5000
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			done := make(chan error, 1)
			go func() { done <- Serve(ctx, addr, d, io.Discard) }()
			path := "/v1/report"
			if tt.stream {
				path = "/v1/events"
			}
			var res *http.Response
			for range 100 { // wait for the listener
				req, _ := http.NewRequestWithContext(t.Context(), http.MethodGet, "http://"+addr+path, http.NoBody)
				if res, err = http.DefaultClient.Do(req); err == nil {
					break
				}
				time.Sleep(10 * time.Millisecond)
			}
			if err != nil {
				t.Fatal(err)
			}
			defer func() { _ = res.Body.Close() }()
			if !tt.stream {
				_, _ = io.Copy(io.Discard, res.Body)
			}
			start := time.Now()
			cancel()
			if err := <-done; err != nil {
				t.Fatalf("Serve() = %v", err)
			}
			closed := make(chan struct{})
			go func() { _, _ = io.Copy(io.Discard, res.Body); close(closed) }()
			select {
			case <-closed:
			case <-time.After(time.Second):
				t.Error("the event stream is still open after Serve returned")
			}
			if elapsed := time.Since(start); elapsed > 2*time.Second {
				t.Errorf("shutdown took %v: the stream held it", elapsed)
			}
		})
	}
}
