---
name: codex-delegate
description: Hand work to OpenAI Codex through the codex plugin (openai/codex-plugin-cc) to spare the Claude plan - bulk mechanical changes, test generation for specified behaviour, independent code review, or a second diagnosis. Use when the router suggests Codex or the user asks to use Codex.
---

# Delegating to Codex

Codex runs on the user's own Codex (ChatGPT) plan. Delegating spares the Claude plan
but spends the Codex plan, so it moves usage rather than removing it.

## Before delegating

1. Check the plugin is ready: `/codex:setup`. If Codex is not installed or not logged
   in, tell the user and keep the work in Claude.
2. If the router said Codex usage is high, keep the work in Claude.
3. Do not send safety-sensitive work (authentication, payments, production data,
   migrations, secrets) without the user's explicit request.

## Which command

| Need | Command |
|---|---|
| Independent read-only review of the current diff | `/codex:review --background` |
| Challenge a design decision or a risky area | `/codex:adversarial-review --background <focus>` |
| Hand over a bounded task with clear success criteria | `/codex:rescue --background --model gpt-6-luna --effort medium <task>` |
| Hard diagnosis that stalled in Claude | `/codex:rescue --background <task>` (Codex default model) |

Notes:

- Without `--model`, `/codex:rescue` uses the Codex default from `~/.codex/config.toml`,
  which may be the most expensive model. Pass `--model gpt-6-luna` for routine work.
- codex-plugin-cc 1.0.6 accepts `--effort` values `none`, `minimal`, `low`, `medium`,
  `high`, `xhigh` only.
- `/codex:rescue` writes to the working tree by default. Review the diff before you
  build on it, and run the tests yourself.
- Check progress with `/codex:status` and fetch output with `/codex:result`.
- Do not enable the stop-time review gate casually: it can loop and drain both plans.

## Writing the task

Give Codex what it cannot infer: the goal, the files in scope, the acceptance check
(a test command or observable result), and anything it must not touch.
