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
| 🧠 Context | Usage bar, tokens used and left, time until the context is full at the session's pace |
| ⚡ Claude | The 5-hour and 7-day windows with the time to their reset, the pace against the window, a forecast when the usage would run out before the reset, the spend limit |
| 🤖 Codex | Codex's usage windows and balance, read from its newest session log |
| 💰 Cost | Session cost; with `ccusage`: today, the billing block, the burn rate and an estimate for the block |
| 🔥 Burn, 📈 KPI, 🚀 Perf | Wall and API time, active time and streak, cost per turn and per line, lines changed and per hour, focus, parallelism, throughput, tool error rate |
| 📦 Cache, 📊 Tokens | Prompt cache hit ratio, what it saves and the time until it expires, tokens in and out, compactions, the surcharge above 200k |
| 🔧 Work | The session's to-do list, tool calls by tool |
| 🌿 Git | Branch, changed files, ahead and behind, stashes, time since the last commit |
| 🔀 PR | With `gh`: the pull request of the branch, its checks and its review |
| 🔖 Session, 🧭 Env | Session name, model, effort, thinking and fast mode, project, output style |
| 💻 System, 🧾 Meta | Clock, terminal uptime, load, memory, disk, battery, Claude Code processes, account, version and whether a newer one is released |
| 🎵 Music | The song that is playing (macOS, with `nowplaying-cli`) |

A chip is shown only when its value is known. A number that would be a guess (a window Codex
did not report, a rate over a few minutes of activity) is left out, not shown as zero.

A warning (the context or a rate limit at 90 % or more, a forecast that runs out before the
reset, a battery below 20 %, a major incident) blinks: its chip alternates every second
between its colour and a red band. The blink is made by drawing the line differently on odd seconds, not by the terminal's blink
attribute, which many terminals ignore.

## Install

The plugin is released in promari-toolkit but not listed in its marketplace yet, so
`claude plugin install promari-statusline@promari-toolkit` does not find it. Load it from a clone:

```bash
git clone https://github.com/tamito0201/promari-toolkit
claude --plugin-dir promari-toolkit/plugins/promari-statusline
```

Then, in Claude Code:

```text
/promari-statusline:setup
```

A plugin cannot declare a status line, so `setup` installs it. It copies the binary to
`~/.claude/promari-statusline/psl` and sets `statusLine` in `~/.claude/settings.json` to
`~/.claude/promari-statusline/psl render`. The settings file keeps its order and every other
key, and a copy of it as it was is left next to it (`settings.json.bak-<time>`). `psl setup
--dry-run` shows what would change without changing it. A settings file that is not valid
JSON is left untouched.

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
| `~/.cache/promari-statusline/last-input.json`, `width.txt` | What the last render received and the width it planned for; the first things to look at when a chip is missing or a line is cut |
| `~/.cache/claude-rate-limits.json`, `~/.cache/codex-rate-statusline.json` | The plan usage, for other tools (see below) |

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
does not fit even then is wrapped at a chip, and its continuation lines are numbered.
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
