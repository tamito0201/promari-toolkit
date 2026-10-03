---
name: model-routing
description: Choose the model tier for a subagent or a piece of work (haiku, sonnet, opus) and decide whether to delegate at all. Use when you are about to start a subagent, when a task could be split between cheap and strong models, or when the user asks to save tokens or plan usage.
---

# Choosing a model tier

The router hook already assigns a model to built-in subagents started without
one. Use this guide when you choose explicitly.

## Decide whether to delegate first

A subagent starts with an empty context and pays its own start-up cost; measured
runs used 2.6 to 5.9 times the input tokens of doing the same work inline. Delegate
only when the work is large enough that keeping its intermediate output out of the
main conversation pays for that cost:

- many searches or file reads whose raw output you do not need to keep
- a batch of similar edits across many files
- an independent check or second opinion

Do the work yourself when it takes a few tool calls.

## Pick the tier by the kind of work

| Kind of work | Agent | Model |
|---|---|---|
| Find, count, list, summarise existing code or docs | `promari-model-router:scout` or `Explore` | haiku |
| Renames, formatting, version bumps, fixed-shape edits | `promari-model-router:worker` | sonnet (haiku if trivial) |
| Clear-spec implementation, tests for known behaviour | `promari-model-router:worker` | sonnet |
| Debugging, root cause, flaky or intermittent failures | `promari-model-router:engineer` | opus, high effort |
| Design, trade-offs, migrations, data models | `promari-model-router:architect` | opus, xhigh effort |

Rules that override the table:

1. Never pick a model above the session model without the user's consent; the hook asks when you do.
2. Keep authentication, payment, production data, migrations and secrets on opus or the session model.
3. When the task is ambiguous, keep the session model. A failed cheap attempt costs a retry.
4. Planning is not cheap work: a weak plan makes every later step worse.
5. Never switch the main conversation's model just to save tokens mid-task; the next
   request re-reads the whole history without cache.

## Escalating

When a cheaper agent reports that the task needs judgement, hand it to the next tier
with what was already tried (files read, hypotheses ruled out) so it does not start
from zero.

## Codex

For bulk work or an independent review, Codex is available if the user has it set up;
see the `codex-delegate` skill.
