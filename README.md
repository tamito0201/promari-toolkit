# Promari Toolkit

A collection of reusable plugins and tools for websites and development workflows.
Each component owns its documentation, tests, changelog, and release version.
The repository has no shared product version.

## Components

| Component | Purpose | Location | Release tags |
|---|---|---|---|
| [Promari SNS Share](plugins/wordpress/promari-sns-share/README.md) | Social sharing for Web Components and WordPress | `plugins/wordpress/promari-sns-share/` | `promari-sns-share-v4.2.3` |
| [Promari Model Router](plugins/claude/promari-model-router/README.md) | Route Claude Code subagents to haiku/sonnet/opus by task difficulty (Japanese and English), keep safety-sensitive work on strong models, suggest Codex for bulk or second-opinion work, and log every decision | `plugins/claude/promari-model-router/` | `promari-model-router-v1.2.0` |

Plugins live under `plugins/<kind>/` (`claude/` for Claude Code, `wordpress/` for WordPress). Standalone tools will live under `tools/`
when they are added; plugin-specific build tools stay with their plugin.

## Use Promari SNS Share from the CDN

```html
<script type="module" src="https://cdn.jsdelivr.net/gh/tamito0201/promari-toolkit@promari-sns-share-v4.2.3/plugins/wordpress/promari-sns-share/dist/promari-sns-share.min.js"></script>
<promari-sns-share></promari-sns-share>
```

Pin the component's complete release tag. Updating another component does not
change this URL. WordPress plugins are also distributed as component-specific
ZIP assets on [GitHub Releases](https://github.com/tamito0201/promari-toolkit/releases).

See the [plugin README](plugins/wordpress/promari-sns-share/README.md) for configuration,
analytics, integration, and development instructions.

## Use the Claude Code plugins

Promari Model Router is released here but not listed in this repository's marketplace yet,
so `claude plugin install <plugin>@promari-toolkit` does not find it. Load it from a clone:

```bash
git clone https://github.com/tamito0201/promari-toolkit
claude --plugin-dir promari-toolkit/plugins/claude/promari-model-router
```

The plugin fetches its binary for your platform from its GitHub Release and installs it only
if it matches the release's `checksums.txt`; when the release has none for your platform, it
builds one with Go instead.

### Promari Model Router

Its hooks route Claude Code's subagents to haiku, sonnet or opus by the difficulty of the task
and keep safety-sensitive work on strong models. `pmr doctor` checks the installation and
`pmr explain` shows how a request would be routed; see its
[README](plugins/claude/promari-model-router/README.md) to configure it.

## Development

Each component is self-contained: it has its own manifest, lockfile, and tool versions, and its
directory here is a published copy that changes only on a release. See each component's README
for its development commands; for example, Promari SNS Share uses Node.js from its `.node-version`,
pnpm, Python 3.13 or later, and PHP 8.1 or later:

```bash
cd plugins/wordpress/promari-sns-share
pnpm install --frozen-lockfile
export PYTHON=python3.13
pnpm run verify
```

The Claude Code plugins are Go modules with their own toolchain pins; each runs its checks
(lint, unit and end-to-end tests, coverage, security) with one command from its directory:

```bash
cd plugins/claude/promari-model-router
sh tools/run.sh task
```

## Independent releases

A component release uses `<component>-v<major>.<minor>.<patch>` as its Git tag.
Only that component's manifest and changelog are advanced. Git tags identify a
repository snapshot, while release notes and attached files describe one component.
The automatically generated source archive contains the whole repository; use
the component ZIP asset when installing a WordPress plugin.

Released tags are immutable. Each release is identified by its component tag.
Releases are published by a release workflow, which commits the component's
directory, creates its tag, and attaches the release files, `checksums.txt`, its
cosign signature, and an SBOM to the GitHub Release.
All documentation and commit messages are written in English.

## License

[MIT](LICENSE). Brand logos remain trademarks of their respective owners.
