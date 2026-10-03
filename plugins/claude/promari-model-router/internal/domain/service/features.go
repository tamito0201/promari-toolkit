package service

import (
	"cmp"
	"hash/fnv"
	"maps"
	"math"
	"slices"
	"strings"

	"promari-model-router/internal/domain/model"
)

// DenseFeatures is the number of dense features appended after the hash
// buckets (model.DenseFeatures).
const DenseFeatures = model.DenseFeatures

// Vector is a sparse feature vector with sorted indices.
type Vector struct {
	Idx []int32
	Val []float32
}

// Dim is the total feature dimension for a spec.
func Dim(spec model.FeatureSpec) int { return spec.Dim() }

func hash32(s string, seed byte) uint32 {
	h := fnv.New32a()
	_, _ = h.Write([]byte{seed})
	_, _ = h.Write([]byte(s))
	return h.Sum32()
}

// NGrams yields the character n-grams of the normalised text, with runs of
// whitespace collapsed. Character n-grams need no word segmentation, which is
// what makes the classifier work for Japanese (arXiv:2608.00106 uses word +
// character n-grams; words add nothing for unsegmented text).
func NGrams(text string, lo, hi int) []string {
	r := []rune(strings.Join(strings.Fields(text), " "))
	var out []string
	for n := lo; n <= hi; n++ {
		for i := 0; i+n <= len(r); i++ {
			out = append(out, string(r[i:i+n]))
		}
	}
	return out
}

// Featurize builds the L2-normalised hashed n-gram vector (sublinear term
// frequency) and appends the dense rule-stage features.
func Featurize(s Signals, spec model.FeatureSpec) Vector {
	counts := map[int32]float64{}
	buckets := boundedUint32(spec.HashBuckets)
	for _, g := range NGrams(s.Normalised, spec.NGramMin, spec.NGramMax) {
		counts[int32(hash32(g, 0)%buckets)]++ //nolint:gosec // < buckets <= MaxInt32
	}
	norm := 0.0
	for k, c := range counts {
		counts[k] = 1 + math.Log(c)
		norm += counts[k] * counts[k]
	}
	norm = math.Sqrt(norm)
	base := int32(buckets) //nolint:gosec // boundedUint32 caps at MaxInt32
	classCap := float64(max(s.ClassCap, 1))
	contextCap := max(spec.DenseContextCap, 1)
	dense := []float64{
		float64(s.Scores[model.ClassLookup]) / classCap,
		float64(s.Scores[model.ClassMechanical]) / classCap,
		float64(s.Scores[model.ClassStandard]) / classCap,
		float64(s.Scores[model.ClassComplex]) / classCap,
		float64(s.Scores[model.ClassArchitecture]) / classCap,
		math.Log1p(float64(s.Chars)) / max(spec.DenseScaleLogChars, 1),
		b2f(len(s.Danger) > 0),
		math.Min(float64(len(s.ContextCues)), contextCap) / contextCap,
		b2f(s.CodeFence),
		b2f(s.StackTrace),
	}
	for i, v := range dense {
		if v != 0 {
			counts[base+int32(i)] = v * max(norm, 1) // keep dense on the same scale before normalising
		}
	}
	idx := slices.Sorted(maps.Keys(counts))
	v := Vector{Idx: idx, Val: make([]float32, len(idx))}
	total := 0.0
	for _, k := range idx {
		total += counts[k] * counts[k]
	}
	total = math.Sqrt(total)
	for i, k := range idx {
		v.Val[i] = float32(counts[k] / cmp.Or(total, 1))
	}
	return v
}

func b2f(b bool) float64 {
	if b {
		return 1
	}
	return 0
}

// Dot is the inner product of a sparse vector and a dense weight row.
func Dot(v Vector, w []float64) float64 {
	sum := 0.0
	for i, k := range v.Idx {
		if int(k) < len(w) {
			sum += float64(v.Val[i]) * w[k]
		}
	}
	return sum
}

// Cosine is the similarity of two L2-normalised sparse vectors.
func Cosine(a, b Vector) float64 {
	i, j, sum := 0, 0, 0.0
	for i < len(a.Idx) && j < len(b.Idx) {
		switch {
		case a.Idx[i] == b.Idx[j]:
			sum += float64(a.Val[i]) * float64(b.Val[j])
			i++
			j++
		case a.Idx[i] < b.Idx[j]:
			i++
		default:
			j++
		}
	}
	return sum
}

// SeenIndex is the position of an n-gram in the Seen bitset (independent seed
// from the feature hash, so collisions do not line up).
func SeenIndex(gram string, bits int) int { return int(hash32(gram, 1) % boundedUint32(bits)) }

// boundedUint32 converts a configured size to a modulus in [1, MaxInt32], so
// neither the uint32 conversion nor the int32 indices derived from it overflow.
func boundedUint32(n int) uint32 {
	return uint32(min(max(n, 1), math.MaxInt32))
}

// UnseenRatio is the share of the prompt's n-grams never observed in training
// (GuardChain, arXiv:2512.19011: small CPU classifiers are over-confident out
// of distribution, so a high unseen ratio should hold instead of route).
func UnseenRatio(s Signals, a model.Artifact) float64 {
	grams := NGrams(s.Normalised, a.Features.NGramMax, a.Features.NGramMax)
	if len(grams) == 0 || len(a.Seen) == 0 {
		return 0
	}
	unseen := 0
	for _, g := range grams {
		i := SeenIndex(g, a.Features.SeenBits)
		if a.Seen[i/model.BitsPerWord]&(1<<(uint(i)%model.BitsPerWord)) == 0 {
			unseen++
		}
	}
	return float64(unseen) / float64(len(grams))
}
