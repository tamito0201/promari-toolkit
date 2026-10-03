package clock_test

import (
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"

	"promari-model-router/internal/domain/repository"
	"promari-model-router/internal/infrastructure/clock"
)

func TestNow(t *testing.T) {
	at := time.Date(2026, 9, 30, 12, 0, 0, 0, time.UTC)
	tests := []struct {
		name  string
		clock repository.Clock
		check func(t *testing.T, before, got, after time.Time)
	}{
		{
			name: "fixed returns the same instant", clock: clock.Fixed{At: at},
			check: func(t *testing.T, _, got, _ time.Time) {
				t.Helper()
				if diff := cmp.Diff(at, got); diff != "" {
					t.Errorf("Now() mismatch (-want +got):\n%s", diff)
				}
			},
		},
		{
			name: "system returns the wall clock", clock: clock.System{},
			check: func(t *testing.T, before, got, after time.Time) {
				t.Helper()
				if got.Before(before) || got.After(after) {
					t.Errorf("Now() = %v, want within [%v, %v]", got, before, after)
				}
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			before := time.Now()
			got := tt.clock.Now()
			tt.check(t, before, got, time.Now())
		})
	}
}
