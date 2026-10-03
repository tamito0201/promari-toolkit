package mcpserver_test

import (
	"context"
	"encoding/json"
	"errors"
	"iter"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	"github.com/modelcontextprotocol/go-sdk/mcp"

	"promari-model-router/internal/application/usecase"
	"promari-model-router/internal/di"
	"promari-model-router/internal/domain/model"
	"promari-model-router/internal/domain/repository"
	"promari-model-router/internal/domain/service"
	"promari-model-router/internal/infrastructure/clock"
	"promari-model-router/internal/interfaces/mcpserver"
)

// brokenLedger fails every read.
type brokenLedger struct{ repository.LedgerRepository }

func (brokenLedger) Since(context.Context, time.Time) iter.Seq2[model.Entry, error] {
	return func(yield func(model.Entry, error) bool) { yield(model.Entry{}, errors.New("ledger unreadable")) }
}

// recordingLedger yields fixed entries and records the requested lower bound.
type recordingLedger struct {
	repository.LedgerRepository
	from *time.Time
}

func (l recordingLedger) Since(_ context.Context, from time.Time) iter.Seq2[model.Entry, error] {
	*l.from = from
	return func(func(model.Entry, error) bool) {}
}

func deps(t *testing.T) mcpserver.Deps {
	t.Helper()
	tmp := t.TempDir()
	for k, v := range map[string]string{
		"HOME": filepath.Join(tmp, "home"), "CLAUDE_PLUGIN_DATA": filepath.Join(tmp, "data"), "CLAUDE_PROJECT_DIR": filepath.Join(tmp, "project"),
	} {
		t.Setenv(k, v)
	}
	c := di.New()
	t.Cleanup(func() { _ = c.Close() })
	return mcpserver.Deps{
		Report: built(c.Report())(t), Explain: built(c.Explain())(t),
		Policy: built(c.Policy())(t), Version: "test",
	}
}

// roundTrip marshals v and decodes it as generic JSON, the shape a client sees.
func roundTrip(t *testing.T, v any) any {
	t.Helper()
	raw, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	var out any
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatal(err)
	}
	return out
}

func TestTools(t *testing.T) {
	now := time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC)
	cwd, _ := os.Getwd()
	tests := []struct {
		name     string
		edit     func(d *mcpserver.Deps, from *time.Time)
		tool     string
		args     any
		want     func(d mcpserver.Deps) any // structured content
		wantErr  bool                       // the tool reports IsError
		wantFrom func(d mcpserver.Deps) time.Time
	}{
		{
			name: "explain_route fills the defaults",
			tool: "explain_route", args: map[string]any{"prompt": "この関数の単体テストを書いて"},
			want: func(d mcpserver.Deps) any {
				return d.Explain.Execute(usecase.ExplainInput{Prompt: "この関数の単体テストを書いて", SubagentType: "general-purpose", Cwd: cwd})
			},
		},
		{
			name: "explain_route honours the subagent type and the session model",
			tool: "explain_route", args: map[string]any{"prompt": "探して", "subagent_type": "Explore", "session_model": "claude-sonnet-4-6"},
			want: func(d mcpserver.Deps) any {
				return d.Explain.Execute(usecase.ExplainInput{Prompt: "探して", SubagentType: "Explore", SessionModel: "claude-sonnet-4-6", Cwd: cwd})
			},
		},
		{
			name: "routing_report uses the default window",
			edit: func(d *mcpserver.Deps, from *time.Time) {
				d.Report.Ledger, d.Report.Clock = recordingLedger{from: from}, clock.Fixed{At: now}
			},
			tool: "routing_report", args: map[string]any{},
			want: func(d mcpserver.Deps) any { return usecase.BuildReport(nil, d.Report.Prices) },
			wantFrom: func(d mcpserver.Deps) time.Time {
				return now.Add(-time.Duration(d.Report.DefaultDays * 24 * float64(time.Hour)))
			},
		},
		{
			name: "routing_report honours days",
			edit: func(d *mcpserver.Deps, from *time.Time) {
				d.Report.Ledger, d.Report.Clock = recordingLedger{from: from}, clock.Fixed{At: now}
			},
			tool: "routing_report", args: map[string]any{"days": 0.5},
			want:     func(d mcpserver.Deps) any { return usecase.BuildReport(nil, d.Report.Prices) },
			wantFrom: func(mcpserver.Deps) time.Time { return now.Add(-12 * time.Hour) },
		},
		{
			// A negative window used to be read as "the default".
			name: "routing_report refuses a negative window",
			tool: "routing_report", args: map[string]any{"days": -1},
			wantErr: true,
		},
		{
			name: "routing_report reports a ledger failure as a tool error",
			edit: func(d *mcpserver.Deps, _ *time.Time) { d.Report.Ledger = brokenLedger{} },
			tool: "routing_report", args: map[string]any{"days": 1},
			wantErr: true,
		},
		{
			name: "route_policy returns the policy and the tags",
			tool: "route_policy", args: map[string]any{},
			want: func(mcpserver.Deps) any {
				st := di.New().Settings()
				return mcpserver.PolicyOut{Policy: service.SessionContext(st.Messages), Tags: st.Messages.RouteTags}
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d := deps(t)
			var from time.Time
			if tt.edit != nil {
				tt.edit(&d, &from)
			}
			st, ct := mcp.NewInMemoryTransports()
			ss, err := mcpserver.NewServer(d).Connect(t.Context(), st, nil)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = ss.Close() })
			cs, err := mcp.NewClient(&mcp.Implementation{Name: "test", Version: "test"}, nil).Connect(t.Context(), ct, nil)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = cs.Close() })

			res, err := cs.CallTool(t.Context(), &mcp.CallToolParams{Name: tt.tool, Arguments: tt.args})
			if err != nil {
				t.Fatal(err)
			}
			if res.IsError != tt.wantErr {
				t.Fatalf("IsError = %v, want %v (%+v)", res.IsError, tt.wantErr, res.Content)
			}
			if tt.want != nil {
				if diff := cmp.Diff(roundTrip(t, tt.want(d)), roundTrip(t, res.StructuredContent)); diff != "" {
					t.Errorf("structured content (-want +got):\n%s", diff)
				}
			}
			if tt.wantFrom != nil {
				if diff := cmp.Diff(tt.wantFrom(d), from); diff != "" {
					t.Errorf("ledger window (-want +got):\n%s", diff)
				}
			}
		})
	}
}

func TestListTools(t *testing.T) {
	// schema is the part of a tool's input schema a client relies on.
	type schema struct {
		Props    []string
		Required []string
	}
	tests := []struct {
		name  string
		tool  string // "" = the list of names
		names []string
		want  schema
	}{
		{name: "three read-only tools", names: []string{"explain_route", "route_policy", "routing_report"}},
		{
			name: "explain_route needs only the prompt", tool: "explain_route",
			want: schema{Props: []string{"prompt", "session_model", "subagent_type"}, Required: []string{"prompt"}},
		},
		{name: "routing_report takes an optional window", tool: "routing_report", want: schema{Props: []string{"days"}}},
		{name: "route_policy takes nothing", tool: "route_policy", want: schema{Props: []string{}}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			st, ct := mcp.NewInMemoryTransports()
			ss, err := mcpserver.NewServer(deps(t)).Connect(t.Context(), st, nil)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = ss.Close() })
			cs, err := mcp.NewClient(&mcp.Implementation{Name: "test", Version: "test"}, nil).Connect(t.Context(), ct, nil)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = cs.Close() })
			res, err := cs.ListTools(t.Context(), nil)
			if err != nil {
				t.Fatal(err)
			}
			got := make([]string, 0, len(res.Tools))
			for _, tool := range res.Tools {
				got = append(got, tool.Name)
				if tool.Name != tt.tool {
					continue
				}
				raw, err := json.Marshal(tool.InputSchema)
				if err != nil {
					t.Fatal(err)
				}
				var s struct {
					Properties map[string]any `json:"properties"`
					Required   []string       `json:"required"`
				}
				if err := json.Unmarshal(raw, &s); err != nil {
					t.Fatal(err)
				}
				gotSchema := schema{Props: slices.Sorted(maps.Keys(s.Properties)), Required: s.Required}
				if gotSchema.Props == nil {
					gotSchema.Props = []string{}
				}
				if diff := cmp.Diff(tt.want, gotSchema); diff != "" {
					t.Errorf("input schema of %s (-want +got):\n%s", tt.tool, diff)
				}
			}
			if tt.tool == "" {
				if diff := cmp.Diff(tt.names, got); diff != "" {
					t.Errorf("tools (-want +got):\n%s", diff)
				}
			} else if !slices.Contains(got, tt.tool) {
				t.Errorf("tool %s is not listed: %v", tt.tool, got)
			}
		})
	}
}

// TestRun drives the stdio entry point: a closed stdin ends the session.
func TestRun(t *testing.T) {
	tests := []struct {
		name   string
		cancel bool
	}{
		{name: "stdin EOF ends the server"},
		{name: "cancellation ends the server", cancel: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d := deps(t)
			r, w, err := os.Pipe()
			if err != nil {
				t.Fatal(err)
			}
			devnull, err := os.OpenFile(os.DevNull, os.O_WRONLY, 0)
			if err != nil {
				t.Fatal(err)
			}
			oldIn, oldOut := os.Stdin, os.Stdout
			os.Stdin, os.Stdout = r, devnull
			t.Cleanup(func() {
				os.Stdin, os.Stdout = oldIn, oldOut
				_ = r.Close()
				_ = devnull.Close()
			})
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			if tt.cancel {
				cancel()
			} else {
				_ = w.Close()
			}
			done := make(chan error, 1)
			go func() { done <- mcpserver.Run(ctx, d) }()
			select {
			case err := <-done:
				if tt.cancel && !errors.Is(err, context.Canceled) {
					t.Errorf("Run() = %v, want context.Canceled", err)
				}
			case <-time.After(10 * time.Second):
				t.Fatal("Run did not return")
			}
			_ = w.Close()
		})
	}
}

// built returns what a scope built, failing the test when it could not:
// built(scope.Report())(t).
func built[T any](v T, err error) func(t *testing.T) T {
	return func(t *testing.T) T {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
		return v
	}
}
