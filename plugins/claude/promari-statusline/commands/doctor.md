---
description: Check the promari-statusline installation (settings, installed binary, optional tools, terminal width)
allowed-tools: Bash(psl doctor:*)
---

Run:

```bash
psl doctor
```

`psl` is this plugin's `bin/psl`, which Claude Code puts on the Bash `PATH` while the plugin
is enabled. If the shell reports `psl: command not found`, run
`"${CLAUDE_PLUGIN_ROOT}/bin/psl" doctor` instead.

Present the lines to the user. For each ❌ or ⚠️ line, explain what it means and the fix:

- `settings …: no statusLine`, or `installed binary …: not installed`: the status line was
  never installed; run `/promari-statusline:setup`.
- `statusLine runs another command`: another status line is configured. `psl setup` replaces
  it and names the previous command and the backup; ask before running it.
- `differs from the running binary`: the plugin was updated after the last setup. The next
  session start refreshes the copy; `psl setup` does it now.
- `git`, `gh`, `ccusage`, `nowplaying-cli` not found: optional. The line names the chips that
  are not shown without the tool and how to install it. Offer the command; run it only when the
  user agrees. A tool that is installed and still not found is missing from the `PATH` Claude
  Code was started with (a version manager that sets `PATH` per shell).
- `terminal width`: the width the layout plans for, and where the number came from. If lines
  are cut at the right edge, compare it with the real width of the terminal.
- `launcher` (download failed, checksum mismatch, or build failed): the binary could not be
  provided. Show the message and its time. A checksum mismatch means the downloaded file did
  not match the release; do not work around it by copying a binary in by hand. The launcher
  retries on its own after a few minutes.

If `psl doctor` itself fails with `no binary for <os>/<arch> yet`, the launcher is still
installing or has failed: show `${CLAUDE_PLUGIN_DATA}/launcher_error` if it exists
(line 1 is the time, the rest is the message).

When a chip is missing or wrong, two files in `~/.cache/promari-statusline/` show what the last
render saw: `last-input.json` (what Claude Code sent) and `width.txt` (the width it planned for).
