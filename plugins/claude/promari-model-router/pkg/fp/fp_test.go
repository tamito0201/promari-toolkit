package fp_test

import (
	"iter"
	"slices"
	"testing"

	"github.com/google/go-cmp/cmp"

	"promari-model-router/pkg/fp"
)

// collect drains seq, stopping after stopAfter elements when it is positive,
// so the early-return path of every lazy transformation is exercised.
func collect[T any](seq iter.Seq[T], stopAfter int) []T {
	out := []T{}
	for v := range seq {
		out = append(out, v)
		if stopAfter > 0 && len(out) == stopAfter {
			break
		}
	}
	return out
}

func isEven(n int) bool { return n%2 == 0 }
func square(n int) int  { return n * n }

func TestLazySequences(t *testing.T) {
	tests := []struct {
		name      string
		in        []int
		apply     func(iter.Seq[int]) iter.Seq[int]
		stopAfter int
		want      []int
	}{
		{
			name:  "filter then map (pipeline)",
			in:    []int{1, 2, 3, 4, 5, 6},
			apply: func(s iter.Seq[int]) iter.Seq[int] { return fp.Map(fp.Filter(s, isEven), square) },
			want:  []int{4, 16, 36},
		},
		{
			name:  "map over an empty sequence",
			in:    []int{},
			apply: func(s iter.Seq[int]) iter.Seq[int] { return fp.Map(s, square) },
			want:  []int{},
		},
		{
			name:      "map stops when the consumer stops",
			in:        []int{1, 2, 3},
			apply:     func(s iter.Seq[int]) iter.Seq[int] { return fp.Map(s, square) },
			stopAfter: 1,
			want:      []int{1},
		},
		{
			name:  "filter keeps nothing",
			in:    []int{1, 3, 5},
			apply: func(s iter.Seq[int]) iter.Seq[int] { return fp.Filter(s, isEven) },
			want:  []int{},
		},
		{
			name:      "filter stops when the consumer stops",
			in:        []int{1, 2, 3, 4},
			apply:     func(s iter.Seq[int]) iter.Seq[int] { return fp.Filter(s, isEven) },
			stopAfter: 1,
			want:      []int{2},
		},
		{
			name:  "take fewer than available",
			in:    []int{1, 2, 3, 4, 5, 6},
			apply: func(s iter.Seq[int]) iter.Seq[int] { return fp.Take(s, 2) },
			want:  []int{1, 2},
		},
		{
			name:  "take more than available",
			in:    []int{1, 2},
			apply: func(s iter.Seq[int]) iter.Seq[int] { return fp.Take(s, 5) },
			want:  []int{1, 2},
		},
		{
			name:  "take zero",
			in:    []int{1, 2},
			apply: func(s iter.Seq[int]) iter.Seq[int] { return fp.Take(s, 0) },
			want:  []int{},
		},
		{
			name:  "take negative",
			in:    []int{1, 2},
			apply: func(s iter.Seq[int]) iter.Seq[int] { return fp.Take(s, -1) },
			want:  []int{},
		},
		{
			name:      "take stops when the consumer stops",
			in:        []int{1, 2, 3},
			apply:     func(s iter.Seq[int]) iter.Seq[int] { return fp.Take(s, 3) },
			stopAfter: 1,
			want:      []int{1},
		},
		{
			name:  "take stops the upstream map once full",
			in:    []int{1, 2, 3},
			apply: func(s iter.Seq[int]) iter.Seq[int] { return fp.Take(fp.Map(s, square), 2) },
			want:  []int{1, 4},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := collect(tt.apply(slices.Values(tt.in)), tt.stopAfter)
			if diff := cmp.Diff(tt.want, got); diff != "" {
				t.Errorf("mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

func TestFolds(t *testing.T) {
	tests := []struct {
		name string
		in   []int
		fold func(iter.Seq[int]) int
		want int
	}{
		{"reduce sums", []int{1, 2, 3}, func(s iter.Seq[int]) int { return fp.Reduce(s, 0, func(a, v int) int { return a + v }) }, 6},
		{"reduce empty returns init", nil, func(s iter.Seq[int]) int { return fp.Reduce(s, 42, func(a, v int) int { return a + v }) }, 42},
		{"count evens", []int{1, 2, 3, 4, 5, 6}, func(s iter.Seq[int]) int { return fp.Count(s, isEven) }, 3},
		{"count none", []int{1, 3}, func(s iter.Seq[int]) int { return fp.Count(s, isEven) }, 0},
		{"sum by identity", []int{1, 2, 3, 4, 5, 6}, func(s iter.Seq[int]) int { return fp.SumBy(s, func(n int) int { return n }) }, 21},
		{"sum by square", []int{1, 2, 3}, func(s iter.Seq[int]) int { return fp.SumBy(s, square) }, 14},
		{"sum by empty is zero", nil, func(s iter.Seq[int]) int { return fp.SumBy(s, square) }, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if diff := cmp.Diff(tt.want, tt.fold(slices.Values(tt.in))); diff != "" {
				t.Errorf("mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

func TestGrouping(t *testing.T) {
	words := []string{"ant", "bee", "apple", "bat", "cow"}
	first := func(s string) byte { return s[0] }
	tests := []struct {
		name string
		in   []string
		got  func(iter.Seq[string]) any
		want any
	}{
		{
			name: "count by first letter",
			in:   words,
			got:  func(s iter.Seq[string]) any { return fp.CountBy(s, first) },
			want: map[byte]int{'a': 2, 'b': 2, 'c': 1},
		},
		{
			name: "count by on empty",
			in:   nil,
			got:  func(s iter.Seq[string]) any { return fp.CountBy(s, first) },
			want: map[byte]int{},
		},
		{
			name: "group by keeps order within a group",
			in:   words,
			got:  func(s iter.Seq[string]) any { return fp.GroupBy(s, first) },
			want: map[byte][]string{'a': {"ant", "apple"}, 'b': {"bee", "bat"}, 'c': {"cow"}},
		},
		{
			name: "index by lets the later element win",
			in:   words,
			got:  func(s iter.Seq[string]) any { return fp.IndexBy(s, first) },
			want: map[byte]string{'a': "apple", 'b': "bat", 'c': "cow"},
		},
		{
			name: "sorted keys of a count",
			in:   []string{"cow", "ant", "bee"},
			got:  func(s iter.Seq[string]) any { return fp.SortedKeys(fp.CountBy(s, first)) },
			want: []byte{'a', 'b', 'c'},
		},
		{
			name: "sorted keys of an empty map",
			in:   nil,
			got:  func(s iter.Seq[string]) any { return fp.SortedKeys(fp.CountBy(s, first)) },
			want: []byte(nil),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if diff := cmp.Diff(tt.want, tt.got(slices.Values(tt.in))); diff != "" {
				t.Errorf("mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

func TestOption(t *testing.T) {
	double := func(n int) int { return n * 2 }
	tests := []struct {
		name       string
		opt        fp.Option[int]
		wantValue  int
		wantOK     bool
		wantOrElse int
	}{
		{"some", fp.Some(7), 7, true, 7},
		{"some zero is still present", fp.Some(0), 0, true, 0},
		{"none", fp.None[int](), 0, false, -1},
		{"from a present pair", fp.OptionFrom(3, true), 3, true, 3},
		{"from an absent pair", fp.OptionFrom(3, false), 3, false, -1},
		{"map a present value", fp.MapOption(fp.Some(4), double), 8, true, 8},
		{"map an absent value", fp.MapOption(fp.None[int](), double), 0, false, -1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			v, ok := tt.opt.Get()
			type result struct {
				Value, OrElse int
				OK, IsSome    bool
			}
			got := result{Value: v, OK: ok, IsSome: tt.opt.IsSome(), OrElse: tt.opt.OrElse(-1)}
			want := result{Value: tt.wantValue, OK: tt.wantOK, IsSome: tt.wantOK, OrElse: tt.wantOrElse}
			if diff := cmp.Diff(want, got); diff != "" {
				t.Errorf("mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

func TestFirstSome(t *testing.T) {
	type result struct {
		Value int
		OK    bool
		Calls int
	}
	tests := []struct {
		name string
		opts []fp.Option[int]
		want result
	}{
		{"no thunks", nil, result{}},
		{"all absent evaluates every thunk", []fp.Option[int]{fp.None[int](), fp.None[int]()}, result{Calls: 2}},
		{"stops at the first present", []fp.Option[int]{fp.None[int](), fp.Some(7), fp.Some(9)}, result{Value: 7, OK: true, Calls: 2}},
		{"first thunk present", []fp.Option[int]{fp.Some(1), fp.Some(2)}, result{Value: 1, OK: true, Calls: 1}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			calls := 0
			thunks := make([]func() fp.Option[int], 0, len(tt.opts))
			for _, o := range tt.opts {
				thunks = append(thunks, func() fp.Option[int] { calls++; return o })
			}
			v, ok := fp.FirstSome(thunks...).Get()
			if diff := cmp.Diff(tt.want, result{Value: v, OK: ok, Calls: calls}); diff != "" {
				t.Errorf("mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

func TestApply(t *testing.T) {
	positive := func(n int) fp.Option[int] { return fp.OptionFrom(n, n > 0) }
	even := func(n int) fp.Option[int] { return fp.OptionFrom(n*10, n%2 == 0) }
	tests := []struct {
		name      string
		fs        []func(int) fp.Option[int]
		arg       int
		wantLen   int
		wantValue int
		wantOK    bool
	}{
		{"no functions", nil, 1, 0, 0, false},
		{"the first function answers", []func(int) fp.Option[int]{positive, even}, 3, 2, 3, true},
		{"falls through to the second", []func(int) fp.Option[int]{positive, even}, -2, 2, -20, true},
		{"nobody answers", []func(int) fp.Option[int]{positive, even}, -3, 2, 0, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			thunks := fp.Apply(tt.fs, tt.arg)
			v, ok := fp.FirstSome(thunks...).Get()
			type result struct {
				Len, Value int
				OK         bool
			}
			want := result{Len: tt.wantLen, Value: tt.wantValue, OK: tt.wantOK}
			if diff := cmp.Diff(want, result{Len: len(thunks), Value: v, OK: ok}); diff != "" {
				t.Errorf("mismatch (-want +got):\n%s", diff)
			}
		})
	}
}
