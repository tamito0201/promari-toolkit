package learn

import (
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"

	"promari-model-router/internal/domain/service"
)

// neighborFloor is reached through Train only with at least as many labelled
// examples as classes, so its small-data guard is tested directly.
func TestNeighborFloor(t *testing.T) {
	vec := func(idx ...int32) prepared {
		val := make([]float32, len(idx))
		for i := range val {
			val[i] = 1
		}
		return prepared{vec: service.Vector{Idx: idx, Val: val}}
	}
	tests := []struct {
		name     string
		data     []prepared
		quantile float64
		want     float64
	}{
		{"no examples", nil, 0.05, 0},
		{"a single example has no neighbour", []prepared{vec(0)}, 0.05, 0},
		{"identical pair", []prepared{vec(0), vec(0)}, 0.05, 1},
		{"orthogonal examples share nothing", []prepared{vec(0), vec(1)}, 0.5, 0},
		{"the quantile picks from the sorted nearest similarities", []prepared{vec(0), vec(0), vec(1)}, 0, 0},
		{"a quantile of one is clamped to the largest", []prepared{vec(0), vec(0), vec(1)}, 1, 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if diff := cmp.Diff(tt.want, neighborFloor(tt.data, tt.quantile), cmpopts.EquateApprox(0, 1e-6)); diff != "" {
				t.Error(diff)
			}
		})
	}
}
