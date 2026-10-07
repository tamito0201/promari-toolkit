# Promari Toolkit

A collection of reusable plugins and tools for websites and development workflows.
Each component owns its documentation, tests, changelog, and release version.
The repository has no shared product version.

## Components

| Component | Purpose | Location | Release tags |
|---|---|---|---|
| [Promari SNS Share](plugins/wordpress/promari-sns-share/README.md) | Social sharing for Web Components and WordPress | `plugins/wordpress/promari-sns-share/` | `promari-sns-share-v4.2.2` |
| [Promari Model Router](plugins/claude/promari-model-router/README.md) | Route Claude Code subagents to haiku/sonnet/opus by task difficulty (Japanese and English), keep safety-sensitive work on strong models, suggest Codex for bulk or second-opinion work, and log every decision | `plugins/claude/promari-model-router/` | `promari-model-router-v1.2.0` |
| [Promari Statusline](plugins/claude/promari-statusline/README.md) | A multi-line status line for Claude Code: context, rate limits with pace and forecast, cost, KPIs, quality of the work, git and pull request, tools and system at a glance, laid out to the width of your terminal | `plugins/claude/promari-statusline/` | `promari-statusline-v1.15.0` |

Plugins live under `plugins/<kind>/` (`claude/` for Claude Code, `wordpress/` for WordPress). Standalone tools will live under `tools/`
when they are added; plugin-specific build tools stay with their plugin.

## Use Promari SNS Share from the CDN

```html
<script type="module" src="https://cdn.jsdelivr.net/gh/tamito0201/promari-toolkit@promari-sns-share-v4.2.2/plugins/wordpress/promari-sns-share/dist/promari-sns-share.min.js"></script>
<promari-sns-share></promari-sns-share>
```

Pin the component's complete release tag. Updating another component does not
change this URL. WordPress plugins are also distributed as component-specific
ZIP assets on [GitHub Releases](https://github.com/tamito0201/promari-toolkit/releases).

See the [plugin README](plugins/wordpress/promari-sns-share/README.md) for configuration,
analytics, integration, and development instructions.

## Use the Claude Code plugins

Promari Statusline and Promari Model Router are released here but not listed in this
repository's marketplace yet, so `claude plugin install <plugin>@promari-toolkit` does not find
them. Load them from a clone:

```bash
git clone https://github.com/tamito0201/promari-toolkit
claude --plugin-dir promari-toolkit/plugins/claude/promari-statusline \
       --plugin-dir promari-toolkit/plugins/claude/promari-model-router
```

Each plugin fetches its binary for your platform from its GitHub Release and installs it only
if it matches the release's `checksums.txt`; when the release has none for your platform, it
builds one with Go instead.

### Promari Statusline

Run `/promari-statusline:setup` once in Claude Code: a plugin cannot declare a status line, so
`setup` copies the binary to `~/.claude/promari-statusline/psl` and points `statusLine` in
`~/.claude/settings.json` at it. `psl doctor` checks the installation.

The status line is laid out in categories, each shown only while it has something to say:

| Category | What it measures |
|---|---|
| Context, rate limits, cost | The context window, the 5-hour and 7-day limits with their pace and a forecast of when they would run out, the session's cost and burn rate |
| Work and the agent | Active and deep-work time, turn times, the to-do list, tool calls, autonomy and interventions, subagents and the models that answered |
| 🧪 Quality, 🧬 Trace | The tests and builds the session ran, judged from their output and not only their exit status, how long they stayed red and how long their repair took, claims the last run contradicts, repeated calls; how the agent reads, searches and edits |
| ⏰ Due | The work owed in the repository: issues past their due day, urgent issues open for 48 hours, a failing default branch, what falls due this week, stale issues and pull requests, and the median lead time of the last two weeks |
| 🎓 Habits, 📏 Rules | A team's repository habits (branch names and age, unpushed commits, commit size and pace, merged and old branches, a stale fetch) and coding rules checked on the lines the working tree adds (SQL built from strings, credentials in code, empty catch blocks and more) |
| 🌿 Git, 🔀 PR | The working tree, the pull request of the branch with its checks, their duration, its review and review rounds |
| Sessions and system | The other sessions running on the machine, the machine itself, and the song that is playing |

Where a threshold comes from research or from a book on measurement, the source is named beside
the code; see the plugin's [README](plugins/claude/promari-statusline/README.md) for every chip.

### Promari Model Router

Its hooks route Claude Code's subagents to haiku, sonnet or opus by the difficulty of the task
and keep safety-sensitive work on strong models. `pmr doctor` checks the installation and
`pmr explain` shows how a request would be routed; see its
[README](plugins/claude/promari-model-router/README.md) to configure it.

## Development

Each component is self-contained: it has its own manifest, lockfile, and tool versions, and its
directory here is a published copy that changes only on a release. See each component's README
for its development commands; for example, Promari SNS Share uses Node.js from its `.node-version`,
pnpm, Python 3.13 or later, and PHP 8.1 or later:

```bash
cd plugins/wordpress/promari-sns-share
pnpm install --frozen-lockfile
export PYTHON=python3.13
pnpm run verify
```

The Claude Code plugins are Go modules with their own toolchain pins; each runs its checks
(lint, unit and end-to-end tests, coverage, security) with one command from its directory:

```bash
cd plugins/claude/promari-statusline
sh tools/run.sh task
```

## Independent releases

A component release uses `<component>-v<major>.<minor>.<patch>` as its Git tag.
Only that component's manifest and changelog are advanced. Git tags identify a
repository snapshot, while release notes and attached files describe one component.
The automatically generated source archive contains the whole repository; use
the component ZIP asset when installing a WordPress plugin.

Released tags are immutable. Each release is identified by its component tag.
Releases are published by a release workflow, which commits the component's
directory, creates its tag, and attaches the release files, `checksums.txt`, its
cosign signature, and an SBOM to the GitHub Release.
All documentation and commit messages are written in English.

## License

[MIT](LICENSE). Brand logos remain trademarks of their respective owners.
