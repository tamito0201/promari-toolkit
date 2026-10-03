package model

import (
	"errors"
	"fmt"
	"time"
)

// LengthBucket groups prompts by length for Mondrian (per-bucket)
// calibration. Boundaries come from [buckets] in the settings.
type LengthBucket string

// Length buckets.
const (
	BucketShort  LengthBucket = "short"
	BucketMedium LengthBucket = "medium"
	BucketLong   LengthBucket = "long"
)

// Buckets lists the length buckets in order.
var Buckets = []LengthBucket{BucketShort, BucketMedium, BucketLong}

// FeatureSpec fixes how text becomes a feature vector; it is stored with the
// weights so that training and inference can never disagree.
type FeatureSpec struct {
	NGramMin           int     `json:"ngram_min" toml:"ngram_min"`
	NGramMax           int     `json:"ngram_max" toml:"ngram_max"`
	HashBuckets        int     `json:"hash_buckets" toml:"hash_buckets"`
	SeenBits           int     `json:"seen_bits" toml:"seen_bits"`
	DenseScaleLogChars float64 `json:"dense_scale_log_chars" toml:"dense_scale_log_chars"`
	DenseContextCap    float64 `json:"dense_context_cap" toml:"dense_context_cap"`
}

// DenseFeatures is the number of dense features appended after the hash
// buckets: five class scores, log length, danger, context cues, code fence,
// stack trace. It is structural (the length of the dense vector), not tunable.
const DenseFeatures = 10

// Dim is the total feature dimension: the hash buckets and the dense block.
func (f FeatureSpec) Dim() int { return f.HashBuckets + DenseFeatures }

// SeenWords is the number of uint64 words of the Seen bitset: seen_bits
// rounded up to whole words (rounding down dropped the last bits).
func (f FeatureSpec) SeenWords() int { return (f.SeenBits + BitsPerWord - 1) / BitsPerWord }

// Beta is a Beta(α, β) posterior over a success probability.
type Beta struct {
	Alpha float64 `json:"alpha"`
	Beta  float64 `json:"beta"`
}

// Mean is α / (α + β).
func (b Beta) Mean() float64 { return b.Alpha / (b.Alpha + b.Beta) }

// Variance is αβ / ((α+β)²(α+β+1)).
func (b Beta) Variance() float64 {
	n := b.Alpha + b.Beta
	return b.Alpha * b.Beta / (n * n * (n + 1))
}

// Observe returns the posterior after one Bernoulli outcome.
func (b Beta) Observe(success bool) Beta {
	if success {
		b.Alpha++
	} else {
		b.Beta++
	}
	return b
}

// Isotonic is a monotone step function fitted by pool-adjacent-violators.
type Isotonic struct {
	X []float64 `json:"x"`
	Y []float64 `json:"y"`
}

// Neighbor is one stored training example for nearest-neighbour checks: a
// sparse, L2-normalised hashed n-gram vector and its label. No text is kept.
type Neighbor struct {
	Idx   []int32   `json:"i"`
	Val   []float32 `json:"v"`
	Class Class     `json:"c"`
}

// Artifact is everything `pmr train` learns. The hook only reads it.
type Artifact struct {
	Version   string    `json:"version"`
	TrainedAt time.Time `json:"trained_at"`
	Samples   int       `json:"samples"`
	Source    string    `json:"source"`
	// Origin is "embedded" for the artifact shipped in the binary (trained on
	// synthetic prompts only) and "local" for one `pmr train` wrote from the
	// user's own labelled prompts. Measured on held-out real prompts, the
	// embedded artifact was over-confident (conformal guarantees assume the
	// calibration data resembles real use), so only a local artifact may
	// drive routing; the embedded one only feeds `pmr explain`.
	Origin      string      `json:"origin,omitempty"`
	Features    FeatureSpec `json:"features"`
	Classes     []Class     `json:"classes"`
	Weights     [][]float64 `json:"weights"` // [class][feature]
	Bias        []float64   `json:"bias"`
	Temperature float64     `json:"temperature"`
	// SafeIsotonic calibrates P(needed tier <= candidate) when enough labels exist.
	SafeIsotonic *Isotonic `json:"safe_isotonic,omitempty"`
	// ConformalQ is the split-conformal quantile of 1 - p(true class).
	ConformalQ float64 `json:"conformal_q"`
	// Tau is the risk-controlled threshold per length bucket.
	Tau map[LengthBucket]float64 `json:"tau"`
	// Alpha is the target rate of wrong downgrades used to pick Tau.
	Alpha float64 `json:"alpha"`
	// Seen is a bitset of hashed n-grams observed in training (OOD check).
	Seen []uint64 `json:"seen"`
	// Neighbors are the training vectors for the nearest-neighbour OOD check.
	Neighbors []Neighbor `json:"neighbors,omitempty"`
	// MinNeighborSim is the similarity below which a prompt is out of distribution.
	MinNeighborSim float64 `json:"min_neighbor_sim"`
	// MaxUnseenRatio is the unseen n-gram ratio above which a prompt is out of distribution.
	MaxUnseenRatio float64 `json:"max_unseen_ratio"`
	// Posteriors are Beta posteriors of subagent success per "class|bucket|tier".
	Posteriors map[string]Beta `json:"posteriors,omitempty"`
	// CostMedian is the median total tokens per "class|tier".
	CostMedian map[string]float64 `json:"cost_median,omitempty"`
	// Gate enables downgrading per class (Triage conditions).
	Gate map[Class]bool `json:"gate,omitempty"`
	// Metrics are the evaluation numbers measured at training time.
	Metrics map[string]float64 `json:"metrics,omitempty"`
}

// PosteriorKey is the key of Posteriors.
func PosteriorKey(c Class, b LengthBucket, t Tier) string {
	return string(c) + "|" + string(b) + "|" + string(t)
}

// CostKey is the key of CostMedian.
func CostKey(c Class, t Tier) string { return string(c) + "|" + string(t) }

// Artifact origins.
const (
	OriginEmbedded = "embedded"
	OriginLocal    = "local"
)

// ErrArtifactBroken marks a local artifact that exists but cannot be used
// (unreadable, not JSON, or the wrong shape); routing falls back to the
// embedded artifact, which never drives routing, so it runs on rules only.
var ErrArtifactBroken = errors.New("local artifact is broken")

// Trusted reports whether the artifact may drive routing decisions.
func (a Artifact) Trusted() bool { return a.Ready() && a.Origin == OriginLocal }

// Ready reports whether the artifact holds a usable classifier.
func (a Artifact) Ready() bool { return a.Validate() == nil }

// Validate checks the shape inference relies on: a vector index, a weight
// row, a bitset word or an isotonic point out of range would panic in the
// hook (or silently read another class's weights).
func (a Artifact) Validate() error {
	dim := a.Features.Dim()
	switch {
	case len(a.Classes) == 0:
		return errors.New("no classes")
	case a.Features.HashBuckets <= 0:
		return errors.New("no hash buckets")
	case len(a.Weights) != len(a.Classes):
		return fmt.Errorf("%d weight rows for %d classes", len(a.Weights), len(a.Classes))
	case len(a.Bias) != len(a.Classes):
		return fmt.Errorf("%d biases for %d classes", len(a.Bias), len(a.Classes))
	case len(a.Seen) > 0 && (a.Features.SeenBits <= 0 || len(a.Seen) != a.Features.SeenWords()):
		return fmt.Errorf("seen bitset of %d words for seen_bits %d", len(a.Seen), a.Features.SeenBits)
	case a.SafeIsotonic != nil && len(a.SafeIsotonic.X) != len(a.SafeIsotonic.Y):
		return fmt.Errorf("isotonic map with %d x and %d y", len(a.SafeIsotonic.X), len(a.SafeIsotonic.Y))
	}
	for c, row := range a.Weights {
		if len(row) != dim {
			return fmt.Errorf("weight row %d has %d values, want %d", c, len(row), dim)
		}
	}
	for i, n := range a.Neighbors {
		if len(n.Idx) != len(n.Val) {
			return fmt.Errorf("neighbour %d has %d indices and %d values", i, len(n.Idx), len(n.Val))
		}
		for _, idx := range n.Idx {
			if idx < 0 || int(idx) >= dim {
				return fmt.Errorf("neighbour %d has index %d outside [0, %d)", i, idx, dim)
			}
		}
	}
	return nil
}
