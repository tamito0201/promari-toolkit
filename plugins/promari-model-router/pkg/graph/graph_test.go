package graph_test

import (
	"errors"
	"testing"

	"github.com/google/go-cmp/cmp"

	"promari-model-router/pkg/graph"
)

func inc(n int) int     { return n + 1 }
func same(n int) int    { return n }
func times10(n int) int { return n * 10 }

// branching loops a -> b -> a until the state exceeds 1, then goes to c.
func branching() graph.Graph[int] {
	return graph.New[int]("a").
		WithNode("a", inc).WithNode("b", inc).WithNode("c", times10).
		WithConditionalEdge("a", func(n int) string {
			if n > 1 {
				return "c"
			}
			return "b"
		}, "b", "c").
		WithEdge("b", "a").
		WithEdge("c", graph.End)
}

// selfLoop never reaches End.
func selfLoop() graph.Graph[int] {
	return graph.New[int]("a").WithNode("a", inc).WithEdge("a", "a")
}

// errorText renders an error for comparison ("" for nil).
func errorText(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

func TestInvoke(t *testing.T) {
	tests := []struct {
		name      string
		graph     graph.Graph[int]
		init      int
		wantState int
		wantPath  []string
		wantIs    error
		wantErr   string
	}{
		{
			name: "loops until the condition holds", graph: branching(), init: 0,
			wantState: 30, wantPath: []string{"a", "b", "a", "c"},
		},
		{
			name: "takes the short branch", graph: branching(), init: 5,
			wantState: 60, wantPath: []string{"a", "c"},
		},
		{
			name: "step limit aborts a cycle", graph: selfLoop().WithStepLimit(5), init: 0,
			wantState: 5, wantPath: []string{"a", "a", "a", "a", "a"},
			wantIs: graph.ErrStepLimit, wantErr: graph.ErrStepLimit.Error(),
		},
		{
			name: "zero step limit aborts before any step", graph: selfLoop().WithStepLimit(0), init: 7,
			wantState: 7, wantPath: []string{},
			wantIs: graph.ErrStepLimit, wantErr: graph.ErrStepLimit.Error(),
		},
		{
			name:  "default step limit guards a cycle",
			graph: selfLoop(), init: 0,
			wantState: graph.DefaultStepLimit, wantPath: repeat("a", graph.DefaultStepLimit),
			wantIs: graph.ErrStepLimit, wantErr: graph.ErrStepLimit.Error(),
		},
		{
			name:  "node without an outgoing edge",
			graph: graph.New[int]("a").WithNode("a", inc), init: 0,
			wantState: 1, wantPath: []string{"a"},
			wantIs: graph.ErrNoEdge, wantErr: `graph: node has no outgoing edge: "a"`,
		},
		{
			name:      "a conditional edge picking an undeclared node",
			graph:     graph.New[int]("a").WithNode("a", inc).WithConditionalEdge("a", func(int) string { return "a" }, graph.End),
			wantState: 1, wantPath: []string{"a"},
			wantIs: graph.ErrUndeclaredTarget, wantErr: `graph: conditional edge picked an undeclared node: "a" -> "a"`,
		},
		{
			name:  "edge to an unknown node",
			graph: graph.New[int]("a").WithNode("a", inc).WithEdge("a", "ghost"), init: 0,
			wantState: 1, wantPath: []string{"a"},
			wantErr: `graph: unknown node "ghost"`,
		},
		{
			name:  "unknown entry",
			graph: graph.New[int]("missing"), init: 3,
			wantState: 3, wantPath: []string{},
			wantErr: `graph: unknown node "missing"`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			state, path, err := tt.graph.Invoke(tt.init)
			if tt.wantIs != nil && !errors.Is(err, tt.wantIs) {
				t.Errorf("err = %v, want errors.Is %v", err, tt.wantIs)
			}
			type result struct {
				State int
				Path  []string
				Err   string
			}
			want := result{State: tt.wantState, Path: tt.wantPath, Err: tt.wantErr}
			if diff := cmp.Diff(want, result{State: state, Path: path, Err: errorText(err)}); diff != "" {
				t.Errorf("mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

func repeat(s string, n int) []string {
	out := make([]string, n)
	for i := range out {
		out[i] = s
	}
	return out
}

func TestRun(t *testing.T) {
	tests := []struct {
		name      string
		graph     graph.Graph[int]
		init      int
		stopAfter int // stop consuming after this many steps (0 = drain)
		want      []graph.Step[int]
		wantErr   string
	}{
		{
			name:  "yields every step",
			graph: branching(), init: 5,
			want: []graph.Step[int]{{Node: "a", State: 6}, {Node: "c", State: 60}},
		},
		{
			name:  "stops when the consumer stops",
			graph: branching(), init: 0, stopAfter: 2,
			want: []graph.Step[int]{{Node: "a", State: 1}, {Node: "b", State: 2}},
		},
		{
			name:  "yields the step then the missing-edge error",
			graph: graph.New[int]("a").WithNode("a", inc), init: 0,
			want:    []graph.Step[int]{{Node: "a", State: 1}},
			wantErr: `graph: node has no outgoing edge: "a"`,
		},
		{
			name:  "yields the step limit error last",
			graph: selfLoop().WithStepLimit(2), init: 0,
			want:    []graph.Step[int]{{Node: "a", State: 1}, {Node: "a", State: 2}},
			wantErr: graph.ErrStepLimit.Error(),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			steps := []graph.Step[int]{}
			var runErr error
			for step, err := range tt.graph.Run(tt.init) {
				if err != nil {
					runErr = err
					if step != (graph.Step[int]{}) {
						t.Errorf("error step = %+v, want the zero step", step)
					}
					continue
				}
				steps = append(steps, step)
				if tt.stopAfter > 0 && len(steps) == tt.stopAfter {
					break
				}
			}
			if diff := cmp.Diff(tt.want, steps); diff != "" {
				t.Errorf("steps mismatch (-want +got):\n%s", diff)
			}
			if diff := cmp.Diff(tt.wantErr, errorText(runErr)); diff != "" {
				t.Errorf("error mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

func TestValidate(t *testing.T) {
	tests := []struct {
		name    string
		graph   graph.Graph[int]
		wantIs  error
		wantErr string
	}{
		{name: "valid graph", graph: branching()},
		{name: "unknown entry", graph: graph.New[int]("a"), wantErr: `graph: unknown entry "a"`},
		{
			name:   "dangling node",
			graph:  graph.New[int]("a").WithNode("a", same),
			wantIs: graph.ErrNoEdge, wantErr: `graph: node has no outgoing edge: "a"`,
		},
		{
			name:   "dangling non-entry node",
			graph:  graph.New[int]("a").WithNode("a", same).WithEdge("a", graph.End).WithNode("b", same),
			wantIs: graph.ErrNoEdge, wantErr: `graph: node has no outgoing edge: "b"`,
		},
		// Validate used to accept an edge to a node that does not exist; the
		// run failed only when it took that edge.
		{
			name:   "an edge to an unknown node",
			graph:  graph.New[int]("a").WithNode("a", same).WithEdge("a", "ghost"),
			wantIs: graph.ErrUnknownTarget, wantErr: `graph: edge leads to an unknown node: "a" -> "ghost"`,
		},
		{
			name: "a conditional edge that may lead to an unknown node",
			graph: graph.New[int]("a").WithNode("a", same).
				WithConditionalEdge("a", func(int) string { return graph.End }, graph.End, "ghost"),
			wantIs: graph.ErrUnknownTarget, wantErr: `graph: edge leads to an unknown node: "a" -> "ghost"`,
		},
		{
			name:   "a conditional edge without declared targets",
			graph:  graph.New[int]("a").WithNode("a", same).WithConditionalEdge("a", func(int) string { return graph.End }),
			wantIs: graph.ErrUnknownTarget, wantErr: `graph: edge leads to an unknown node: the edge from "a" declares no target`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.graph.Validate()
			if tt.wantIs != nil && !errors.Is(err, tt.wantIs) {
				t.Errorf("Validate = %v, want errors.Is %v", err, tt.wantIs)
			}
			if diff := cmp.Diff(tt.wantErr, errorText(err)); diff != "" {
				t.Errorf("mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

func TestBuildersAreImmutable(t *testing.T) {
	base := graph.New[int]("a").WithNode("a", inc).WithEdge("a", graph.End)
	type result struct {
		Validate string
		State    int
		Path     []string
		Err      string
	}
	observe := func(g graph.Graph[int]) result {
		state, path, err := g.Invoke(0)
		return result{Validate: errorText(g.Validate()), State: state, Path: path, Err: errorText(err)}
	}
	tests := []struct {
		name        string
		base        graph.Graph[int]
		derive      func(graph.Graph[int]) graph.Graph[int]
		wantBase    result
		wantDerived result
	}{
		{
			name:        "WithNode",
			base:        base,
			derive:      func(g graph.Graph[int]) graph.Graph[int] { return g.WithNode("b", same) },
			wantBase:    result{State: 1, Path: []string{"a"}},
			wantDerived: result{Validate: `graph: node has no outgoing edge: "b"`, State: 1, Path: []string{"a"}},
		},
		{
			name:        "WithEdge",
			base:        base,
			derive:      func(g graph.Graph[int]) graph.Graph[int] { return g.WithEdge("a", "ghost") },
			wantBase:    result{State: 1, Path: []string{"a"}},
			wantDerived: result{Validate: `graph: edge leads to an unknown node: "a" -> "ghost"`, State: 1, Path: []string{"a"}, Err: `graph: unknown node "ghost"`},
		},
		{
			name: "WithConditionalEdge",
			base: base,
			derive: func(g graph.Graph[int]) graph.Graph[int] {
				return g.WithConditionalEdge("a", func(n int) string {
					if n < 3 {
						return "a"
					}
					return graph.End
				}, "a", graph.End)
			},
			wantBase:    result{State: 1, Path: []string{"a"}},
			wantDerived: result{State: 3, Path: []string{"a", "a", "a"}},
		},
		{
			name: "a conditional edge that picks an undeclared node stops the run",
			base: base,
			derive: func(g graph.Graph[int]) graph.Graph[int] {
				return g.WithConditionalEdge("a", func(int) string { return "a" }, graph.End)
			},
			wantBase:    result{State: 1, Path: []string{"a"}},
			wantDerived: result{State: 1, Path: []string{"a"}, Err: `graph: conditional edge picked an undeclared node: "a" -> "a"`},
		},
		{
			name:        "WithStepLimit",
			base:        selfLoop(),
			derive:      func(g graph.Graph[int]) graph.Graph[int] { return g.WithStepLimit(1) },
			wantBase:    result{State: graph.DefaultStepLimit, Path: repeat("a", graph.DefaultStepLimit), Err: graph.ErrStepLimit.Error()},
			wantDerived: result{State: 1, Path: []string{"a"}, Err: graph.ErrStepLimit.Error()},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			derived := tt.derive(tt.base)
			if diff := cmp.Diff(tt.wantDerived, observe(derived)); diff != "" {
				t.Errorf("derived mismatch (-want +got):\n%s", diff)
			}
			if diff := cmp.Diff(tt.wantBase, observe(tt.base)); diff != "" {
				t.Errorf("deriving changed the base (-want +got):\n%s", diff)
			}
		})
	}
}
