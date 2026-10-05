# Changelog

## 1.12.0 — 2026-10-06

The order of the status line is fixed, one category per line:

- Every category starts a line of its own. Categories no longer share a line behind ` ┃ ` when
  they happen to fit, which moved a category up or down as the chips of its neighbours grew and
  shrank; the same category is now always in the same place. A category wider than the
  terminal still wraps under its own first line.
- The categories are ordered by priority and by how often they are looked at: alerts and
  forecasts, then the limits (`🧠 Context`, `⚡ Claude`, `🤖 Codex`), the money (`💰 Cost`,
  `🔥 Burn`), where the work stands (`🌿 Git`, `🔀 PR`, `🔧 Work`, `⏰ Due`, `🔖 Session`,
  `👥 Sessions`), how well it goes (`📈 KPI` … `🎓 Habits`), and last the surroundings
  (`🧭 Env`, `💻 System`, `🧾 Meta`, `🎵 Music`). `🌿 Git` and `⏰ Due` move up from below
  the metrics.

## 1.11.0 — 2026-10-05

Fixes found by a review of the layers (layered architecture with DDD, SOLID, dependency injection):

- Sessions side by side in other repositories or on other branches no longer make every render
  ask GitHub and git again: the remembered pull request, history, review queue, workload and
  account are kept per key, a file each, instead of one file that each session overwrote.
- An answer cut short when Claude Code stops a render is no longer remembered as "nothing": the
  `🔀 PR` chip stayed hidden for five minutes and today's commits read 0 for a minute.
- `psl uninstall` stops before removing the binary when a project's personal settings cannot be
  read, instead of skipping it and leaving that project running a binary that is gone; it prints
  what it changed and the backup even when it fails. `psl setup --global` and `psl uninstall`
  stop between projects on Ctrl-C.
- A rule added without its label can no longer stop the status line: the label table has one
  entry per rule, checked by a test. The `⏰ Due` and `🎓 Habits` labels are made from the
  windows they count (`/14d`, `>48h`), so they cannot go on naming an old window.
- A pull request whose checks report their times, a workload with merged pull requests, and a
  session whose tests went from red to green again are remembered again: each held a duration,
  which `encoding/json/v2` refuses to write, so the cache was never saved and every render asked
  `gh` again or read the transcript from its start (since 1.10.0). Durations are now written as
  text (`"1m30s"`).
- `ccusage` that is missing or hangs no longer holds every render up to its timeout: after two
  failures in a row it is not asked for 30 seconds, then for twice as long after each failure
  that follows, up to 10 minutes (a circuit breaker, its state in `breakers/spend.json`).
- A reader that panics loses its own chips only; the rest of the status line is shown.
- The last render's sources are written to `sources.json`: how long each took and whether it
  answered, had nothing, failed or panicked.
- The activity files of sessions that stopped (`sessions/<id>.json`) are removed after a week,
  when a new session starts.
- A use case built with a dependency missing stops at once and names it, instead of panicking at
  the first render that asks that source.
- A line as wide as the whole budget was cut by Claude Code, which draws the status line two
  cells indented and cuts a line that would touch the last column: at 66 columns a 64-cell
  packed line was shown as `Est $2…` (measured 2026-10-05). The margin is now three cells —
  the host's indent and the column it keeps free — so no planned line reaches the cut.
- A render Claude Code kills between creating and renaming a temporary file could never clean
  it up, and the leftovers piled up in the cache (41 `*.tmp*` files on one machine). The next
  write of the same target now sweeps its leftovers older than an hour, the roster sweeps those
  of dead sessions in `peers/`, and the prunes sweep those of keys never written again.
- The production screen of 2026-10-05 (26 lines, 21 categories, from `⏳ ETA 44m` to
  `+/- 62.4`) is replayed as a test: the view is the screen read backwards, the identities it
  satisfied (the last request's tokens sum to the context, the issue counts sum to their
  total) are asserted, and an end-to-end render at 66 columns keeps every line inside the 63
  cells the host showed.
- Inside: `psl doctor`'s words live in the command line, the use case reports what it found;
  the ports read and write apart (the doctor and a project's shared settings get the reading
  side only); the rules of a transcript (compactions, re-edits, masked failures) live in the
  model, the adapter only reads the file; a working tree embeds its branch's history instead of
  copying it field by field.

## 1.10.0 — 2026-10-03

More measures after the KPI books of 1.9.0, each beside the measures it belongs with:

- `🎓 Habits` shows `Old ×N 14d+` (local branches without a commit for two weeks: candidates to
  abandon), `Switch ×N today` (branch switches in the reflog: time lost to changes of plan) and
  `Fetched 2d ago` (yellow when the remote has not been fetched for a day, as the counts read
  from it are no fresher); `🧪 Quality` shows `Revert ×N today` and, once failing tests pass
  again, `green 45m (fixed in 12m)` (the time since the last quality failure and how long its
  repair took); `🔀 PR` shows how long the checks of the last push took (`CI ✅ 16 6m`) and
  `Rounds ×N`, the reviews that asked for changes (yellow from two); `📏 Rules` adds `No ///`,
  a public C# type or member without an XML documentation comment (the course's team exercise
  asks for one on every class and member).
- The test coverage is 100% and the gate is raised to it. The calls to the operating system
  whose failures a test cannot cause (a write to a file just created, the size of a file just
  opened) are variables a test of the package swaps; branches that could not be reached are
  gone.

## 1.9.0 — 2026-10-03

A new `⏰ Due` category shows the work the user owes in the repository, after three books on
KPIs and one on customer support (David Parmenter's "Key Performance Indicators", 2nd and 4th
editions; Bernie Smith's "KPI Checklists"; Nishtha Duggal's "Mastering Customer Support"). As
the books ask, it shows the exceptions only, keeps red for what needs action now, puts the
current and the future before the past, and writes a count beside every rate:

- Red: `Overdue ×N (#issue Nd)`, the assigned open issues whose milestone's due day has passed,
  with the latest one (Parmenter's most common weekly KPI is the list of late projects);
  `Urgent >48h ×N`, the issues labelled P0, P1, critical, urgent, blocker or incident open for
  more than 48 hours ("requests outstanding for more than 48 hours"); `Base ✗ <workflow> 40m`,
  a workflow whose last run on the default branch failed, since the first failure in a row
  (blinking after 25 minutes, with the number of other failing workflows).
- Yellow: `Due today ×N`, `My turn ×N` (own pull requests whose review asks for changes) and
  `Stale 14d+ ×N` (assigned issues and own pull requests without an update for two weeks:
  "still unresolved after two weeks").
- Grey: `Due 7d ×N` (the future measure), `Undated N/M` (assigned issues with no due day),
  `WIP N PR (N draft)`, and the last two weeks' flow: `Lead p50 6h ×N/14d` (the median time
  from opening to merging; the median, as "beware of averages"), `1st-pass N/M` (pull requests
  approved by their first review, yellow below half only from 10 reviewed: Smith asks for 10 to
  30 points before judging) and `Abandoned ×N/14d` (closed without merging).
- A list cut at the most gh is asked for (200 issues, 100 pull requests) shows its count with
  `+`, as a lower bound. The five questions to gh run at once and are remembered for five
  minutes.

## 1.8.1 — 2026-10-03

- The release of 1.8.0, which was stopped before it was published: a test wrote a token in a git
  remote (`user:token@` followed by GitHub's host), which the check for private information reads
  as an e-mail address. The test joins the URL from two strings now; the code is the same as
  1.8.0.

## 1.8.0 — 2026-10-03 (not published; use 1.8.1)

A new `📏 Rules` category checks the lines the working tree adds against the coding rules a
training course for new engineers teaches (C#, ASP.NET Core MVC, SQL Server, JavaScript and
HTML), the most serious first. Only rules that one added line, or a catch block within one hunk,
can decide are checked:

- Red and blinking, for what lets data leak, be lost or be attacked: `SQL+str` (an SQL statement
  interpolated, joined with `+` or formatted; a join beside `?`, `@name` or `$1` parameters joins
  only names and is not counted), `No WHERE` (an UPDATE or DELETE that ends without WHERE),
  `Cred in code` (a connection string, a password literal, a token in a git remote or a GitHub
  token; placeholders and tests are not counted), `throw ex` (which loses the stack trace) and
  `Empty catch` (a catch block with nothing in it; a comment or an exception named `ignored`
  says the error is ignored on purpose and is not counted).
- Yellow, for what hides a failure or breaks a stated rule: `Catch-all` (catching Exception, or
  anything, without throwing it on, in the languages whose catch names its type), `No CSRF` (a
  `[HttpPost]` without `[ValidateAntiForgeryToken]` outside an `[ApiController]`), `Html.Raw` of
  a value, `Singleton DB` (a DbContext registered as a singleton), `= NULL` in SQL, `JS var`,
  `<br><br>`, `Naming` (a type in lower case, an interface without `I`, an exception class without
  the `Exception` suffix or derived from ApplicationException, a bool named `...Flg`, a local named
  temp, data, info, str or buf) and `No {}` (an if, for or while without braces, in C#).
- Grey: `>120 col` (a line longer than 120 columns, a wide character counting as two) and
  `Tab indent` (C# is indented with four spaces).
- Bundles, vendored libraries, EF Core migrations and designer files are not checked. At most
  5,000 added lines are checked per render; the rest shows as `Unchecked N lines`, so that no
  break shown is not read as none.

`Tests … red` adds `→ ask` after 15 minutes: the course has a newcomer try alone for about 15
minutes before asking (and its team exercise says to ask after 30).

## 1.7.1 — 2026-10-03

- The release of 1.6.0 and 1.7.0, which were stopped before they were published: their tests
  wrote a real e-mail address as the co-author of a commit, and the check for private
  information refused to publish them. The tests use an address under example.com now; the code
  is the same as 1.7.0, which includes 1.6.0.

## 1.7.0 — 2026-10-03 (not published; use 1.7.1)

A new `🎓 Habits` category measures how a repository is worked with against the rules and the
numbers of a training course for new engineers, whose team exercise grades its trainees by their
git history:

- Where the work is done: `On main directly` blinks when the default branch (read from
  `origin/HEAD`) has changes or unpushed commits. A branch not named "kind/topic"
  (`feature/login-ui`, `fix/cart-bug`) shows `Name ✗` in a repository with a remote.
- How long: `Branch 5h00m` is the age of the branch's first commit of its own (yellow after
  3 hours, as a task is cut to two or three; red after a day), `Unpushed 26h00m` the age of the
  oldest commit not pushed (red and blinking after a day: push within the day),
  `Behind main ×12` the commits of the default branch not taken in yet.
- How the commits are made: lines per commit of today (green at 10 to 100, the course's standard
  being 20 to 50), commits per day (green at 2 to 5), the largest commit when above 100 lines, the
  lines added per line deleted, `Conv N/M` for a team that follows Conventional Commits, and
  `Vague msg ×N` for subjects such as "fix", "update" or "修正" that say nothing.
- What should not be there: `Conflict marks ×N` (conflict markers added to files, blinking),
  `Junk ×N` (build output, editor state, dependencies and `.env` files changed or untracked; red
  when staged), `Merged ×N left` (merged branches not deleted).
- `Streak Nd (max Md)`: the run of days with the user's commits (green from 3).
- The pull request: `No assignee`, `No reviewer`, `Review wait 2h00m` (no review after 30
  minutes), and `To review ×N (oldest …)`, the pull requests waiting for the user's review.

The history these come from (today's commits, the branch's age, the merged branches, the days
with commits) changes only with commits, so it is read at most once a minute; the questions git
is asked are asked at once instead of one after another. The status line takes no longer to draw
than 1.6.0 once the history is remembered.

## 1.6.0 — 2026-10-03 (not published; use 1.7.1)

Measures from the 2025-2026 studies of coding agents, each checked against the paper it comes
from and against the transcripts of real sessions before it was added.

- `🧪 Quality` adds:
  - `Claim≠`: the times the agent said the tests passed while the last run had failed or had not
    run since an edit, or said a problem was fixed while the last run had failed. Inaccurate
    self-reports were 22.58% of the developers' complaints in 20,574 sessions (Tang et al. 2026),
    and 26% of failed trajectories reported a success (Zhao et al. 2026). Only test runs the
    status line recognises count, so a claim made before any of them is not counted.
  - The tool calls made since the tests went red (`red 12m · 14 calls`): most failed
    trajectories went on without progress after the failure was certain (Zhao et al. 2026).
  - `Weaken`: skips added to tests and assertions removed from them, the marks of reward
    hacking (13.8% of the rollouts of SWE-Marathon, 2026).
  - `Mocks +N`: test doubles added (coding agents added mocks in 36% of their commits, people in
    26%: Hora and Robbes, MSR 2026); `Deps +N`: dependencies added to a manifest (19.6% of the
    packages sixteen models generated did not exist: Spracklen et al., USENIX Security 2025);
    `AI N/M today`: the commits of today co-authored by an AI, from the Co-authored-by trailer
    (Robbes et al. 2026).
- A new `🧬 Trace` category shows how the agent works through the session:
  - `Explore N/edit`: reads and searches per edit (passing trajectories browse more:
    Oderinwale 2026).
  - `Reread ×N`: reads and searches identical to one made since the last edit (in Claude Code
    they came up in 64-92% of the tasks and took 5-11% of their cost: Hu et al. 2026).
  - `EditRun max N`: the longest run of edits with no other call between, yellow from five
    (about 80% of the trajectories with such runs failed, against 59%: Oderinwale 2026).
  - `Obs ≈N (P% ctx) max M`: the tokens of the tools' output since the last compaction,
    estimated at four bytes a token (observations took 62-84% of an agent's context:
    Lindenbauer et al., NeurIPS 2025 workshop).
  - `Since compact N prompts` (multi-turn conversations answered 39% worse: Laban et al. 2025)
    and `PromptTok p50 · max (×N)`, the spread of the tokens per prompt (runs of the same task
    differed up to 30 times: Bai et al. 2026).
- Test runs started through a wrapper (`sh tools/run.sh task ci`), with variables set in front
  (`CI=true go test`), with `node --test` or a Node test file are recognised.
- The reading of a transcript moves to format 3 and is read again from the start once.

## 1.5.0 — 2026-10-03

- A new `🧪 Quality` category shows what the session's checks and the uncommitted change say
  about the quality of the work. From the transcript: the tests and the builds, type checks and
  lints the session ran (`Tests ×12 ✅ (fail 29% · piped 2)`), how long the tests have been red
  (blinking after 25 minutes, by which three quarters of the developers' test repairs were done
  in Beller et al., ESEC/FSE 2015), the source files edited since the tests last passed
  (`Untested`), failed edits with the failures in a row (after one failed edit, an agent's edit
  succeeded 57.2% of the time against 90.5%: SWE-agent, NeurIPS 2024), and tool calls repeated
  as they were. From git: the spread of the change (files, directories, top-level directories and
  the normalised entropy of its lines, the diffusion measures of just-in-time defect prediction:
  Kamei et al., TSE 2013; Hassan, ICSE 2009), the share of its lines in tests, the TODO, FIXME,
  HACK and XXX it adds and removes (self-admitted technical debt: Potdar and Shihab, ICSME 2014),
  and the commits of today that fix or revert.
- A check is judged from its output as well as its exit status. An agent pipes a test run
  through `tail` or `grep`, and the pipe exits with the status of its last command: in 359 test
  runs of real sessions, 87 printed failures and exited with 0. A run whose output was cut and
  whose exit status a pipe hid is shown as unknown (`?`), not as passed.
- The reading of a transcript carries a format version. A reading made by an earlier version is
  read again from the start once, so that what the new version counts is counted for the whole
  session and not only for what was written after the update.
- `git diff --shortstat` is replaced by one run of `git diff --numstat --patch`, which gives the
  lines per file and the debt markers together; `rev-list --count` by `log --format=%s`, which
  gives the subjects of today's commits. The status line starts no more git processes than before.

## 1.4.1 — 2026-10-03

- A section too wide for a line no longer repeats its title with a number on the next line
  (`📦 Cache 2`), which read as a section of its own. Its continuation lines carry no title and
  hang under the chips of its first line, behind the same `│`.
- The compactions (`compact ×N`, with the time since the last) move from `📊 Tokens` to
  `🧠 Context`, where they belong, and `📊 Tokens` wraps less often.


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
