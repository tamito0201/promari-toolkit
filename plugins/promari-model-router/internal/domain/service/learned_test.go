package service_test

import (
	"maps"
	"slices"
	"testing"

	"github.com/google/go-cmp/cmp"

	"github.com/tamito0201/promari-toolkit/plugins/promari-model-router/internal/domain/model"
	"github.com/tamito0201/promari-toolkit/plugins/promari-model-router/internal/domain/service"
	"github.com/tamito0201/promari-toolkit/plugins/promari-model-router/internal/infrastructure/artifact"
)

// Every stage that reads the artifact (cascade, gate, ledger) must switch off
// together: an embedded (synthetic) artifact or `--rules-only` must not let its
// Triage gate close a class on real traffic.
func TestArtifactStagesNeedTheLearnedStage(t *testing.T) {
	lex, table, defaults := fixtures(t)
	base, err := artifact.Embedded()
	if err != nil {
		t.Fatal(err)
	}
	base.Gate = map[model.Class]bool{model.ClassLookup: false}

	tests := []struct {
		name          string
		origin        string
		enabled       bool
		allowEmbedded bool
		wantReason    string
	}{
		{name: "embedded artifact: gate ignored", origin: model.OriginEmbedded, enabled: true, wantReason: "rule:lookup"},
		{name: "embedded artifact explicitly allowed: gate closes the class", origin: model.OriginEmbedded, enabled: true, allowEmbedded: true, wantReason: "gate-closed"},
		{name: "rules only: gate ignored", origin: model.OriginLocal, enabled: false, wantReason: "rule:lookup"},
		{name: "local artifact: gate closes the class", origin: model.OriginLocal, enabled: true, wantReason: "gate-closed"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			a := base
			a.Gate = maps.Clone(base.Gate)
			a.Origin = tt.origin
			// Keep the learned stages from deciding on their own so the gate is what we observe.
			a.Seen, a.Neighbors, a.Tau = nil, nil, nil
			st := defaults
			st.Model.Enabled = tt.enabled
			st.Model.AllowEmbedded = tt.allowEmbedded
			got, trace := service.Route(service.RouteInput{
				Call:     model.AgentCall{Prompt: "UserService がどこで定義されているか探して"},
				Session:  model.SessionModel{Model: defaults.Eval.SessionModel, Source: model.SourceTranscript},
				Settings: st, Table: table, Lexicon: lex, Artifact: a,
			})
			if diff := cmp.Diff(tt.wantReason, got.Reason); diff != "" {
				t.Errorf("reason mismatch (-want +got):\n%s\npath %v", diff, trace.Path)
			}
		})
	}
}

// syntheticSpec is small enough that a test can reason about every feature.
var syntheticSpec = model.FeatureSpec{NGramMin: 1, NGramMax: 2, HashBuckets: 8, SeenBits: 64, DenseScaleLogChars: 8, DenseContextCap: 3}

// synthetic is a trusted artifact whose weights are all zero, so the class
// probabilities are softmax(bias) whatever the prompt: each case sets exactly
// the probabilities the stage under test should see.
func synthetic(bias ...float64) model.Artifact {
	w := make([][]float64, len(model.ClassOrder))
	for i := range w {
		w[i] = make([]float64, service.Dim(syntheticSpec))
	}
	return model.Artifact{
		Origin: model.OriginLocal, Features: syntheticSpec, Classes: slices.Clone(model.ClassOrder),
		Weights: w, Bias: bias, Temperature: 1, ConformalQ: 0.5,
	}
}

func beta(alpha, b float64) model.Beta { return model.Beta{Alpha: alpha, Beta: b} }

func TestRouteLearnedStages(t *testing.T) {
	lex, table, defaults := fixtures(t)
	const (
		lookup = "UserService がどこで定義されているか探して" // the rule says lookup (short bucket)
		vague  = "いい感じにして"                     // the rule abstains
	)
	confidentLookup := []float64{10, 0, 0, 0, 0}
	key := func(tier model.Tier) string { return model.PosteriorKey(model.ClassLookup, model.BucketShort, tier) }
	// A neighbour identical to the vague prompt (similarity 1).
	twin := service.Featurize(lex.ExtractSignals(vague), syntheticSpec)
	// The twin's similarity to itself is 1 up to float32 rounding; a floor at
	// exactly that value must still pass.
	twinSim := service.NearestNeighbors(twin, []model.Neighbor{{Idx: twin.Idx, Val: twin.Val}}, 1)[0].Score

	tests := []struct {
		name       string
		call       model.AgentCall
		artifact   func() model.Artifact
		policy     func(model.Settings) model.Settings
		table      func(model.TierTable) model.TierTable
		want       model.Decision
		wantSource string
		wantEscal  bool
	}{
		{
			name:       "a confident model decides when the rule abstains",
			call:       model.AgentCall{Prompt: vague},
			artifact:   func() model.Artifact { return synthetic(confidentLookup...) },
			want:       model.Decision{Action: model.ActionInject, Reason: "model:lookup", Target: model.TierHaiku, Class: model.ClassLookup},
			wantSource: "model",
		},
		{
			// A trusted artifact with a bias missing and a seen bitset shorter
			// than seen_bits panicked in the hook (index out of range); it is
			// not Ready now, so the rules decide.
			name: "a malformed artifact is not used (rules only), never a panic",
			call: model.AgentCall{Prompt: lookup},
			artifact: func() model.Artifact {
				a := synthetic(confidentLookup...)
				a.Bias = a.Bias[:1]
				a.Seen, a.Features.SeenBits = []uint64{0}, 1<<10
				return a
			},
			want:       model.Decision{Action: model.ActionInject, Reason: "rule:lookup", Target: model.TierHaiku, Class: model.ClassLookup},
			wantSource: "rule",
		},
		{
			name: "a prediction set spanning two tiers abstains",
			call: model.AgentCall{Prompt: vague},
			artifact: func() model.Artifact {
				a := synthetic(5, 0, 0, 5, 0)
				a.ConformalQ = 0.6
				return a
			},
			want: model.Decision{Action: model.ActionNone, Reason: "abstain"},
		},
		{
			name: "unseen n-grams hold a model decision",
			call: model.AgentCall{Prompt: vague},
			artifact: func() model.Artifact {
				a := synthetic(confidentLookup...)
				a.Seen, a.MaxUnseenRatio = []uint64{0}, 0.5
				return a
			},
			want:       model.Decision{Action: model.ActionNone, Reason: "out-of-distribution"},
			wantSource: "model",
		},
		{
			name: "the settings threshold outranks the artifact's",
			call: model.AgentCall{Prompt: vague},
			artifact: func() model.Artifact {
				a := synthetic(confidentLookup...)
				a.Seen = []uint64{0}
				return a
			},
			policy:     func(p model.Settings) model.Settings { p.Model.MaxUnseenRatio = 0.5; return p },
			want:       model.Decision{Action: model.ActionNone, Reason: "out-of-distribution"},
			wantSource: "model",
		},
		{
			name: "a dissimilar nearest neighbour holds a model decision",
			call: model.AgentCall{Prompt: vague},
			artifact: func() model.Artifact {
				a := synthetic(confidentLookup...)
				a.Neighbors = []model.Neighbor{{Idx: []int32{int32(syntheticSpec.Dim() - 1)}, Val: []float32{1}, Class: model.ClassLookup}}
				a.MinNeighborSim = 0.5
				return a
			},
			want:       model.Decision{Action: model.ActionNone, Reason: "out-of-distribution"},
			wantSource: "model",
		},
		{
			name: "a similar nearest neighbour lets the model decide",
			call: model.AgentCall{Prompt: vague},
			artifact: func() model.Artifact {
				a := synthetic(confidentLookup...)
				a.Neighbors = []model.Neighbor{{Idx: twin.Idx, Val: twin.Val, Class: model.ClassLookup}}
				a.MinNeighborSim = 0.5
				return a
			},
			want:       model.Decision{Action: model.ActionInject, Reason: "model:lookup", Target: model.TierHaiku, Class: model.ClassLookup},
			wantSource: "model",
		},
		{
			name: "a similarity exactly at the floor lets the model decide",
			call: model.AgentCall{Prompt: vague},
			artifact: func() model.Artifact {
				a := synthetic(confidentLookup...)
				a.Neighbors = []model.Neighbor{{Idx: twin.Idx, Val: twin.Val, Class: model.ClassLookup}}
				a.MinNeighborSim = twinSim
				return a
			},
			want:       model.Decision{Action: model.ActionInject, Reason: "model:lookup", Target: model.TierHaiku, Class: model.ClassLookup},
			wantSource: "model",
		},
		{
			name: "a similarity floor without neighbours holds nothing",
			call: model.AgentCall{Prompt: vague},
			artifact: func() model.Artifact {
				a := synthetic(confidentLookup...)
				a.MinNeighborSim = 0.5
				return a
			},
			want:       model.Decision{Action: model.ActionInject, Reason: "model:lookup", Target: model.TierHaiku, Class: model.ClassLookup},
			wantSource: "model",
		},
		{
			// Every n-gram of the vague prompt is unseen (ratio 1).
			name: "an unseen ratio exactly at the limit lets the model decide",
			call: model.AgentCall{Prompt: vague},
			artifact: func() model.Artifact {
				a := synthetic(confidentLookup...)
				a.Seen, a.MaxUnseenRatio = []uint64{0}, 1
				return a
			},
			want:       model.Decision{Action: model.ActionInject, Reason: "model:lookup", Target: model.TierHaiku, Class: model.ClassLookup},
			wantSource: "model",
		},
		{
			// A two-class set (complex, architecture) is not confident deep
			// work: the fixed rule for Explore stands.
			name: "a fixed rule stands against an unconfident deep model",
			call: model.AgentCall{Prompt: "認証まわりのファイルを探して", SubagentType: "Explore"},
			artifact: func() model.Artifact {
				a := synthetic(0, 0, 0, 5, 5)
				a.ConformalQ = 0.6
				return a
			},
			want:       model.Decision{Action: model.ActionInject, Reason: "fixed:lookup", Target: model.TierHaiku, Class: model.ClassLookup, SubagentType: "Explore"},
			wantSource: "fixed",
		},
		{
			name: "a rule decision is not held by the ood guard",
			call: model.AgentCall{Prompt: lookup},
			artifact: func() model.Artifact {
				a := synthetic(confidentLookup...)
				a.Seen, a.MaxUnseenRatio = []uint64{0}, 0.5
				return a
			},
			want:       model.Decision{Action: model.ActionInject, Reason: "rule:lookup", Target: model.TierHaiku, Class: model.ClassLookup},
			wantSource: "rule",
		},
		{
			name: "the risk threshold holds an unsafe downgrade",
			call: model.AgentCall{Prompt: lookup},
			artifact: func() model.Artifact {
				a := synthetic(0, 0, 0, 0, 0) // P(haiku is enough) = 0.4
				a.Tau = map[model.LengthBucket]float64{model.BucketShort: 0.9}
				return a
			},
			want:       model.Decision{Action: model.ActionNone, Reason: "risk-hold", Class: model.ClassLookup},
			wantSource: "rule",
		},
		{
			// Uniform probabilities: P(haiku is enough) = 0.2 + 0.2 = 0.4 = τ.
			name: "a safe probability exactly at the threshold passes",
			call: model.AgentCall{Prompt: lookup},
			artifact: func() model.Artifact {
				a := synthetic(0, 0, 0, 0, 0)
				a.Tau = map[model.LengthBucket]float64{model.BucketShort: 0.4}
				return a
			},
			want:       model.Decision{Action: model.ActionInject, Reason: "rule:lookup", Target: model.TierHaiku, Class: model.ClassLookup},
			wantSource: "rule",
		},
		{
			name: "the risk threshold passes a safe downgrade",
			call: model.AgentCall{Prompt: lookup},
			artifact: func() model.Artifact {
				a := synthetic(confidentLookup...)
				a.Tau = map[model.LengthBucket]float64{model.BucketShort: 0.9}
				return a
			},
			want:       model.Decision{Action: model.ActionInject, Reason: "rule:lookup", Target: model.TierHaiku, Class: model.ClassLookup},
			wantSource: "rule",
		},
		{
			name:     "a fixed rule yields to a confident deep model",
			call:     model.AgentCall{Prompt: "認証まわりのファイルを探して", SubagentType: "Explore"},
			artifact: func() model.Artifact { return synthetic(0, 0, 0, 10, 0) },
			want:     model.Decision{Action: model.ActionNone, Reason: "deep-work-keep", Class: model.ClassLookup, SubagentType: "Explore"},
		},
		{
			name: "no ledger evidence keeps the static table",
			call: model.AgentCall{Prompt: lookup},
			artifact: func() model.Artifact {
				a := synthetic(confidentLookup...)
				a.Posteriors = map[string]model.Beta{"unrelated": beta(100, 1)}
				return a
			},
			want:       model.Decision{Action: model.ActionInject, Reason: "rule:lookup", Target: model.TierHaiku, Class: model.ClassLookup},
			wantSource: "rule",
		},
		{
			name: "too few observations are no evidence",
			call: model.AgentCall{Prompt: lookup},
			artifact: func() model.Artifact {
				a := synthetic(confidentLookup...)
				a.Posteriors = map[string]model.Beta{key(model.TierHaiku): beta(1, 4)}
				return a
			},
			want:       model.Decision{Action: model.ActionInject, Reason: "rule:lookup", Target: model.TierHaiku, Class: model.ClassLookup},
			wantSource: "rule",
		},
		{
			name: "a good record at the target keeps it",
			call: model.AgentCall{Prompt: lookup},
			artifact: func() model.Artifact {
				a := synthetic(confidentLookup...)
				a.Posteriors = map[string]model.Beta{key(model.TierHaiku): beta(100, 1)}
				return a
			},
			want:       model.Decision{Action: model.ActionInject, Reason: "rule:lookup", Target: model.TierHaiku, Class: model.ClassLookup},
			wantSource: "rule",
		},
		{
			name: "a poor record escalates to the next tier",
			call: model.AgentCall{Prompt: lookup},
			artifact: func() model.Artifact {
				a := synthetic(confidentLookup...)
				a.Posteriors = map[string]model.Beta{key(model.TierHaiku): beta(1, 100), key(model.TierSonnet): beta(100, 1)}
				return a
			},
			want:       model.Decision{Action: model.ActionInject, Reason: "rule:standard+posterior", Target: model.TierSonnet, Class: model.ClassStandard},
			wantSource: "rule",
			wantEscal:  true,
		},
		{
			// Beta(8, 2): exactly min_observations (8), mean 0.8, sd
			// sqrt(16 / (100 * 11)) ≈ 0.121, so mean − 1·sd ≈ 0.679 < 0.8.
			name: "exactly min_observations is evidence, and the sd is a penalty",
			call: model.AgentCall{Prompt: lookup},
			artifact: func() model.Artifact {
				a := synthetic(confidentLookup...)
				a.Posteriors = map[string]model.Beta{key(model.TierHaiku): beta(8, 2), key(model.TierSonnet): beta(100, 1)}
				return a
			},
			want:       model.Decision{Action: model.ActionInject, Reason: "rule:standard+posterior", Target: model.TierSonnet, Class: model.ClassStandard},
			wantSource: "rule",
			wantEscal:  true,
		},
		{
			// γ = 0: the score is the mean 75/100 = 0.75, exactly the target.
			name: "a score exactly at target_success keeps the tier",
			call: model.AgentCall{Prompt: lookup},
			artifact: func() model.Artifact {
				a := synthetic(confidentLookup...)
				a.Posteriors = map[string]model.Beta{key(model.TierHaiku): beta(75, 25), key(model.TierSonnet): beta(100, 1)}
				return a
			},
			policy: func(p model.Settings) model.Settings {
				p.Model.PosteriorGamma, p.Model.TargetSuccess = 0, 0.75
				return p
			},
			want:       model.Decision{Action: model.ActionInject, Reason: "rule:lookup", Target: model.TierHaiku, Class: model.ClassLookup},
			wantSource: "rule",
		},
		{
			// 1,000 tokens over cost_scale_tokens (1,000,000) is a cost of
			// 0.001: too small to outweigh a record of 100/101.
			name: "the token cost is scaled before it is weighed",
			call: model.AgentCall{Prompt: lookup},
			artifact: func() model.Artifact {
				a := synthetic(confidentLookup...)
				a.Posteriors = map[string]model.Beta{key(model.TierHaiku): beta(100, 1), key(model.TierSonnet): beta(100, 1)}
				a.CostMedian = map[string]float64{model.CostKey(model.ClassLookup, model.TierHaiku): 1000}
				return a
			},
			policy:     func(p model.Settings) model.Settings { p.Model.CostLambda = 1; return p },
			want:       model.Decision{Action: model.ActionInject, Reason: "rule:lookup", Target: model.TierHaiku, Class: model.ClassLookup},
			wantSource: "rule",
		},
		{
			name: "the token cost outweighs a good record",
			call: model.AgentCall{Prompt: lookup},
			artifact: func() model.Artifact {
				a := synthetic(confidentLookup...)
				a.Posteriors = map[string]model.Beta{key(model.TierHaiku): beta(100, 1), key(model.TierSonnet): beta(100, 1)}
				a.CostMedian = map[string]float64{model.CostKey(model.ClassLookup, model.TierHaiku): 500_000}
				return a
			},
			policy:     func(p model.Settings) model.Settings { p.Model.CostLambda = 1; return p },
			want:       model.Decision{Action: model.ActionInject, Reason: "rule:standard+posterior", Target: model.TierSonnet, Class: model.ClassStandard},
			wantSource: "rule",
			wantEscal:  true,
		},
		{
			name: "a poor record everywhere stops at the ceiling",
			call: model.AgentCall{Prompt: lookup},
			artifact: func() model.Artifact {
				a := synthetic(confidentLookup...)
				a.Posteriors = map[string]model.Beta{
					key(model.TierHaiku): beta(1, 100), key(model.TierSonnet): beta(1, 100),
					key(model.TierOpus): beta(1, 100), key(model.TierFable): beta(100, 1),
				}
				return a
			},
			want:       model.Decision{Action: model.ActionInject, Reason: "rule:lookup", Target: model.TierHaiku, Class: model.ClassLookup},
			wantSource: "rule",
		},
		{
			name: "escalating to a tier no class maps to keeps the class",
			call: model.AgentCall{Prompt: lookup},
			artifact: func() model.Artifact {
				a := synthetic(confidentLookup...)
				a.Posteriors = map[string]model.Beta{key(model.TierHaiku): beta(1, 100), key(model.TierSonnet): beta(100, 1)}
				return a
			},
			table: func(t model.TierTable) model.TierTable {
				t.Classes = maps.Clone(t.Classes)
				t.Classes[model.ClassStandard] = model.TierSpec{Model: model.TierOpus}
				return t
			},
			want:       model.Decision{Action: model.ActionInject, Reason: "rule:lookup+posterior", Target: model.TierHaiku, Class: model.ClassLookup},
			wantSource: "rule",
			wantEscal:  true,
		},
		{
			name: "an open gate lets the class through",
			call: model.AgentCall{Prompt: lookup},
			artifact: func() model.Artifact {
				a := synthetic(confidentLookup...)
				a.Gate = map[model.Class]bool{model.ClassLookup: true}
				return a
			},
			want:       model.Decision{Action: model.ActionInject, Reason: "rule:lookup", Target: model.TierHaiku, Class: model.ClassLookup},
			wantSource: "rule",
		},
		{
			name: "a closed gate holds the class",
			call: model.AgentCall{Prompt: lookup},
			artifact: func() model.Artifact {
				a := synthetic(confidentLookup...)
				a.Gate = map[model.Class]bool{model.ClassLookup: false}
				return a
			},
			want:       model.Decision{Action: model.ActionNone, Reason: "gate-closed", Class: model.ClassLookup},
			wantSource: "rule",
		},
		{
			name: "a route tag passes a closed gate",
			call: model.AgentCall{Prompt: "[route: lookup] " + lookup},
			artifact: func() model.Artifact {
				a := synthetic(0, 0, 0, 0, 0)
				a.Gate = map[model.Class]bool{model.ClassLookup: false}
				a.Tau = map[model.LengthBucket]float64{model.BucketShort: 0.9}
				return a
			},
			want:       model.Decision{Action: model.ActionInject, Reason: "tag:lookup", Target: model.TierHaiku, Class: model.ClassLookup},
			wantSource: "tag",
		},
	}
	type outcome struct {
		Decision  model.Decision
		Source    string
		Escalated bool
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			st := defaults
			if tt.policy != nil {
				st = tt.policy(st)
			}
			tb := table
			if tt.table != nil {
				tb = tt.table(tb)
			}
			got, trace := service.Route(service.RouteInput{
				Call:     tt.call,
				Session:  model.SessionModel{Model: defaults.Eval.SessionModel, Source: model.SourceTranscript},
				Settings: st, Table: tb, Lexicon: lex, Artifact: tt.artifact(),
			})
			want := outcome{Decision: tt.want, Source: tt.wantSource, Escalated: tt.wantEscal}
			if want.Decision.SubagentType == "" {
				want.Decision.SubagentType = "general-purpose"
			}
			if diff := cmp.Diff(want, outcome{got, trace.Source, trace.Escalated}); diff != "" {
				t.Errorf("Route() mismatch (-want +got):\n%s\npath %v probs %v set %v psafe %.3f tau %.3f posterior %.3f unseen %.3f sim %.3f",
					diff, trace.Path, trace.Probs, trace.Set, trace.PSafe, trace.Tau, trace.Posterior, trace.Unseen, trace.NeighborSim)
			}
		})
	}
}
