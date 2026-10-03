---
description: Check promari-model-router's environment (conflicting env vars, usage caches, agent tiers, recent hook errors)
allowed-tools: Bash(pmr doctor:*)
---

Run:

```bash
pmr doctor
```

`pmr` is this plugin's `bin/pmr`, which Claude Code puts on the Bash `PATH` while the
plugin is enabled. If the shell reports `pmr: command not found`, run
`"${CLAUDE_PLUGIN_ROOT}/bin/pmr" doctor` instead.

Present the table to the user. For each ❌ or ⚠️ line, explain what it means and the fix:

- `CLAUDE_CODE_SUBAGENT_MODEL_FORCE` set: routing cannot work; unset it.
- `CLAUDE_CODE_EFFORT_LEVEL` set: the effort in agent definitions is ignored.
- No usage cache: usage-aware advice is off; it needs a status line that writes
  `~/.cache/claude-rate-limits.json`.
- Hook errors: show the last trace and suggest reporting it.
- Launcher error (download failed, checksum mismatch, or build failed): the binary could
  not be installed. Show the message and its time. A checksum mismatch means the
  downloaded file did not match the release; do not work around it by copying a binary in
  by hand. The launcher retries on its own after a few minutes.

If `pmr doctor` itself fails with `no binary for <os>/<arch> yet`, the launcher is still
installing or has failed: show `${CLAUDE_PLUGIN_DATA}/launcher_error` if it exists
(line 1 is the time, the rest is the message).
