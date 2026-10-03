# Methods and sources

Each routing stage implements a published method. Where the method was trained or
evaluated on something other than subagent briefs, the adaptation is noted. Numbers
reported by the papers are theirs, measured under their conditions; they are not claims
about this plugin.

| # | Method | Source | Where |
|---|---|---|---|
| 1 | Hold unless the probability that a cheaper tier suffices clears a threshold; governance floors; switch only at subagent start (cache-preserving) | Harness Tokenomics, arXiv:2609.28919 | `service.riskStage`, `floorStage`, PreToolUse only |
| 2 | Static class → model table as the default destination | Most of the LLM Routing Gap Is Task Type, arXiv:2608.23023 | `data/tiers.toml` |
| 3 | Compare against simple baselines and the oracle | LLMRouterBench, arXiv:2601.07206 | `learn.Evaluate` baselines |
| 4 | Separate signal extraction from decision rules | vLLM Semantic Router, arXiv:2603.04444 | `service.ExtractSignals` vs the state graph |
| 5 | One temperature softmax instead of independent per-class thresholds | Conflict-Free Policy Languages, arXiv:2603.18174 | `service.Softmax` |
| 6 | Cascade: rules → CPU classifier → hold; only a confident stage decides | GuardChain, arXiv:2512.19011 | `cascadeStage`, `oodStage` |
| 7 | Character n-gram logistic regression with temperature scaling | Compositional Meta-Routing, arXiv:2608.00106 | `learn.TrainSoftmax`, `FitTemperature` |
| 8 | Nearest-neighbour check against training data | kNN routers, arXiv:2505.12601 | `service.NearestNeighbors` (out-of-distribution) |
| 10 | Isotonic calibration once labels are plentiful | UCCI arXiv:2605.18796; AutoRelAnnotator arXiv:2606.25871 | `learn.FitIsotonic` |
| 11 | Score granularity as a metric | Score Granularity Gap, arXiv:2606.22179 | `learn.Granularity` |
| 12 | Abstain when the split-conformal set spans several tiers | Conformal Cascade, arXiv:2607.25018 | `service.PredictionSet` |
| 13 | Conformal risk control of the wrong-downgrade rate | CR² arXiv:2605.12001; Conformal Risk Control arXiv:2208.02814 | `learn.RiskControlledThreshold` |
| 15 | Mondrian (per length bucket) risk thresholds | Calibrated Trust, arXiv:2608.14617 | `Artifact.Tau` per bucket |
| 16 | Beta posteriors per context cell with an uncertainty penalty | CADMAS-CTX, arXiv:2604.17950 | `ledgerStage`, `learn.LedgerStats` |
| 17 | Reward attributed per task | TRACE-Router, arXiv:2607.22465 | ledger join by `tool_use_id` |
| 18 | A retry is an implicit failure signal | ACQB, arXiv:2602.02061 | `learn.LabelRetries`, `retry-keep` floor |
| 23 | Predict quality and cost separately; choose by success − λ·cost | CARROT, arXiv:2502.03261 | `ledgerStage` (`cost_lambda`) |
| 24 | Routing collapse metric | EquiRouter, arXiv:2602.03478 | `learn.Collapse` |
| 27 | Two falsifiable conditions before downgrading a class | Triage, arXiv:2604.07494 | `learn.TriageGate`, `gateStage` |
| 28 | The planner writes the tier | Planner-as-Router, arXiv:2609.32917 | `[route: <class>]` tag |
| 29 | Difficulty that only shows during execution raises the tier | SWE-Router, arXiv:2607.00053 | correction advice, `retry-keep` |

Numbers skip the methods that were evaluated but not adopted (LinUCB, budgeted bandits,
Lagrangian budgets, IRT, UniRoute clusters, learn-then-test sets, surrogate-reward bandits,
sparse supervision): they need data volumes or budget features the plugin does not have yet.
