package service_test

import (
	"math"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"

	"promari-model-router/internal/domain/model"
	"promari-model-router/internal/domain/service"
)

func TestDim(t *testing.T) {
	tests := []struct {
		name string
		spec model.FeatureSpec
		want int
	}{
		{name: "hash buckets plus the dense block", spec: model.FeatureSpec{HashBuckets: 8}, want: 8 + service.DenseFeatures},
		{name: "no hash buckets", spec: model.FeatureSpec{}, want: service.DenseFeatures},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if diff := cmp.Diff(tt.want, service.Dim(tt.spec)); diff != "" {
				t.Errorf("Dim() mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

func TestNGrams(t *testing.T) {
	tests := []struct {
		name   string
		text   string
		lo, hi int
		want   []string
	}{
		{name: "unigrams and bigrams", text: "a b", lo: 1, hi: 2, want: []string{"a", " ", "b", "a ", " b"}},
		{name: "whitespace runs collapse", text: " a \n\t b ", lo: 1, hi: 2, want: []string{"a", " ", "b", "a ", " b"}},
		{name: "characters, not bytes", text: "探して", lo: 2, hi: 2, want: []string{"探し", "して"}},
		{name: "n longer than the text", text: "ab", lo: 3, hi: 3, want: nil},
		{name: "empty text", text: "", lo: 1, hi: 3, want: nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if diff := cmp.Diff(tt.want, service.NGrams(tt.text, tt.lo, tt.hi), cmpopts.EquateEmpty()); diff != "" {
				t.Errorf("NGrams() mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

func TestFeaturize(t *testing.T) {
	spec := syntheticSpec
	base := int32(spec.HashBuckets)
	// view reduces a vector to what the case is about: its L2 norm and which
	// dense features are set.
	type view struct {
		Norm  float64
		Dense []int32
		Width int
	}
	project := func(v service.Vector) view {
		out := view{Width: len(v.Idx)}
		sum := 0.0
		for i, k := range v.Idx {
			sum += float64(v.Val[i]) * float64(v.Val[i])
			if k >= base {
				out.Dense = append(out.Dense, k-base)
			}
		}
		out.Norm = math.Round(math.Sqrt(sum)*1e6) / 1e6
		return out
	}
	tests := []struct {
		name string
		sig  service.Signals
		spec model.FeatureSpec
		want view
	}{
		{
			name: "no text and no signals is the zero vector",
			sig:  service.Signals{},
			spec: spec,
			want: view{},
		},
		{
			name: "text only is unit length without dense features",
			sig:  service.Signals{Normalised: "aaa"},
			spec: spec,
			want: view{Norm: 1, Width: 2}, // "a" and "aa" (different buckets for this hash)
		},
		{
			name: "every dense feature is set",
			sig: service.Signals{
				Normalised: "ab", Chars: 2, ClassCap: 9,
				Scores: map[model.Class]int{
					model.ClassLookup: 1, model.ClassMechanical: 1, model.ClassStandard: 1,
					model.ClassComplex: 1, model.ClassArchitecture: 1,
				},
				Danger: []string{"x"}, ContextCues: []string{"a", "b", "c", "d"}, CodeFence: true, StackTrace: true,
			},
			spec: spec,
			want: view{Norm: 1, Dense: []int32{0, 1, 2, 3, 4, 5, 6, 7, 8, 9}, Width: 13},
		},
		{
			name: "zero caps and scales fall back to one",
			sig:  service.Signals{Chars: 1, ContextCues: []string{"a"}},
			spec: model.FeatureSpec{NGramMin: 1, NGramMax: 1, HashBuckets: 8},
			want: view{Norm: 1, Dense: []int32{5, 7}, Width: 2},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if diff := cmp.Diff(tt.want, project(service.Featurize(tt.sig, tt.spec)), cmpopts.EquateEmpty()); diff != "" {
				t.Errorf("Featurize() mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

func TestDot(t *testing.T) {
	tests := []struct {
		name string
		v    service.Vector
		w    []float64
		want float64
	}{
		{name: "matching indices", v: service.Vector{Idx: []int32{0, 2}, Val: []float32{1, 2}}, w: []float64{3, 0, 4}, want: 11},
		{name: "indices beyond the row are ignored", v: service.Vector{Idx: []int32{0, 5}, Val: []float32{1, 2}}, w: []float64{3, 0, 4}, want: 3},
		{name: "empty vector", v: service.Vector{}, w: []float64{1}, want: 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if diff := cmp.Diff(tt.want, service.Dot(tt.v, tt.w)); diff != "" {
				t.Errorf("Dot() mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

func TestCosine(t *testing.T) {
	tests := []struct {
		name string
		a, b service.Vector
		want float64
	}{
		{
			name: "only shared indices count, whichever side is behind",
			a:    service.Vector{Idx: []int32{0, 2, 4}, Val: []float32{1, 2, 3}},
			b:    service.Vector{Idx: []int32{1, 2, 5}, Val: []float32{5, 4, 6}},
			want: 8,
		},
		{
			name: "identical unit vectors",
			a:    service.Vector{Idx: []int32{3}, Val: []float32{1}},
			b:    service.Vector{Idx: []int32{3}, Val: []float32{1}},
			want: 1,
		},
		{name: "disjoint", a: service.Vector{Idx: []int32{1}, Val: []float32{1}}, b: service.Vector{Idx: []int32{2}, Val: []float32{1}}, want: 0},
		{name: "empty", want: 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if diff := cmp.Diff(tt.want, service.Cosine(tt.a, tt.b)); diff != "" {
				t.Errorf("Cosine() mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

func TestSeenIndex(t *testing.T) {
	tests := []struct {
		name string
		gram string
		bits int
	}{
		{name: "within a 64-bit set", gram: "ab", bits: 64},
		{name: "within a large set", gram: "探し", bits: 1 << 20},
		{name: "a zero size is one slot", gram: "ab", bits: 0},
		{name: "a negative size is one slot", gram: "ab", bits: -5},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := service.SeenIndex(tt.gram, tt.bits)
			inRange := got >= 0 && got < max(tt.bits, 1)
			if diff := cmp.Diff(true, inRange); diff != "" {
				t.Errorf("SeenIndex(%q, %d) = %d out of range", tt.gram, tt.bits, got)
			}
			if diff := cmp.Diff(got, service.SeenIndex(tt.gram, tt.bits)); diff != "" {
				t.Errorf("SeenIndex is not deterministic:\n%s", diff)
			}
		})
	}
}

func TestUnseenRatio(t *testing.T) {
	spec := syntheticSpec // bigrams, 64 seen bits
	set := func(grams ...string) []uint64 {
		words := make([]uint64, spec.SeenBits/model.BitsPerWord)
		for _, g := range grams {
			i := service.SeenIndex(g, spec.SeenBits)
			words[i/model.BitsPerWord] |= 1 << (uint(i) % model.BitsPerWord)
		}
		return words
	}
	if service.SeenIndex("ab", spec.SeenBits) == service.SeenIndex("bc", spec.SeenBits) {
		t.Fatal("test fixture: ab and bc collide in the seen set")
	}
	tests := []struct {
		name string
		text string
		seen []uint64
		want float64
	}{
		{name: "no n-grams", text: "a", seen: set(), want: 0},
		{name: "no seen set", text: "abc", seen: nil, want: 0},
		{name: "nothing seen", text: "abc", seen: set(), want: 1},
		{name: "half seen", text: "abc", seen: set("ab"), want: 0.5},
		{name: "all seen", text: "abc", seen: set("ab", "bc"), want: 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			a := model.Artifact{Features: spec, Seen: tt.seen}
			if diff := cmp.Diff(tt.want, service.UnseenRatio(service.Signals{Normalised: tt.text}, a)); diff != "" {
				t.Errorf("UnseenRatio() mismatch (-want +got):\n%s", diff)
			}
		})
	}
}
