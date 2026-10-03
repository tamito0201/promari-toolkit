# Changelog

## 1.2.0 — 2026-10-04

The rest of the review (#3084), fixed:

- The ledger's write-ahead log (`ledger.db-wal`) and shared-memory index (`ledger.db-shm`) are
  kept 0600 like the database. SQLite's driver created them 0644, so the latest rows were
  readable by other users while the ledger was open; files an older version left are
  tightened when the ledger opens.
- A `pmr cloud send` stopped before the CLI answered (the timeout, Ctrl-C) says the message
  may have been queued, instead of saying it was not: sending again could run the billed work
  twice. A child the CLI left holding its output no longer holds the command past 2 seconds.
- A labelled prompt whose class is misspelled is an error naming the file and line, instead
  of being learned as "abstain". `abstain` and an empty class still mean no class.
- When the hook cannot apply an inject it recorded (the tool input is not an object), it
  records why, instead of leaving the ledger saying the model was changed.
- `pmr eval` and `pmr explain` say which artifact they routed with. A local artifact that
  cannot be used fails the eval gate, instead of passing on the embedded artifact.
- An artifact with a temperature at or below 0, a τ that is not a number, or a calibration
  outside [0, 1] is not used: the risk guards would have let downgrades through.

Fixes found by a review of the layers (layered architecture with DDD, SOLID, design patterns):

- Routing no longer sends a call to a tier the ledger shows failing. When the target tier's
  record is poor and no tier above it has the evidence to take the call (none was observed,
  or every tier up to the ceiling fails too), the call is held on the session's tier with the
  reason `posterior-hold`. It used to go to the failing tier.
- A probability that is not a number no longer passes the risk threshold: it holds.
- Subagents launched in the background are no longer learned as failures that used no tokens:
  their result arrives when they start, without an outcome.
- `pmr train` without `--output` no longer writes, over the artifact routing uses, one that
  cannot route (too few labelled prompts, or the embedded set alone); it says so and writes
  nothing. With `--output` it writes as before.
- Settings that silently turned a guard off are rejected: `runtime.workflow_step_limit` below
  the 10 stages of the routing workflow (every route stopped), `classifier.class_cap = 0`
  (every prompt abstained), a `training.tau_grid` or `temperature_grid` without a positive
  step, and a non-positive `eval.relative_cost`.
- A command whose use case cannot be built (the ledger cannot be opened) fails with the error
  instead of a panic and a stack trace.
- Inside: the dependency injection is `go.uber.org/dig` instead of `samber/do`. The command
  line no longer resolves use cases from a container: it declares what it needs
  (`cli.Scope`, an abstract factory) and each command is handed only its use case. A use case
  built with a port left nil fails, naming the field, when it is built. The reasons a guard
  holds a call are named once in the model (`Decision.Held`, `Decision.Classified`), and the
  Beta prior once (`model.NewBeta`, `Beta.Observations`).

## 1.1.1 — 2026-10-03

- The release of 1.1.0 stopped at the coverage gate (99.7% against the required 100%), so
  nothing was published; 1.1.1 is the first release with `pmr cloud`. The missing tests cover
  the relay's failure paths (unreadable CLI output, an unreadable or unwritable state file,
  an unreadable standard input) and `pmr cloud wait` finishing.
- `pmr cloud wait` returns at once when it is cancelled, even with a zero interval.

## 1.1.0 — 2026-10-03 (tagged, not published)

- New: `pmr cloud` and `/promari-model-router:cloud` relay messages from this terminal to a
  Claude Code cloud session and print its reply here, so the work runs (and is billed) in
  the cloud. Sending goes through `claude -p --cloud`, with the message on standard input;
  reading goes through Claude Code's RemoteTrigger tool. pmr holds no Claude.ai credential.
- `[cloud]` settings (`claude_bin`, `send_timeout_ms`, `link_file`, `poll_interval_seconds`,
  `max_polls`) are read from the user file only.
- The Go module is `promari-model-router`: import paths no longer carry the owner's name.

## 1.0.3 — 2026-10-02

- Fix: `bin/pmr` builds the binary with Go when the release has no binary for the platform
  (1.0.2 shipped `linux_amd64` only) or the download fails. Before, a plugin with `checksums.txt`
  only tried the download, so on macOS the hooks silently did nothing and retried every five
  minutes. A checksum mismatch still stops without building, and stays in `launcher_error`.
- The release also ships `darwin_arm64`.
- `tools/test-launcher.sh` checks every way the launcher provides a binary; `task ci` runs it.

## 1.0.2 — 2026-10-01

The first published release. It is released to promari-toolkit (files, tag and GitHub Release) but
not listed in the toolkit marketplace yet, so install it from a clone (see the README).
1.0.0 and 1.0.1 were tagged but never published (1.0.0 ran out of
disk space while building, 1.0.1 could not authenticate to promari-toolkit), so the entries below
describe the contents of this release as well.

- Publishing to promari-toolkit uses a deploy key limited to that repository instead of a personal
  access token; toolkit's own workflow creates the GitHub Release.

## 1.0.1 — 2026-10-01 (not published)

- Release binaries are built only for the targets given by the repository variable
  `PMR_RELEASE_TARGETS` (or the `targets` input of a manual run); this release ships `linux_amd64`.
  A platform without a release binary gets no download; `bin/pmr` then does nothing, or builds the
  binary when Go is installed.
- The SBOM attached to the release describes the shipped binaries (read from the dependency list Go
  embeds in them) instead of the source tree.
- A stable version merged into develop is tagged and released automatically, and the released
  commit is then merged into main.

## 1.0.0 — 2026-10-01

First release. It includes the hardening from the review before release;
the development versions 1.0.1-dev and 1.0.2-dev were never published.

- Hooks for `SessionStart`, `UserPromptSubmit`, `PreToolUse` / `PostToolUse` (Agent) and
  `PostModelSwitch`; subagent models are rewritten through `updatedInput` without a
  `permissionDecision`.
- Routing state graph: guard, route tag, bilingual rule stage (longest match, character
  lengths, abstention), learned stage (softmax regression, temperature scaling,
  split-conformal sets, out-of-distribution checks, conformal risk control per length
  bucket), safety floors, ledger posteriors, session-model cap.
- Session model resolved from the transcript first; values from `ANTHROPIC_MODEL` or
  settings never cap a route.
- Fixed-tier agents `scout`, `worker`, `engineer`, `architect`; skills `model-routing`
  and `codex-delegate`; commands `report`, `doctor`, `classify`.
- Ledger on SQLite (GORM, pure Go) with a SHA-256 hash chain; `pmr verify`, read-only
  `pmr query`.
- `pmr train` / `pmr eval` with baselines, collapse, granularity and Triage gates.
- `pmr serve` (OpenAPI-validated, loopback only) and `pmr mcp` (MCP stdio server).
- Every tunable value in `data/defaults.toml`; strict layered overrides, including
  `lexicon_extra` names (a misspelt class rejects the file instead of matching nothing).
- The learned stages (cascade, Triage gate, ledger posteriors) run only with a trusted
  local artifact; `pmr train` without `--file` records the artifact as embedded.
- A continuation prompt still gets the safety note when it names risky work.
- `pmr eval` evaluates the routing policy with `mode = "enforce"` regardless of overrides.
- Coverage counts the E2E binary (`-cover` + covdata); the release signs `checksums.txt`
  with cosign (keyless) and attaches the Sigstore bundle.

Hardening before release:

- Tests: every Test function is table-driven (checked by `internal/testpolicy` with
  go/ast), and statement coverage is 100% (unit tests plus the instrumented E2E
  binary; only generated code is excluded). The coverage gate is now 100%.
- Review fixes: concurrent hook appends no longer drop ledger rows (`_txlock=immediate`);
  the hash chain (v2, `hash_version`) covers every column; SSE follows the row position
  instead of timestamps; failures that were discarded are recorded (`last_error`,
  `launcher_error`, doctor); settings are range-checked and the project file is read
  through an allow-list; the HTTP API validates `Host`.
- Tests: mutation testing kills every non-equivalent mutant (108/108; 57.4% before);
  every test table has at least two cases (checked by `internal/testpolicy`).
- SOLID: use cases reach files, environment variables and embedded data only through
  ports (`CaseSource`, `AgentSource`, `EnvReader`, `ArtifactWriter`, `LedgerQuery`);
  the ledger and configuration ports are split by use (`LedgerWriter`, `LedgerReader`,
  `PromptHistory`, `LedgerMaintainer`, `LedgerVerifier`; `SettingsProvider`,
  `LexiconProvider`, `TierProvider`, `PriceProvider`, `ConfigDiagnostics`); the artifact
  output writer no longer pretends to load; `usecase/tools.go` and `cli/root.go` are
  split per use case and per command. depguard forbids `os` and the root package in
  the application layer and infrastructure imports in the interfaces layer.
  No change in CLI, hook or HTTP output.
- Fix: `SessionRepo.Save` kept the injected clock's time; GORM had overwritten
  `updated_at` with the wall clock on upsert.
- Fix: `/v1/report` and `/v1/explain` dropped fields (`from`, `to`, `prices_as_of`,
  `estimated_cost_usd`, `scores`, `continuation`) from their JSON responses.
