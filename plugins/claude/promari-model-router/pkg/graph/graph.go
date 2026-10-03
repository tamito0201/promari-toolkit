// Package graph is a tiny state-graph runner in the spirit of LangGraph: a
// workflow is a set of named nodes, each a pure function from state to state,
// joined by edges that may be conditional. Running a graph yields every step,
// so callers can record the full trace of a decision.
package graph

import (
	"errors"
	"fmt"
	"iter"
	"maps"
	"slices"
)

// End is the name of the terminal node.
const End = "__end__"

// Node transforms the state.
type Node[S any] func(S) S

// Router picks the next node from the state (a conditional edge).
type Router[S any] func(S) string

// Graph is an immutable workflow definition. Build it with New and the With*
// methods, which return a new graph each time.
type Graph[S any] struct {
	entry   string
	nodes   map[string]Node[S]
	edges   map[string]Router[S]
	targets map[string][]string // the nodes each edge may lead to
	limit   int
}

// Len is the number of nodes. A graph without cycles visits each node at most
// once, so a step limit below Len can stop a run that is not looping.
func (g Graph[S]) Len() int { return len(g.nodes) }

// Step is one executed node and the state it produced.
type Step[S any] struct {
	Node  string
	State S
}

// ErrNoEdge is returned when a node has no outgoing edge.
var ErrNoEdge = errors.New("graph: node has no outgoing edge")

// ErrUnknownTarget is returned when an edge leads (or may lead) to a node
// that does not exist.
var ErrUnknownTarget = errors.New("graph: edge leads to an unknown node")

// ErrUndeclaredTarget is returned when a conditional edge picks a node it
// did not declare.
var ErrUndeclaredTarget = errors.New("graph: conditional edge picked an undeclared node")

// ErrStepLimit is returned when the run exceeds the step limit (a cycle guard).
var ErrStepLimit = errors.New("graph: step limit exceeded")

// DefaultStepLimit is the cycle guard of a graph built without WithStepLimit.
const DefaultStepLimit = 64

// New starts a graph whose run begins at entry.
func New[S any](entry string) Graph[S] {
	return Graph[S]{entry: entry, nodes: map[string]Node[S]{}, edges: map[string]Router[S]{}, targets: map[string][]string{}, limit: DefaultStepLimit}
}

func (g Graph[S]) clone() Graph[S] {
	return Graph[S]{entry: g.entry, nodes: maps.Clone(g.nodes), edges: maps.Clone(g.edges), targets: maps.Clone(g.targets), limit: g.limit}
}

// WithNode adds a node.
func (g Graph[S]) WithNode(name string, n Node[S]) Graph[S] {
	out := g.clone()
	out.nodes[name] = n
	return out
}

// WithEdge adds an unconditional edge.
func (g Graph[S]) WithEdge(from, to string) Graph[S] {
	return g.WithConditionalEdge(from, func(S) string { return to }, to)
}

// WithConditionalEdge adds an edge whose target is chosen from the state
// among the declared targets (End included when the edge may stop the run).
// Declaring them lets Validate check the graph before it runs.
func (g Graph[S]) WithConditionalEdge(from string, r Router[S], targets ...string) Graph[S] {
	out := g.clone()
	out.edges[from] = r
	out.targets[from] = slices.Clone(targets)
	return out
}

// WithStepLimit sets the maximum number of steps before a run is aborted.
func (g Graph[S]) WithStepLimit(n int) Graph[S] {
	out := g.clone()
	out.limit = n
	return out
}

// Validate checks that the entry exists, that every node has an outgoing
// edge, and that every edge declares its targets and they all exist (End
// counts as a node). Errors name the first problem in sorted order.
func (g Graph[S]) Validate() error {
	if _, ok := g.nodes[g.entry]; !ok {
		return fmt.Errorf("graph: unknown entry %q", g.entry)
	}
	for _, name := range slices.Sorted(maps.Keys(g.nodes)) {
		if _, ok := g.edges[name]; !ok {
			return fmt.Errorf("%w: %q", ErrNoEdge, name)
		}
	}
	for _, from := range slices.Sorted(maps.Keys(g.edges)) {
		if len(g.targets[from]) == 0 {
			return fmt.Errorf("%w: the edge from %q declares no target", ErrUnknownTarget, from)
		}
		for _, to := range g.targets[from] {
			if _, ok := g.nodes[to]; !ok && to != End {
				return fmt.Errorf("%w: %q -> %q", ErrUnknownTarget, from, to)
			}
		}
	}
	return nil
}

// Run yields each executed step. On failure it yields a zero step and the error.
func (g Graph[S]) Run(init S) iter.Seq2[Step[S], error] {
	return func(yield func(Step[S], error) bool) {
		state, current := init, g.entry
		for range g.limit {
			node, ok := g.nodes[current]
			if !ok {
				yield(Step[S]{}, fmt.Errorf("graph: unknown node %q", current))
				return
			}
			state = node(state)
			if !yield(Step[S]{Node: current, State: state}, nil) {
				return
			}
			edge, ok := g.edges[current]
			if !ok {
				yield(Step[S]{}, fmt.Errorf("%w: %q", ErrNoEdge, current))
				return
			}
			next := edge(state)
			if !slices.Contains(g.targets[current], next) {
				yield(Step[S]{}, fmt.Errorf("%w: %q -> %q", ErrUndeclaredTarget, current, next))
				return
			}
			if current = next; current == End {
				return
			}
		}
		yield(Step[S]{}, ErrStepLimit)
	}
}

// Invoke runs the graph to the end and returns the final state and the path.
func (g Graph[S]) Invoke(init S) (S, []string, error) {
	state, path := init, []string{}
	for step, err := range g.Run(init) {
		if err != nil {
			return state, path, err
		}
		state = step.State
		path = append(path, step.Node)
	}
	return state, path, nil
}
