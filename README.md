# Promari Toolkit

A collection of reusable plugins and tools for websites and development workflows.
Each component owns its documentation, tests, changelog, and release version.
The repository has no shared product version.

## Components

| Component | Purpose | Location | Release tags |
|---|---|---|---|
| [Promari SNS Share](plugins/wordpress/promari-sns-share/README.md) | Social sharing for Web Components and WordPress | `plugins/wordpress/promari-sns-share/` | `promari-sns-share-v4.2.1` |
| [Promari Model Router](plugins/claude/promari-model-router/README.md) | Route Claude Code subagents to haiku/sonnet/opus by task difficulty (Japanese and English), keep safety-sensitive work on strong models, suggest Codex for bulk or second-opinion work, and log every decision | `plugins/claude/promari-model-router/` | `promari-model-router-v1.1.1` |
| [Promari Statusline](plugins/claude/promari-statusline/README.md) | A multi-line status line for Claude Code: context, rate limits with pace and forecast, cost, KPIs, quality of the work, git and pull request, tools and system at a glance, laid out to the width of your terminal | `plugins/claude/promari-statusline/` | `promari-statusline-v1.8.1` |

Plugins live under `plugins/<kind>/` (`claude/` for Claude Code, `wordpress/` for WordPress). Standalone tools will live under `tools/`
when they are added; plugin-specific build tools stay with their plugin.

## Use Promari SNS Share from the CDN

```html
<script type="module" src="https://cdn.jsdelivr.net/gh/tamito0201/promari-toolkit@promari-sns-share-v4.2.1/plugins/promari-sns-share/dist/promari-sns-share.min.js"></script>
<promari-sns-share></promari-sns-share>
```

Pin the component's complete release tag. Updating another component does not
change this URL. WordPress plugins are also distributed as component-specific
ZIP assets on [GitHub Releases](https://github.com/tamito0201/promari-toolkit/releases).

See the [plugin README](plugins/wordpress/promari-sns-share/README.md) for configuration,
analytics, integration, and development instructions.

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
