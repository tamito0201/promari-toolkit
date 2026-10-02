---
description: Install the promari-statusline status line into Claude Code's user settings
argument-hint: '[--dry-run]'
allowed-tools: Bash(psl setup:*), Bash(psl doctor:*)
---

A plugin cannot declare a status line, so this command installs it: it copies the plugin's
binary to `~/.claude/promari-statusline/psl` (a path that stays the same across plugin
versions) and sets `statusLine` in `~/.claude/settings.json` to run it.

1. Show what would change, and present it to the user:

   ```bash
   psl setup --dry-run
   ```

2. If the user's argument is `--dry-run`, stop here. Otherwise run:

   ```bash
   psl setup
   ```

3. Check the result:

   ```bash
   psl doctor
   ```

`psl` is this plugin's `bin/psl`, which Claude Code puts on the Bash `PATH` while the plugin
is enabled. If the shell reports `psl: command not found`, run
`"${CLAUDE_PLUGIN_ROOT}/bin/psl"` with the same arguments instead.

Tell the user:

- The settings file was backed up next to it (`settings.json.bak-<time>`) when it changed; the
  output names the backup and the command that ran before.
- The status line appears after the next message, or after restarting Claude Code.
- A `statusLine` in a project's `.claude/settings.json` or `.claude/settings.local.json` takes
  precedence over the user settings. If the old status line is still shown in one project,
  that is where to look; do not edit project settings without asking.
- `psl uninstall` takes the status line out again.

If `psl setup` fails because the settings file is not valid JSON, show the message and leave
the file alone; the user decides how to repair it.
