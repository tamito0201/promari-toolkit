# Changelog

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
