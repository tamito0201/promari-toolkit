# Changelog

## 1.0.0 — 2026-10-03

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
