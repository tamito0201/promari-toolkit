<!-- generated: published from promari-portal; edit the source there -->

# promari-statusline

A status line for Claude Code that shows, on a few lines laid out to the width of your
terminal, what you would otherwise look up: how full the context is, how much of the plan's
rate limits is used and whether it will last until the reset, what the session costs, what
git and the pull request look like, and how the machine is doing.

```text
🧠 Context │ ████░░░░░░ 42% 84k/200k 残 116k ⏳ ETA 1h56m │ ⚡ Claude 5h ░░░░░ 1% 🔄 4h40m 7d ████░ 73% 🔄 4d7h Pace ×1.9
💰 Cost │ Sess $12.50 │ Today $45.67 │ Blk $8.90 (残 2h15m) │ 🔥 Burn │ ⏰ 10m (API 5m) │ Active 8m
🌿 Git │ develop │ 📝 2 Files │ 🔽 16 Behind ┃ 🔀 PR │ #2996 CI ✅ 3 approved
🧭 Env │ Opus 5.5 │ Mode high·think │ 📂 promari │ 💻 System │ 🕐 04:09 │ CPU 3.9/10c │ 🔌 Bat 100%
```

It is one Go binary without a runtime to install. It reads the JSON Claude Code sends to a
status line, a few local files and the output of tools you already have. Its only requests
are two reads (the API's status page and the npm registry, for the newest version of Claude
Code); it sends none of your data.

## What it shows

| Section | Chips |
|---|---|
| 🚨 Alert | An incident on the API's status page (shown only while there is one) |
| 📉 Forecast | When a rate limit would run out, if that is before its reset |
| 🧠 Context | Usage bar, tokens used and left, time until the context is full at the session's pace, compactions and the time since the last |
| ⚡ Claude | The 5-hour and 7-day windows with the time to their reset, the pace against the window, a forecast when the usage would run out before the reset, the spend limit with its dollars and period |
| 🤖 Codex | Codex's usage windows and balance, read from its newest session log |
| 💰 Cost | Session cost; with `ccusage`: today, the billing block, the burn rate and an estimate for the block |
| 🔥 Burn, 📈 KPI, 🚀 Perf | Wall and API time, active time and streak, cost per turn and per line, lines changed (and net) and per hour, focus, deep work (streaks of 23 minutes or more), the longest streak and the breaks, parallelism, turn times (last, median, 90th percentile), thinking time, tool error rate |
| 📦 Cache, 📊 Tokens | Prompt cache hit ratio, lifetime (5m/1h), whether it went cold, the last miss with its cause and age, misses by cause, tokens written (and by misses), expected rebuilds, what it saves; tokens over the whole session (subagents included) with the cached share and the thinking share, requests, the last request's input split, the surcharge above 200k; "Caching off" when no response reported cache tokens |
| 🔧 Work | The session's to-do list, tool calls by tool, files edited, hooks run (and failed), editor diagnostics handed to the model |
| 🤝 Agent | Prompts typed, tool calls per prompt (autonomy), interventions (interrupts and refused tool calls per prompt), refusals, responses cut at the output limit, prompts queued while the agent worked, web searches and fetches, subagent requests, the models that answered |
| 🧪 Quality | The tests and the builds, type checks and lints the session ran, how the last ended and the share that failed, judged from their output as well as their exit status (a failure piped through `tail` exits with 0); how long the tests have been red; the source files edited since the tests last passed; failed edits and the failures in a row; tool calls repeated as they were; claims that the tests passed or a problem was fixed that the last run contradicts; the calls made since the tests went red. For the uncommitted change: how widely it is spread (files, directories, top-level directories and the entropy of its lines), the share of its lines in tests, the TODO, FIXME, HACK and XXX it adds and removes, skips added to tests and assertions removed, test doubles and dependencies added, and the commits of today that fix or revert or that an AI co-authored |
| 🧬 Trace | How the agent works through the session: reads and searches per edit, reads and searches repeated since the last edit, the longest run of edits without another call, the tokens of the tools' output since the last compaction (estimated) and its share of the context, the prompts since the last compaction, and the spread of the tokens per prompt |
| ⏰ Due | The work the user owes, after the KPI books: assigned issues past their milestone's due day (with the latest), urgent issues open for 48 hours, a failing workflow of the default branch, issues due today and within a week, own pull requests whose review asks for changes, issues and pull requests without an update for two weeks, issues with no due day, open pull requests, and the last two weeks' median lead time, first-pass approvals and abandoned pull requests. Nothing shows while nothing is owed |
| 📏 Rules | The coding rules of a training course for new engineers, checked on the lines the working tree adds: SQL built from strings, UPDATE or DELETE without WHERE, credentials in code, `throw ex`, empty catch blocks (these blink), catching everything without throwing on, a POST without an anti-forgery token, Html.Raw of a value, a singleton DbContext, `= NULL` in SQL, `var` in JavaScript, repeated `<br>`, names the rules forbid, if without braces in C#, lines over 120 columns and tabs in C#; lines past the 5,000 checked per render are shown as unchecked |
| 🎓 Habits | The habits of a team's repository, after a training course for new engineers: work on the default branch (blinking), a branch not named kind/topic, the branch's age and the age of the oldest unpushed commit (red after a day), commits behind the default branch, lines per commit and commits per day of today against the course's standards, the largest commit, Conventional Commits, vague commit messages, conflict markers and junk files, merged branches not deleted, the run of days with commits, a pull request without an assignee or a reviewer or waiting for review, and the pull requests waiting for the user's review |
| 🌿 Git | Branch, worktree, an operation in progress (rebase, merge, cherry-pick, revert, bisect), conflicts, changed, staged and new files, the lines changed against HEAD (yellow above 400, red above 1,000), ahead and behind, stashes, time since the last commit and the commits of today |
| 🔀 PR | With `gh`: the pull request of the branch, its checks, its review, draft, conflicts, size (coloured like the diff) and age. Without `gh` (or for a GitLab merge request): the number and review state Claude Code found |
| 🔖 Session, 🧭 Env | Session name, model, effort, thinking and fast mode, project, output style, permission mode, agent, vim mode, the directory the session moved to, added directories |
| 👥 Sessions | The other sessions of the same account running on this machine: their name or project, branch, context and cost, as their own status lines last showed them |
| 💻 System, 🧾 Meta | Clock, terminal uptime, load, memory, disk, battery, Claude Code processes, account, version and whether a newer one is released |
| 🎵 Music | The song that is playing (macOS, with `nowplaying-cli`) |

A chip is shown only when its value is known. A number that would be a guess (a window Codex
did not report, a rate over a few minutes of activity) is left out, not shown as zero.

A warning (the context or a rate limit at 90 % or more, a forecast that runs out before the
reset, a battery below 20 %, a major incident) blinks: its chip alternates every second
between its colour and a red band. The blink is made by drawing the line differently on odd seconds, not by the terminal's blink
attribute, which many terminals ignore.

Most of 🤝 Agent, 🧪 Quality, 🧬 Trace, 📊 Tokens over the session, the turn times and the files edited come from the
session's transcript. It is read once and then only from where the last read stopped, so a
transcript of tens of megabytes costs a fraction of a millisecond per render (169 ms for the
first read of a 29 MB transcript, 0.13 ms for the next). Where a chip's threshold comes from
research, the source is named in the code beside it; the list is in
[docs/statusline/README.md](../docs/statusline/README.md).

## Install

The plugin is released in promari-toolkit but not listed in its marketplace yet, so
`claude plugin install promari-statusline@promari-toolkit` does not find it. Load it from a clone:

```bash
git clone https://github.com/tamito0201/promari-toolkit
claude --plugin-dir promari-toolkit/plugins/claude/promari-statusline
```

Then, in Claude Code:

```text
/promari-statusline:setup
```

A plugin cannot declare a status line, so `setup` installs it. It copies the binary to
`~/.claude/promari-statusline/psl` and sets `statusLine` in `~/.claude/settings.json` to
`~/.claude/promari-statusline/psl render`, redrawn every 5 seconds (`refreshInterval`) so that an
idle session follows the others. The settings file keeps its order and every other
key, and a copy of it as it was is left next to it (`settings.json.bak-<time>`). `psl setup
--dry-run` shows what would change without changing it. A settings file that is not valid
JSON is left untouched.

### Every terminal: `--global`

A project's own settings take precedence over the user's: a project whose
`.claude/settings.json` sets another status line shows that one, whatever `setup` wrote.

```text
psl setup --global --dry-run   # what would change
psl setup --global
```

also writes the status line into the personal `.claude/settings.local.json` of every project
Claude Code has opened (listed in `.claude.json`) that shows another one. That file takes
precedence over the shared one and is not shared: when the repository does not ignore it already,
it is added to the repository's own `.git/info/exclude`, which is never committed. A project's
shared settings are never changed. `psl doctor` names the projects that still show another status
line, and `psl uninstall` takes the status line out of the projects' personal settings as well.

The copy has a path of its own because the plugin's directory changes with every version,
and the settings would point at a binary that is gone after an update. At the start of a
session a hook compares the copy with the plugin's binary and replaces it when they differ;
it does nothing for a user who never ran `setup`.

The first call of `psl` downloads the binary for your platform from the release and installs
it only if its SHA-256 matches `checksums.txt`. When the release has no binary for your
platform or the download fails, it builds one with Go instead (Go must be installed; the
toolchain the module asks for is fetched automatically). A checksum mismatch is never built
over. `psl doctor` shows why a binary is missing.

To remove it: `psl uninstall` takes `statusLine` out of the settings (only when it is this
plugin's) and deletes the copy.

## Commands

| Command | Purpose |
|---|---|
| `psl render` | Draw the status line from Claude Code's JSON on standard input (what the settings run) |
| `psl setup [--dry-run]` | Install the binary and point the settings at it |
| `psl uninstall` | Take the status line out of the settings and remove the binary |
| `psl doctor` | Check the settings, the installed binary, the optional tools and the terminal width |
| `psl version` | Print the version |

`/promari-statusline:setup` and `/promari-statusline:doctor` run the same commands from
inside Claude Code.

## Optional tools

Each is looked up on `PATH`; without it its chips are not shown, and nothing else changes.
`psl doctor` lists the ones that are missing, with how to install them.

| Tool | Chips | Install |
|---|---|---|
| `git` | 🌿 Git | <https://git-scm.com/downloads> |
| `gh` (signed in) | 🔀 PR | <https://cli.github.com> |
| [`ccusage`](https://github.com/ryoppippi/ccusage) | Today, Blk, $/h and Est in 💰 Cost | `npm install -g ccusage` |
| `nowplaying-cli` (macOS) | 🎵 Music | `brew install nowplaying-cli` |

The status line runs with the `PATH` Claude Code was started with. A tool installed by a
version manager that changes `PATH` per shell is found only if Claude Code was started from
such a shell.

## Files

| Path | What |
|---|---|
| `~/.claude/promari-statusline/psl` | The installed binary |
| `~/.cache/promari-statusline/` (or `$XDG_CACHE_HOME/promari-statusline/`) | What the status line remembers between renders: each session's activity, the rate limits last seen and their history, and the answers of slow sources |
| `~/.cache/promari-statusline/peers/` | Each running session's summary, posted on every render for the other status lines; a session whose Claude Code has exited is removed |
| `~/.cache/promari-statusline/accounts/` | The rate limits last seen and their history, apart for each account (named after a digest, not the address) |
| `~/.cache/promari-statusline/last-input.json`, `width.txt` | What the last render received and the width it planned for; the first things to look at when a chip is missing or a line is cut |
| `~/.cache/claude-rate-limits.json`, `~/.cache/codex-rate-statusline.json` | The plan usage, for other tools (see below) |

## All sessions, in step

Every status line posts its session (name, project, branch, context, cost) to a file of its
own and shows the other sessions under `👥 Sessions`, so every terminal shows the same
sessions. Only sessions of the same account are shown together: the account is the one signed
in to the session's configuration directory (`CLAUDE_CONFIG_DIR`, or `~/.claude`), so sessions
started with another configuration directory keep to their own. The rate limits remembered
between sessions are kept apart for each account in the same way, so a session never shows
another account's limits.

A session runs while its Claude Code runs: the status line notes the Claude Code process that
started it (past a shell that may stand between them) and leaves a session out as soon as that
process has exited. Where a process cannot be asked about (Windows), a session counts as running
for ten minutes after its last render.

Slow sources are asked once and remembered: the pull request and the status page for five
minutes, Codex's log for one, the tool counts of the transcript for thirty seconds, the
newest release for six hours. An answer of "nothing there" is remembered as well, so a
branch without a pull request does not start `gh` on every render.

Creating the file `~/.cache/promari-statusline/blink-demo` makes every warning chip blink, to
check how a warning looks in your terminal; deleting it turns that off.

### Plan usage for other tools

Claude Code tells the rate limits to the status line only; a hook is not told. So the status
line writes them where a tool that runs as a hook can read them. `promari-model-router` reads
both files to advise on plan usage.

```json
{"ts": 1790971649.05, "rl": {"five_hour": {"used_percentage": 29, "resets_at": 1790985000}, "seven_day": {"used_percentage": 80, "resets_at": 1791342000}}}
```

```json
{"rl": {"primary": {"used_percent": 100, "window_minutes": 10080, "resets_at": 1791055716}}, "mtime": 1790827230.99}
```

Times are Unix seconds. A window that is not known is left out.

## Width

A terminal cuts a line that is too long without saying so, so the layout is planned in
display cells (an emoji or a CJK character takes two) for the width in `COLUMNS`, which
Claude Code sets for a status line. Sections are put onto lines in a fixed order. A section
a few cells too wide for a line is packed (its chips stand closer, `│` for ` │ `); one that
does not fit even then is wrapped at a chip. Its continuation lines carry no title: they
hang under the chips of its first line, so every section is named once.
`psl doctor` prints the width that is used and where it came from.

## Development

```bash
sh tools/run.sh task          # lint, tests (unit + E2E of the built binary), launcher, security
sh tools/run.sh task build    # snapshot binaries for every platform
```

Go 1.27, two dependencies (`golang.org/x/term`, `golang.org/x/text`). Layered architecture
with DDD: `internal/domain` (the session, the chips, how they are composed and laid out; no
I/O), `internal/application` (the use cases, which know the domain and its ports),
`internal/infrastructure` (the adapters behind the ports: git, gh, files, the machine),
`internal/interfaces` (the command line and the ANSI presenter) and `internal/di` (the one
place that names them all). The direction of the imports is checked by `depguard` in
`.golangci.yml`.

## License

MIT.
