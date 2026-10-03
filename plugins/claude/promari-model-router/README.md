<!-- generated: published from promari-portal; edit the source there -->

# promari-model-router

A Claude Code plugin that routes subagents to **haiku / sonnet / opus by the kind of work**,
works on **Japanese and English** briefs, keeps safety-sensitive work on strong models,
suggests **Codex** for bulk or second-opinion work, and records every decision so you can
measure whether it helps.

It runs inside Claude Code as hooks. It never proxies traffic, never touches your Claude
credentials, and never calls a model to classify (no `claude -p` from a hook).

## What it does

| Hook | What happens |
|---|---|
| `SessionStart` | Records the session model and injects a short routing policy asking the main model to start built-in subagent briefs with a `[route: <class>]` tag. |
| `PreToolUse` (Agent) | Decides the subagent model and rewrites only `model` in the tool input (no `permissionDecision`, so your permission rules still apply). Asks you before any subagent runs above the session model. |
| `UserPromptSubmit` | Adds short advice (never blocks): keep safety-sensitive work strong, delegate lookups, use Codex for review or bulk work, watch plan usage. |
| `PostToolUse` (Agent) | Records the model that actually ran (`resolvedModel`) and its tokens, joined to the decision by `tool_use_id`. |
| `PostModelSwitch` | Follows `/model` changes. |

## How a decision is made

The decision is a small state graph; every stage is a pure function and the ledger keeps
the trace of each run.

1. **Guard** — `mode = "off"`, `CLAUDE_CODE_SUBAGENT_MODEL_FORCE`, an explicit `model`
   (respected; an upgrade above the session asks you), custom agents (their frontmatter wins).
2. **Route tag** — a leading `[route: lookup|mechanical|standard|complex|architecture]`
   written by the main model decides the class in any language.
3. **Rule stage** — bilingual cue phrases, longest match first, scored per class; no word
   splitting, so Japanese works. Low confidence or a thin margin abstains.
4. **Learned stage** (only with a local artifact you trained) — character n-gram softmax
   regression with temperature scaling, a split-conformal prediction set (abstain when it
   spans tiers), out-of-distribution checks, and a risk-controlled threshold per length bucket.
5. **Floors** — never downgrade safety-sensitive work, context-dependent briefs, or a brief
   already retried in this session.
6. **Ledger posteriors** — Beta posteriors of observed success per class, length and tier.
7. **Finish** — never above the session model; never above opus automatically.

Methods and their sources are listed in [docs/methods.md](docs/methods.md).

## Install

The plugin is released in promari-toolkit but not listed in its marketplace yet, so
`claude plugin install promari-model-router@promari-toolkit` does not find it. Load it from a clone:

```bash
git clone https://github.com/tamito0201/promari-toolkit
claude --plugin-dir promari-toolkit/plugins/claude/promari-model-router
```

The first hook call downloads the binary for your platform from the release and installs it
only if its SHA-256 matches `checksums.txt`. When the release has no binary for your platform
or the download fails, it builds one with Go instead (Go must be installed; the toolchain the
module asks for is fetched automatically). A checksum mismatch is never built over. Until a
binary is ready hooks do nothing; `pmr doctor` shows why.

## Configure

Every tunable value is in [`data/defaults.toml`](data/defaults.toml). Override keys in
`~/.claude/promari-model-router.toml` or `<project>/.claude/promari-model-router.toml`
(project wins). Unknown keys reject the file; `pmr doctor` shows why.

```toml
[routing]
mode = "shadow"          # record what would change, change nothing

[lexicon_extra.mechanical]
strong = ["表記ゆれを直して"]

[lexicon_extra.danger]
items = ["給与計算"]
```

## Commands

| Command | Purpose |
|---|---|
| `pmr explain "<brief>"` | Show every stage of a decision |
| `pmr report [--days N]` | Classification rate per language, routed calls, tokens by the model that ran |
| `pmr doctor` | Conflicting settings, usage caches, agent tiers, ledger chain, recent hook errors |
| `pmr eval` | Evaluation gate (accuracy, harmful downgrades, danger leaks, baselines) |
| `pmr train --file <labelled.jsonl> [--ledger-days N]` | Train a local artifact from your own labelled prompts |
| `pmr query "SELECT ..."` / `--schema` | Read-only SQL on the ledger |
| `pmr verify` | Verify the ledger's hash chain |
| `pmr cost` | Price one subagent run on every tier |
| `pmr serve` | Read-only HTTP API on loopback ([OpenAPI](openapi/openapi.yaml)) |
| `pmr mcp` | MCP server (stdio): `explain_route`, `routing_report`, `route_policy` |
| `pmr cloud use <session-id\|url>` / `send` / `status` / `wait` | Relay messages to a Claude Code cloud session (see below) |

### Relay to a cloud session

`/promari-model-router:cloud <message>` sends the message to the cloud session set with
`pmr cloud use`, then prints the session's reply in this terminal. The work runs, and is
billed, in the cloud session; this terminal only carries text both ways.

- Sending runs `claude -p --cloud <id> --output-format json` with the message on standard
  input. Reading uses Claude Code's RemoteTrigger tool, which adds the token inside Claude
  Code. pmr never reads, stores or forwards a Claude.ai credential.
- Each read of the reply costs this local session one log page (about ten thousand tokens),
  so `/cloud` reads at most `max_polls` times, `poll_interval_seconds` apart. Run it with no
  message to read the reply again later.
- The cloud session cannot reach this machine's files or programs.
- `[cloud]` is read from `~/.claude/promari-model-router.toml` only: a project file may not
  name the program pmr runs.

## Measured

- Hook latency: median 23.1 ms (p90 26.3 ms) for `PreToolUse` on Apple Silicon.
- A learned classifier trained only on synthetic prompts raised harmful downgrades on real
  held-out prompts, so the embedded artifact never drives routing; train a local one.

## Development

```bash
go tool -modfile=tools/go.mod task          # lint, tests (unit + runn E2E), eval, security
go tool -modfile=tools/go.mod task build    # snapshot binaries for every platform
```

Go 1.27. Layered architecture with DDD (domain / application / infrastructure /
interfaces), GORM on pure-Go SQLite, Cobra, samber/do, the official MCP Go SDK, and an
oapi-codegen server validated against its OpenAPI contract.

## License

MIT.
