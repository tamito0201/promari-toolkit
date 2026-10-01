package service

import (
	"testing"

	"github.com/google/go-cmp/cmp"

	"github.com/tamito0201/promari-toolkit/plugins/promari-model-router/pkg/graph"
)

// The edges are tested on their own because Route cannot reach every case:
// the tag stage never stops, so a stopped state after it only exists here.
func TestRoutingEdges(t *testing.T) {
	tests := []struct {
		name  string
		route graph.Router[routeState]
		state routeState
		want  string
	}{
		{name: "after tag: a stopped state ends", route: afterTag, state: routeState{done: true, trace: RouteTrace{Source: "tag"}}, want: graph.End},
		{name: "after tag: a tag goes to the floors", route: afterTag, state: routeState{trace: RouteTrace{Source: "tag"}}, want: StageFloor},
		{name: "after tag: no tag goes to the cascade", route: afterTag, state: routeState{}, want: StageCascade},
		{name: "next: a stopped state ends", route: next(StageOOD), state: routeState{done: true}, want: graph.End},
		{name: "next: otherwise the named stage", route: next(StageOOD), state: routeState{}, want: StageOOD},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if diff := cmp.Diff(tt.want, tt.route(tt.state)); diff != "" {
				t.Errorf("edge mismatch (-want +got):\n%s", diff)
			}
		})
	}
}
