# Promari Toolkit

A collection of reusable plugins and tools for websites and development workflows.
Each component owns its documentation, tests, changelog, and release version.
The repository has no shared product version.

## Components

| Component | Purpose | Location | Release tags |
|---|---|---|---|
| [Promari SNS Share](plugins/promari-sns-share/README.md) | Social sharing for Web Components and WordPress | `plugins/promari-sns-share/` | `promari-sns-share-v4.2.0` |
| [Promari Model Router](plugins/promari-model-router/README.md) | Route Claude Code subagents to haiku/sonnet/opus by task difficulty (Japanese and English), keep safety-sensitive work on strong models, suggest Codex for bulk or second-opinion work, and log every decision | `plugins/promari-model-router/` | `promari-model-router-v1.0.2` |

Website plugins live under `plugins/`. Standalone tools will live under `tools/`
when they are added; plugin-specific build tools stay with their plugin.

## Use Promari SNS Share from the CDN

```html
<script type="module" src="https://cdn.jsdelivr.net/gh/tamito0201/promari-toolkit@promari-sns-share-v4.2.0/plugins/promari-sns-share/dist/promari-sns-share.min.js"></script>
<promari-sns-share></promari-sns-share>
```

Pin the component's complete release tag. Updating another component does not
change this URL. WordPress plugins are also distributed as component-specific
ZIP assets on [GitHub Releases](https://github.com/tamito0201/promari-toolkit/releases).

See the [plugin README](plugins/promari-sns-share/README.md) for configuration,
analytics, integration, and development instructions.

## Development

Use Node.js from `.node-version`, pnpm, Python 3.13 or later, and PHP 8.1 or later.

```bash
pnpm install --frozen-lockfile
export PYTHON=python3.13
pnpm --filter @promari/sns-share verify
pnpm --filter @promari/sns-share build
```

## Independent releases

A component release uses `<component>-v<major>.<minor>.<patch>` as its Git tag.
Only that component's manifest and changelog are advanced. Git tags identify a
repository snapshot, while release notes and attached files describe one component.
The automatically generated source archive contains the whole repository; use
the component ZIP asset when installing a WordPress plugin.

Released tags are immutable. Each release is identified by its component tag.
All documentation and commit messages are written in English.

## License

[MIT](LICENSE). Brand logos remain trademarks of their respective owners.
