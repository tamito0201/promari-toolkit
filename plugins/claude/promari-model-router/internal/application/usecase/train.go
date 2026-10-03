package usecase

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"time"

	"promari-model-router/internal/domain/learn"
	"promari-model-router/internal/domain/model"
	"promari-model-router/internal/domain/repository"
	"promari-model-router/internal/domain/service"
	"promari-model-router/pkg/fp"
)

// ErrNotRouting is returned instead of writing, over the artifact routing
// uses, one that cannot route: trained on too few labelled prompts (it has no
// classifier) or on the embedded set alone (it is never trusted). Written
// there, it would silently put routing back on the rules.
var ErrNotRouting = errors.New("not written: the trained artifact cannot route (too few labelled prompts, or the embedded set alone) " +
	"and would replace the one routing uses; pass --output to write it elsewhere")

// TrainUseCase fits the artifact from labelled files and the ledger.
type TrainUseCase struct {
	Config repository.RoutingConfig
	Ledger repository.LedgerReader
	Cases  repository.CaseSource
	// Artifacts is where the artifact goes by default (the data directory).
	Artifacts repository.ArtifactWriter
	// OutputTo is where it goes for an explicit --output path.
	OutputTo func(path string) repository.ArtifactWriter
	Clock    repository.Clock
}

// TrainInput selects the data.
type TrainInput struct {
	Files      []string // JSONL files; empty = the embedded evaluation set
	LedgerDays float64  // 0 = do not learn from the ledger
	Output     string   // "" = the artifact store's local path
	Cwd        string
}

// Execute trains, calibrates, and saves the artifact.
func (u TrainUseCase) Execute(ctx context.Context, in TrainInput) (model.Artifact, string, error) {
	var samples []learn.Sample
	sources := in.Files
	if len(sources) == 0 {
		sources = []string{""}
	}
	st := u.Config.Settings(in.Cwd)
	for _, f := range sources {
		cases, err := u.Cases.Cases(f)
		if err != nil {
			return model.Artifact{}, "", err
		}
		samples = append(samples, slices.Collect(fp.Map(slices.Values(cases), func(c learn.Case) learn.Sample {
			return learn.Sample{Text: c.Text, Class: c.Expect}
		}))...)
	}
	art := learn.Train(samples, u.Config.Lexicon(in.Cwd), u.Config.Tiers(), st, learn.Meta{
		Now: u.Clock.Now().UTC(), Source: fmt.Sprintf("%d labelled prompts from %d file(s)", len(samples), len(sources)),
	})
	// Only the user's own labelled prompts make a trusted artifact. Trained on
	// the embedded synthetic set alone, it raised harmful downgrades on real
	// prompts, so it is recorded as embedded and never drives routing.
	art.Origin = model.OriginLocal
	if len(in.Files) == 0 {
		art.Origin = model.OriginEmbedded
	}

	if in.LedgerDays != 0 {
		w, err := window(in.LedgerDays, 0)
		if err != nil {
			return art, "", err
		}
		outcomes, err := u.outcomes(ctx, w, st.Buckets)
		if err != nil {
			return art, "", err
		}
		art.Posteriors, art.CostMedian = learn.LedgerStats(outcomes)
		art.Metrics["ledger_outcomes"] = float64(len(outcomes))
	}
	store := u.Artifacts
	switch {
	case in.Output != "":
		store = u.OutputTo(in.Output)
	case !art.Trusted():
		return art, "", ErrNotRouting
	}
	path, err := store.Save(art)
	return art, path, err
}

// outcomes joins subagent decisions and results by tool_use_id.
func (u TrainUseCase) outcomes(ctx context.Context, window time.Duration, buckets model.BucketSettings) ([]learn.Outcome, error) {
	entries, err := collect(u.Ledger.Since(ctx, u.Clock.Now().Add(-window)))
	if err != nil {
		return nil, err
	}
	var out []learn.Outcome
	runs := service.JoinSubagentRuns(entries)
	for i := range runs {
		run := &runs[i]
		d, r := &run.Decision, &run.Result
		if !run.Evidence() {
			continue
		}
		out = append(out, learn.Outcome{
			SessionID: d.SessionID, PromptSHA: d.PromptSHA, Class: d.Class, Bucket: buckets.Of(d.PromptChars),
			Tier: model.TierOf(r.Resolved), Success: r.Status.Succeeded(), Tokens: r.TotalTokens, At: r.At,
		})
	}
	return out, nil
}
