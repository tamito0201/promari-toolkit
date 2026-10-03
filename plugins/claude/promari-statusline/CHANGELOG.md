# Changelog

## 1.4.0 — 2026-10-03

- Everything Claude Code reports that was not read yet: the spend limit's dollars and period,
  the prompt cache's lifetime, warmth, writes, rebuilds, misses by cause and the time of the
  last miss (and "Caching off" when no response reported cache tokens), the worktree, the vim
  mode, the agent, the directory the session moved to, added directories, and the pull request
  Claude Code found (shown when `gh` finds none, and for GitLab merge requests).
- The session's transcript is read for the whole session: tokens with the cached and thinking
  shares, requests, subagent requests, the models that answered, turn times (last, median,
  90th percentile), thinking time, prompts, interrupts, refused tool calls, refusals, responses
  cut at the output limit, queued prompts, web searches and fetches, files edited, hooks run and
  failed, editor diagnostics, the permission mode, and the compactions with the time since the
  last. A new `🤝 Agent` category shows how the agent and the human work together: tool calls
  per prompt and interventions per prompt. The transcript is read incrementally and remembered
  per transcript, so sessions running side by side no longer read each other's from the start.
- `🌿 Git` shows staged, new and conflicted files, an operation in progress, the lines changed
  against HEAD and the commits of today; `🔀 PR` shows a draft, conflicts, the size and the age.
  Sizes above 400 lines are yellow and above 1,000 red, after the code review studies at Cisco
  (SmartBear) and Google.
- `📈 KPI` shows the net lines, deep work (streaks of 23 minutes or more, after the time it
  takes to resume interrupted work in Mark et al., CHI 2008), the longest streak and the breaks.
- Fix: the cause of the last cache miss was never shown. Claude Code reports it as an object
  with a list of causes, and it was read as a string.
- Fix: `Thruput` and `📊 Tokens In/Out` divided and showed the token counts of the last request
  as if they were totals of the session. They are replaced by the totals from the transcript.


## 1.3.0 — 2026-10-03

- `psl setup --global` puts the status line on every terminal. A project's own settings take
  precedence over the user's, so a project whose `.claude/settings.json` sets another status line
  kept showing it. The global setup also writes the status line into the personal
  `.claude/settings.local.json` of each such project (the projects Claude Code has opened, from
  `.claude.json`), and keeps that file out of git through the repository's own
  `.git/info/exclude` when it is not ignored already. A project's shared settings are never
  changed. `--dry-run` shows what it would do.
- `psl doctor` names the projects that show another status line.
- `psl uninstall` also takes the status line out of the projects' personal settings, so that no
  project is left running a binary that is gone.

## 1.2.1 — 2026-10-03

- The release of 1.2.0, which was stopped before it was published: its tests named a
  project after an organisation, and the check for private information refused to publish
  them. The tests use a neutral name now; the code is the same as 1.2.0.

## 1.2.0 — 2026-10-03 (not published; use 1.2.1)

- `👥 Sessions` shows the other sessions running on this machine, one chip each (name or
  project, branch, context, cost, and how long it has been idle), the same on every terminal.
  Only sessions of the same account are shown together; a session of another configuration
  directory (`CLAUDE_CONFIG_DIR`) keeps to its own.
- `psl setup` sets `refreshInterval: 5`, so an idle session's status line follows the others.
  `psl doctor` warns when it is missing; run `psl setup` again after updating.
- Fix: the rate limits remembered for the first render of a session, and the history the
  forecasts are made from, are kept apart for each account. A session could show the limits
  another account had last seen.
- Fix: the account is read from the session's configuration directory (`CLAUDE_CONFIG_DIR`)
  and again as soon as its login file changes. It was read from `~/.claude.json` only and kept
  for an hour, so it lagged behind a `/login`.

## 1.1.1 — 2026-10-03

- Fix: `Tools` and `ErrRate` count tool calls only. Every `"name"` in the transcript was
  counted, so a git remote showed up as a tool (`origin52`) and the total was too high: 502
  for 380 calls in the session this was found in, which also made the error rate too low
  (2.2 % for 2.9 %).
- The tools of an MCP server whose names have digits, dots or hyphens are counted.

## 1.1.0 — 2026-10-03

- A category a few cells too wide for the terminal is packed onto one line (its chips stand
  closer, `│` for ` │ `) before it is broken into numbered lines. In a 76-cell terminal the
  session this was measured on went from four continuation lines to none.
- The pull request is a category of its own (`🔀 PR`), so a long branch name no longer pushes
  Git onto a second line. `CacheSave` moved from Perf to Cache, where it is `Save`.
- `$/Line` and `$/Turn` show the digits their size needs (37.1, 2.44, 0.13, 0.0035). A small
  cost per line was rounded to 0.00.
- `psl doctor` says how to install an optional tool that is missing.

## 1.0.1 — 2026-10-03

- Fix: the release ships `checksums.txt`. 1.0.0 was published without it (the plugin's
  `.gitignore` listed the file, and it applied in promari-toolkit as well), so `bin/psl` could not
  download the release binary and only worked where Go was installed to build one.
- The binaries are the same code as 1.0.0.

## 1.0.0 — 2026-10-03 (published without checksums.txt; use 1.0.1)

The first release. It is released to promari-toolkit (files, tag and GitHub Release) but not
listed in the toolkit marketplace yet, so install it from a clone (see the README).

- `psl render` draws the status line: context, Claude and Codex rate limits with pace and
  forecast, cost, KPIs, tokens, to-dos and tools, git and the pull request, the environment,
  the machine and the song that is playing, laid out in display cells to the width of the
  terminal. A warning blinks by alternating colours every second.
- `psl setup` copies the binary to `~/.claude/promari-statusline/psl` and sets `statusLine`
  in `~/.claude/settings.json`, keeping the file's order and a backup; `--dry-run` shows the
  change. `psl uninstall` reverses it. `psl doctor` checks the installation.
- A `SessionStart` hook keeps the installed copy at the plugin's version.
- The plan usage is written to `~/.cache/claude-rate-limits.json` and
  `~/.cache/codex-rate-statusline.json` for tools that run as hooks.
- The release ships `linux_amd64` and `darwin_arm64`; `bin/psl` builds the binary with Go on
  other platforms.
