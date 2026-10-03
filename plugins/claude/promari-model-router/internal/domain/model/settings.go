package model

import (
	"errors"
	"fmt"
	"maps"
	"math"
	"slices"
	"strings"
)

// Settings is the complete, tunable configuration of the router, decoded from
// data/defaults.toml and the user and project override files. The domain
// takes every threshold, weight and text from here; nothing is hard-coded.
type Settings struct {
	Routing      RoutingSettings         `toml:"routing"`
	Classifier   ClassifierSettings      `toml:"classifier"`
	Buckets      BucketSettings          `toml:"buckets"`
	Model        LearnedSettings         `toml:"model"`
	Features     FeatureSpec             `toml:"features"`
	Training     TrainingSettings        `toml:"training"`
	Eval         EvalSettings            `toml:"eval"`
	Advice       AdviceSettings          `toml:"advice"`
	Pressure     PressureSettings        `toml:"pressure"`
	Codex        CodexSettings           `toml:"codex"`
	Cloud        CloudSettings           `toml:"cloud"`
	Runtime      RuntimeSettings         `toml:"runtime"`
	Display      DisplaySettings         `toml:"display"`
	Doctor       DoctorSettings          `toml:"doctor"`
	Report       ReportSettings          `toml:"report"`
	Query        QuerySettings           `toml:"query"`
	Serve        ServeSettings           `toml:"serve"`
	Cost         CostSettings            `toml:"cost"`
	Messages     Messages                `toml:"messages"`
	LexiconExtra map[string]LexiconExtra `toml:"lexicon_extra"`
}

// Mode switches routing on, off, or to shadow (record only).
type Mode string

// Routing modes.
const (
	ModeEnforce Mode = "enforce"
	ModeShadow  Mode = "shadow"
	ModeOff     Mode = "off"
)

// SubagentRule is how a built-in subagent type is routed.
type SubagentRule string

// Built-in subagent rules. Any other value names a fixed class.
const (
	RuleKeep     SubagentRule = "keep"
	RuleClassify SubagentRule = "classify"
)

// RoutingSettings are the enforcement rules.
type RoutingSettings struct {
	Mode                 Mode                    `toml:"mode"`
	AskOnUpgrade         bool                    `toml:"ask_on_upgrade"`
	HonorRouteTag        bool                    `toml:"honor_route_tag"`
	UnknownSessionAllows []Tier                  `toml:"unknown_session_allows"`
	ContextHold          bool                    `toml:"context_hold"`
	RetryEscalation      bool                    `toml:"retry_escalation"`
	Subagents            map[string]SubagentRule `toml:"subagents"`
	LongPrompt           struct {
		Chars     int `toml:"chars"`
		MinMargin int `toml:"min_margin"`
	} `toml:"long_prompt"`
}

// ClassifierSettings tune the rule stage.
type ClassifierSettings struct {
	StrongWeight         int `toml:"strong_weight"`
	WeakWeight           int `toml:"weak_weight"`
	ClassCap             int `toml:"class_cap"`
	MinConfidence        int `toml:"min_confidence"`
	MinMargin            int `toml:"min_margin"`
	ConfidentMultiple    int `toml:"confident_multiple"`
	ContinuationMaxChars int `toml:"continuation_max_chars"`
	CodeFenceBonus       int `toml:"code_fence_bonus"`
	StackTraceBonus      int `toml:"stack_trace_bonus"`
	LongPromptChars      int `toml:"long_prompt_chars"`
	LongPromptLines      int `toml:"long_prompt_lines"`
	LongPromptBonus      int `toml:"long_prompt_bonus"`
	MaxReasonHits        int `toml:"max_reason_hits"`
	MaxDangerHits        int `toml:"max_danger_hits"`
}

// BucketSettings are the length-bucket boundaries in characters.
type BucketSettings struct {
	ShortMaxChars  int `toml:"short_max_chars"`
	MediumMaxChars int `toml:"medium_max_chars"`
}

// Of returns the length bucket of a prompt of n characters.
func (b BucketSettings) Of(chars int) LengthBucket {
	switch {
	case chars < b.ShortMaxChars:
		return BucketShort
	case chars < b.MediumMaxChars:
		return BucketMedium
	default:
		return BucketLong
	}
}

// LearnedSettings tune the learned stages.
type LearnedSettings struct {
	Enabled         bool    `toml:"enabled"`
	AllowEmbedded   bool    `toml:"allow_embedded"`
	MaxUnseenRatio  float64 `toml:"max_unseen_ratio"`
	MinNeighborSim  float64 `toml:"min_neighbor_sim"`
	NeighborsK      int     `toml:"neighbors_k"`
	PosteriorGamma  float64 `toml:"posterior_gamma"`
	TargetSuccess   float64 `toml:"target_success"`
	MinObservations float64 `toml:"min_observations"`
	CostLambda      float64 `toml:"cost_lambda"`
	CostScaleTokens float64 `toml:"cost_scale_tokens"`
}

// Grid is an integer-stepped grid start, start+step, ..., stop.
type Grid struct {
	Start float64 `toml:"start"`
	Stop  float64 `toml:"stop"`
	Step  float64 `toml:"step"`
}

// BitsPerWord is the width of one uint64 word of a bitset (structural).
const BitsPerWord = 64

// gridDecimals is the rounding precision of grid values: 0.5 + 44*0.01 is
// 0.9400000000000001 in binary floating point; rounding to 12 decimals lands
// on the same double as the literal 0.94, so a point exactly on a grid value
// compares equal. A numerical constant, not a tuning parameter.
const gridDecimals = 1e12

// Values enumerates the grid from integer indices, rounded to gridDecimals
// (accumulating the step would drift).
func (g Grid) Values() []float64 {
	if g.Step <= 0 || g.Stop < g.Start {
		return []float64{g.Start}
	}
	n := int(math.Round((g.Stop - g.Start) / g.Step))
	out := make([]float64, 0, n+1)
	for i := range n + 1 {
		out = append(out, math.Round((g.Start+float64(i)*g.Step)*gridDecimals)/gridDecimals)
	}
	return out
}

// TrainingSettings tune `pmr train`.
type TrainingSettings struct {
	Folds                 int     `toml:"folds"`
	Alpha                 float64 `toml:"alpha"`
	SetAlpha              float64 `toml:"set_alpha"`
	MinBucketN            int     `toml:"min_bucket_n"`
	IsotonicMinN          int     `toml:"isotonic_min_n"`
	TauFloor              float64 `toml:"tau_floor"`
	Epochs                int     `toml:"epochs"`
	LearningRate          float64 `toml:"learning_rate"`
	L2                    float64 `toml:"l2"`
	AdamBeta1             float64 `toml:"adam_beta1"`
	AdamBeta2             float64 `toml:"adam_beta2"`
	AdamEps               float64 `toml:"adam_eps"`
	NeighborFloorQuantile float64 `toml:"neighbor_floor_quantile"`
	MaxUnseenRatio        float64 `toml:"max_unseen_ratio"`
	ECEBins               int     `toml:"ece_bins"`
	GranularityDecimals   int     `toml:"granularity_decimals"`
	TemperatureGrid       Grid    `toml:"temperature_grid"`
	TauGrid               Grid    `toml:"tau_grid"`
	Triage                struct {
		MinAUC float64 `toml:"min_auc"`
		MinN   int     `toml:"min_n"`
	} `toml:"triage"`
}

// EvalSettings are the CI gate.
type EvalSettings struct {
	SessionModel string           `toml:"session_model"`
	MinAccuracy  float64          `toml:"min_accuracy"`
	MaxHarmful   int              `toml:"max_harmful"`
	RelativeCost map[Tier]float64 `toml:"relative_cost"`
}

// Cost returns the relative cost unit of a tier.
func (e EvalSettings) Cost(t Tier) float64 {
	if v, ok := e.RelativeCost[t]; ok {
		return v
	}
	return e.RelativeCost["unknown"]
}

// AdviceSettings tune the UserPromptSubmit advice.
type AdviceSettings struct {
	Enabled        bool `toml:"enabled"`
	MaxChars       int  `toml:"max_chars"`
	MinPromptChars int  `toml:"min_prompt_chars"`
}

// PressureSettings locate and interpret the status-line usage caches.
type PressureSettings struct {
	ClaudeRateFile    string  `toml:"claude_rate_file"`
	CodexRateFile     string  `toml:"codex_rate_file"`
	FiveHourHigh      float64 `toml:"five_hour_high"`
	SevenDayHigh      float64 `toml:"seven_day_high"`
	CodexBlockPercent float64 `toml:"codex_block_percent"`
	MaxAgeHours       float64 `toml:"max_age_hours"`
}

// CodexSettings configure the Codex suggestions.
type CodexSettings struct {
	Enabled bool   `toml:"enabled"`
	Model   string `toml:"model"`
	Effort  string `toml:"effort"`
}

// CloudSettings tune `pmr cloud`, the relay to a Claude Code cloud session.
// They are read from the defaults and the user file only: a project file may
// not name the program to run (see infrastructure/settings).
type CloudSettings struct {
	// ClaudeBin is the Claude Code CLI that queues a message into the session.
	ClaudeBin     string `toml:"claude_bin"`
	SendTimeoutMS int    `toml:"send_timeout_ms"`
	// LinkFile holds the session and the time of the last message (in the data dir).
	LinkFile string `toml:"link_file"`
	// PollIntervalSeconds and MaxPolls pace /cloud reading the reply: each
	// read costs the local session tokens, so it reads seldom and stops early.
	PollIntervalSeconds int `toml:"poll_interval_seconds"`
	MaxPolls            int `toml:"max_polls"`
}

// RuntimeSettings are process-level limits and paths.
type RuntimeSettings struct {
	HookTimeoutMS        int    `toml:"hook_timeout_ms"`
	StdinLimitBytes      int64  `toml:"stdin_limit_bytes"`
	TranscriptTailBytes  int64  `toml:"transcript_tail_bytes"`
	SessionRetentionDays int    `toml:"session_retention_days"`
	LedgerRetentionDays  int    `toml:"ledger_retention_days"`
	LedgerBatch          int    `toml:"ledger_batch"`
	InjectSessionPolicy  bool   `toml:"inject_session_policy"`
	DataDir              string `toml:"data_dir"`
	LedgerFile           string `toml:"ledger_file"`
	ArtifactFile         string `toml:"artifact_file"`
	SQLiteBusyTimeoutMS  int    `toml:"sqlite_busy_timeout_ms"`
	WorkflowStepLimit    int    `toml:"workflow_step_limit"`
	ErrorDetailRunes     int    `toml:"error_detail_runes"`
	ErrorWriteTimeoutMS  int    `toml:"error_write_timeout_ms"`
	LastErrorFile        string `toml:"last_error_file"`
}

// DisplaySettings tune human-facing output.
type DisplaySettings struct {
	TraceDecimals int `toml:"trace_decimals"`
	MissChars     int `toml:"miss_chars"`
	ErrorChars    int `toml:"error_chars"`
	CostDecimals  int `toml:"cost_decimals"`
}

// DoctorSettings tune `pmr doctor`.
type DoctorSettings struct {
	WindowDays int `toml:"window_days"`
}

// ReportSettings tune `pmr report`.
type ReportSettings struct {
	DefaultDays float64 `toml:"default_days"`
}

// QuerySettings tune `pmr query`.
type QuerySettings struct {
	DefaultLimit int `toml:"default_limit"`
}

// ServeSettings tune `pmr serve`.
type ServeSettings struct {
	Addr                string `toml:"addr"`
	SSEPollMS           int    `toml:"sse_poll_ms"`
	ReadHeaderTimeoutMS int    `toml:"read_header_timeout_ms"`
	ShutdownTimeoutMS   int    `toml:"shutdown_timeout_ms"`
}

// CostSettings are the default subagent shape for `pmr cost`.
type CostSettings struct {
	ToolCalls     int `toml:"tool_calls"`
	Prompt        int `toml:"prompt"`
	ToolResult    int `toml:"tool_result"`
	OutputPerCall int `toml:"output_per_call"`
	Report        int `toml:"report"`
}

// Messages are every text the router shows to Claude (text/template).
type Messages struct {
	Prefix            string   `toml:"prefix"`
	SessionPolicy     string   `toml:"session_policy"`
	ForceDisabled     string   `toml:"force_disabled"`
	Danger            string   `toml:"danger"`
	Correction        string   `toml:"correction"`
	DeepOnWeakSession string   `toml:"deep_on_weak_session"`
	Lookup            string   `toml:"lookup"`
	BulkMechanical    string   `toml:"bulk_mechanical"`
	CodexBlocked      string   `toml:"codex_blocked"`
	CodexReview       string   `toml:"codex_review"`
	CodexTask         string   `toml:"codex_task"`
	Pressure          string   `toml:"pressure"`
	AskUpgrade        string   `toml:"ask_upgrade"`
	ConfigRejected    string   `toml:"config_rejected"`
	RouteTags         []string `toml:"route_tags"`
}

// LexiconExtra adds cue phrases per class or to the flag lists.
type LexiconExtra struct {
	Strong []string `toml:"strong"`
	Weak   []string `toml:"weak"`
	Items  []string `toml:"items"` // for danger / continuation / context / correction
}

// FlagLists are the lexicon_extra names that take `items` instead of a class.
var FlagLists = []string{"danger", "continuation", "context", "correction"}

// RoutingStages is the number of stages of the routing workflow. The workflow
// has no cycles, so a run takes at most this many steps, and a step limit below
// it would stop every route that goes the whole way (a service test keeps it
// equal to the workflow's size).
const RoutingStages = 10

// ValidateLexiconExtra rejects names that are neither a class nor a flag list,
// and keys that the named entry would ignore. A misspelt class would otherwise
// load without error and its phrases would silently never match.
func (s Settings) ValidateLexiconExtra() error {
	for _, name := range slices.Sorted(maps.Keys(s.LexiconExtra)) {
		extra := s.LexiconExtra[name]
		switch {
		case slices.Contains(FlagLists, name):
			if len(extra.Strong)+len(extra.Weak) > 0 {
				return fmt.Errorf("lexicon_extra.%s takes `items`, not strong/weak", name)
			}
		case ParseClass(name) != ClassNone:
			if len(extra.Items) > 0 {
				return fmt.Errorf("lexicon_extra.%s takes strong/weak, not `items`", name)
			}
		default:
			return fmt.Errorf("lexicon_extra.%s: unknown name (classes: %v; lists: %v)", name, ClassOrder, FlagLists)
		}
	}
	return nil
}

// minFolds is the smallest number of cross-validation folds (one to train on,
// one to hold out): structural, not a tuning parameter.
const minFolds = 2

// Validate rejects values that would crash the router or silently disable a
// safeguard: a zero timeout, a retention that deletes everything at once, a
// probability outside [0, 1]. A file with such a value is rejected like a file
// with an unknown key, so a typo cannot turn a safety check off.
func (s Settings) Validate() error {
	var errs []string
	check := func(ok bool, key string, v any, want string) {
		if !ok {
			errs = append(errs, fmt.Sprintf("%s = %v: must be %s", key, v, want))
		}
	}
	positive := func(key string, v int) { check(v > 0, key, v, "> 0") }
	nonNegative := func(key string, v int) { check(v >= 0, key, v, ">= 0") }
	probability := func(key string, v float64) { check(v >= 0 && v <= 1, key, v, "within [0, 1]") }
	percent := func(key string, v float64) { check(v >= 0 && v <= 100, key, v, "within [0, 100]") }

	check(slices.Contains([]Mode{ModeEnforce, ModeShadow, ModeOff}, s.Routing.Mode), "routing.mode", s.Routing.Mode, `"enforce", "shadow" or "off"`)
	for _, t := range s.Routing.UnknownSessionAllows {
		check(t.Known(), "routing.unknown_session_allows", t, fmt.Sprintf("one of %v", TierOrder))
	}

	rt := s.Runtime
	positive("runtime.hook_timeout_ms", rt.HookTimeoutMS)
	check(rt.StdinLimitBytes > 0, "runtime.stdin_limit_bytes", rt.StdinLimitBytes, "> 0")
	check(rt.TranscriptTailBytes > 0, "runtime.transcript_tail_bytes", rt.TranscriptTailBytes, "> 0")
	positive("runtime.ledger_retention_days", rt.LedgerRetentionDays)
	positive("runtime.session_retention_days", rt.SessionRetentionDays)
	positive("runtime.ledger_batch", rt.LedgerBatch)
	nonNegative("runtime.sqlite_busy_timeout_ms", rt.SQLiteBusyTimeoutMS)
	check(rt.WorkflowStepLimit >= RoutingStages, "runtime.workflow_step_limit", rt.WorkflowStepLimit,
		fmt.Sprintf(">= %d (the stages of the routing workflow)", RoutingStages))
	// 0 is allowed: it keeps error traces out of the ledger.
	nonNegative("runtime.error_detail_runes", rt.ErrorDetailRunes)
	positive("runtime.error_write_timeout_ms", rt.ErrorWriteTimeoutMS)
	check(rt.LastErrorFile != "", "runtime.last_error_file", `""`, "a file name")
	check(rt.LedgerFile != "", "runtime.ledger_file", `""`, "a file name")
	check(rt.ArtifactFile != "", "runtime.artifact_file", `""`, "a file name")

	positive("serve.sse_poll_ms", s.Serve.SSEPollMS)
	positive("serve.read_header_timeout_ms", s.Serve.ReadHeaderTimeoutMS)
	positive("serve.shutdown_timeout_ms", s.Serve.ShutdownTimeoutMS)
	positive("doctor.window_days", s.Doctor.WindowDays)
	check(s.Report.DefaultDays > 0, "report.default_days", s.Report.DefaultDays, "> 0")
	positive("query.default_limit", s.Query.DefaultLimit)
	nonNegative("display.trace_decimals", s.Display.TraceDecimals)
	nonNegative("display.miss_chars", s.Display.MissChars)
	nonNegative("display.error_chars", s.Display.ErrorChars)
	nonNegative("display.cost_decimals", s.Display.CostDecimals)

	f := s.Features
	positive("features.ngram_min", f.NGramMin)
	check(f.NGramMax >= f.NGramMin, "features.ngram_max", f.NGramMax, ">= features.ngram_min")
	positive("features.hash_buckets", f.HashBuckets)
	positive("features.seen_bits", f.SeenBits)

	// 0 scores every class 0: the router would abstain on every prompt.
	positive("classifier.class_cap", s.Classifier.ClassCap)

	tr := s.Training
	check(tr.Folds >= minFolds, "training.folds", tr.Folds, fmt.Sprintf(">= %d", minFolds))
	// A grid without a positive step or with its stop below its start is the
	// start alone: a τ grid collapsed to 0.5 can no longer say "never downgrade".
	for _, g := range []struct {
		key  string
		grid Grid
	}{{"training.temperature_grid", tr.TemperatureGrid}, {"training.tau_grid", tr.TauGrid}} {
		check(g.grid.Step > 0 && g.grid.Stop >= g.grid.Start, g.key, fmt.Sprintf("%+v", g.grid), "a step > 0 and stop >= start")
	}
	probability("training.alpha", tr.Alpha)
	probability("training.set_alpha", tr.SetAlpha)
	probability("training.tau_floor", tr.TauFloor)
	probability("training.neighbor_floor_quantile", tr.NeighborFloorQuantile)
	probability("training.max_unseen_ratio", tr.MaxUnseenRatio)
	probability("training.triage.min_auc", tr.Triage.MinAUC)
	positive("training.ece_bins", tr.ECEBins)

	probability("model.max_unseen_ratio", s.Model.MaxUnseenRatio)
	probability("model.min_neighbor_sim", s.Model.MinNeighborSim)
	probability("model.target_success", s.Model.TargetSuccess)
	check(s.Model.PosteriorGamma >= 0, "model.posterior_gamma", s.Model.PosteriorGamma, ">= 0")
	positive("model.neighbors_k", s.Model.NeighborsK)

	probability("eval.min_accuracy", s.Eval.MinAccuracy)
	// A tier without a cost falls back to "unknown"; without that, the cost
	// ratios of the gates divide by zero.
	check(s.Eval.RelativeCost["unknown"] > 0, "eval.relative_cost.unknown", s.Eval.RelativeCost["unknown"], "> 0")
	for _, t := range slices.Sorted(maps.Keys(s.Eval.RelativeCost)) {
		check(s.Eval.RelativeCost[t] > 0, "eval.relative_cost."+string(t), s.Eval.RelativeCost[t], "> 0")
	}
	nonNegative("eval.max_harmful", s.Eval.MaxHarmful)

	p := s.Pressure
	percent("pressure.five_hour_high", p.FiveHourHigh)
	percent("pressure.seven_day_high", p.SevenDayHigh)
	percent("pressure.codex_block_percent", p.CodexBlockPercent)
	check(p.MaxAgeHours > 0, "pressure.max_age_hours", p.MaxAgeHours, "> 0")
	if len(errs) > 0 {
		return errors.New(strings.Join(errs, "; "))
	}
	return nil
}

// Clone returns a deep copy: a caller that adjusts its copy (eval forces the
// mode, tests mutate thresholds) must not change the settings another caller
// holds.
func (s Settings) Clone() Settings {
	s.Routing.UnknownSessionAllows = slices.Clone(s.Routing.UnknownSessionAllows)
	s.Routing.Subagents = maps.Clone(s.Routing.Subagents)
	s.Eval.RelativeCost = maps.Clone(s.Eval.RelativeCost)
	s.Messages.RouteTags = slices.Clone(s.Messages.RouteTags)
	if s.LexiconExtra != nil {
		extra := make(map[string]LexiconExtra, len(s.LexiconExtra))
		for k, v := range s.LexiconExtra {
			extra[k] = LexiconExtra{Strong: slices.Clone(v.Strong), Weak: slices.Clone(v.Weak), Items: slices.Clone(v.Items)}
		}
		s.LexiconExtra = extra
	}
	return s
}
