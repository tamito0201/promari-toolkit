package usecase_test

import (
	"errors"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"

	"promari-model-router/internal/application/usecase"
	"promari-model-router/internal/domain/model"
	"promari-model-router/internal/domain/service"
	"promari-model-router/internal/infrastructure/clock"
)

func TestVerifyUseCase(t *testing.T) {
	tests := []struct {
		name    string
		ledger  memLedger
		want    usecase.Verification
		wantErr error
	}{
		{name: "intact", ledger: memLedger{checked: 3}, want: usecase.Verification{Checked: 3}},
		{name: "broken", ledger: memLedger{checked: 1, broken: 2}, want: usecase.Verification{Checked: 1, Broken: 2}},
		{name: "unreadable", ledger: memLedger{verifyErr: errLedger}, wantErr: errLedger},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := usecase.VerifyUseCase{Ledger: &tt.ledger}.Execute(t.Context())
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("err = %v, want %v", err, tt.wantErr)
			}
			if diff := cmp.Diff(tt.want, got); diff != "" {
				t.Errorf("(-want +got):\n%s", diff)
			}
		})
	}
}

func TestCostUseCase(t *testing.T) {
	tests := []struct {
		name  string
		shape service.SubagentShape
	}{
		{name: "the default shape", shape: service.SubagentShape{ToolCalls: 10, Prompt: 8000, ToolResult: 1500, OutputPerCall: 300, Report: 800}},
		{name: "no tool calls", shape: service.SubagentShape{Prompt: 100, Report: 10}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := newFixture(t)
			prices := f.config.Prices()
			got := usecase.CostUseCase{Prices: f.config}.Execute(tt.shape)
			want := usecase.CostEstimate{PricesAsOf: prices.AsOf, Usage: tt.shape.Usage(), USDByTier: prices.Compare(tt.shape)}
			if diff := cmp.Diff(want, got); diff != "" {
				t.Errorf("(-want +got):\n%s", diff)
			}
		})
	}
}

// TestFeedUseCase: the feed follows positions, so every entry is sent once,
// whatever its timestamp says.
// errClientGone is the error an SSE client's write returns once it left.
var errClientGone = errors.New("client gone")

func TestFeedUseCase(t *testing.T) {
	now := testNow
	e := func(at time.Time, reason string) model.Entry {
		return model.Entry{At: at, Event: model.EventPrompt, Reason: reason}
	}
	tests := []struct {
		name     string
		existing []model.Entry
		back     time.Duration
		later    []model.Entry // appended after the first Next
		headErr  error
		sinceErr error
		emitErr  error
		want     [][]string // reasons per Next call (three calls)
		wantErr  error      // matched with errors.Is
	}{
		{
			name: "without a backlog only new entries", existing: []model.Entry{e(now, "old")}, later: []model.Entry{e(now, "new")},
			want: [][]string{nil, {"new"}, nil},
		},
		{
			name:     "the backlog is the window, then only new entries, even with an older timestamp",
			existing: []model.Entry{e(now.Add(-2*time.Hour), "too old"), e(now.Add(-time.Minute), "recent")},
			back:     time.Hour, later: []model.Entry{e(now.Add(-3*time.Hour), "late clock")},
			want: [][]string{{"recent"}, {"late clock"}, nil},
		},
		{name: "nothing new twice", existing: []model.Entry{e(now, "a")}, back: time.Hour, want: [][]string{{"a"}, nil, nil}},
		{name: "a head error ends the start", headErr: errLedger, wantErr: errLedger},
		{name: "a read error ends the feed", existing: []model.Entry{e(now, "a")}, back: time.Hour, sinceErr: errLedger, wantErr: errLedger},
		{name: "an emit error ends the feed", existing: []model.Entry{e(now, "a")}, back: time.Hour, emitErr: errClientGone, wantErr: errClientGone},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			l := &memLedger{entries: tt.existing, headErr: tt.headErr, sinceErr: tt.sinceErr}
			u := usecase.FeedUseCase{Ledger: l, Clock: clock.Fixed{At: now}}
			cur, err := u.Start(t.Context(), tt.back)
			var got [][]string
			for call := range 3 {
				if err != nil {
					break
				}
				var reasons []string
				cur, err = u.Next(t.Context(), cur, func(e model.Entry) error {
					reasons = append(reasons, e.Reason)
					return tt.emitErr
				})
				got = append(got, reasons)
				if call == 0 {
					l.entries = append(l.entries, tt.later...)
				}
			}
			if (err != nil) != (tt.wantErr != nil) || !errors.Is(err, tt.wantErr) {
				t.Fatalf("err = %v, want %v", err, tt.wantErr)
			}
			if tt.wantErr != nil {
				return
			}
			if diff := cmp.Diff(tt.want, got); diff != "" {
				t.Errorf("reasons per call (-want +got):\n%s", diff)
			}
		})
	}
}
