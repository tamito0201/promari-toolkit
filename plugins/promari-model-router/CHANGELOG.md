# Changelog

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
