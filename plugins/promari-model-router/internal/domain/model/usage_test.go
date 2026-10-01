package model_test

import (
	"testing"

	"github.com/google/go-cmp/cmp"

	"github.com/tamito0201/promari-toolkit/plugins/promari-model-router/internal/domain/model"
	"github.com/tamito0201/promari-toolkit/plugins/promari-model-router/pkg/fp"
)

func TestNewPressure(t *testing.T) {
	cfg := model.PressureSettings{FiveHourHigh: 80, SevenDayHigh: 90, CodexBlockPercent: 95}
	type view struct {
		Known, High, CodexAvailable bool
	}
	tests := []struct {
		name              string
		five, seven, used fp.Option[float64]
		zero              bool // every threshold 0
		want              view
	}{
		{name: "nothing known", want: view{Known: true, CodexAvailable: true}},
		{name: "below both", five: fp.Some(79.9), seven: fp.Some(89.9), used: fp.Some(94.9), want: view{Known: true, CodexAvailable: true}},
		{name: "five hours at the threshold", five: fp.Some(80.0), want: view{Known: true, High: true, CodexAvailable: true}},
		{name: "seven days at the threshold", seven: fp.Some(90.0), want: view{Known: true, High: true, CodexAvailable: true}},
		{name: "Codex at the block threshold", used: fp.Some(95.0), want: view{Known: true}},
		{name: "an unknown window is not high even at a zero threshold", zero: true, want: view{Known: true, CodexAvailable: true}},
		{name: "a known window at a zero threshold is high", five: fp.Some(0.0), zero: true, want: view{Known: true, High: true, CodexAvailable: true}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := cfg
			if tt.zero {
				c = model.PressureSettings{}
			}
			p, q := model.NewPressure(tt.five, tt.seven, c), model.NewCodexQuota(tt.used, c)
			if diff := cmp.Diff(tt.want, view{p.Known, p.High, q.Available}); diff != "" {
				t.Errorf("(-want +got):\n%s", diff)
			}
		})
	}
}
