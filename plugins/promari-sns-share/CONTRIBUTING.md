# Contributing

## Development environment

| Requirement | Version | Purpose |
|---|---|---|
| Node.js | 22.6 or later | Direct TypeScript execution for tests; esbuild for bundling |
| Python | 3.13 or later | `tools/config.py`, using only the standard library |
| PHP | 8.1 or later | Plugin syntax checks and rendering tests |

Run the commands in this directory, which has its own `package.json` and `pnpm-lock.yaml`.

```bash
pnpm install --frozen-lockfile
export PYTHON=python3.13 # Set this when python3 is older than 3.13.
pnpm run verify         # Types, TS tests, Python tests, PHP tests, distribution drift
pnpm run build          # Regenerate dist/ after destination or TOML changes.
```

## Making changes

1. Create a branch from `main` (`feat/...`, `fix/...`, or `docs/...`).
2. Make your changes. Edit destination specifications only in `destinations/*.toml`;
   JavaScript metadata is generated from them.
3. Update `dist/` with `pnpm run build`, then run `pnpm run verify`.
4. Open a pull request. CI runs the same checks; `config.py --check` detects
   distribution files that were not regenerated.

## Conventions

- **Fail closed:** invalid configuration must stop the generator with an error.
  Add type, range, and allowed-value validation in `tools/config.py`, plus tests,
  whenever you introduce an option.
- **Two sources of truth:** TOML defines configuration; PHP defines destinations.
  Do not manually edit their outputs (`share.json`, `generated/*.ts`, or `dist/`).
- **No telemetry requests:** the Web Component must not send analytics requests
  to Promari or other servers. The host page handles emitted events.
- **One-way dependencies:** domain has no dependencies; application depends on
  domain. Only infrastructure and presentation interact with the DOM.
- Use modern **PHP 8.1** (`readonly`, `enum`, `match`, and array pipelines),
  **Python 3.13** (frozen, slotted dataclasses, PEP 695, and `match`), and
  **strict TypeScript** without `any`.
- Write documentation and commit messages in English. Use English identifiers.

## Tests

| Target | Location | Command |
|---|---|---|
| Pure domain, application, and infrastructure logic | `web/test/*.test.ts` | `pnpm test` |
| Configuration generator | `tools/tests/test_config.py` | `pnpm run test:py` |
| Plugin rendering | `tools/tests/render_test.php` | `pnpm run test:php` |

## Releases

Releases are made by the maintainers' release workflow, not from this directory. This directory in
promari-toolkit is a published copy that changes only on a release, so do not edit it here.

1. Update the version in `package.json`, `pyproject.toml`, the PHP plugin header, and `web_version` and
   `web_url` in `config/share_config.example.toml`, and record the changes in `CHANGELOG.md` under `## <version>`.
2. Run `pnpm run build`, then commit the result (`dist/` is served by the CDN at the release tag).
3. When the new version is merged, the release workflow checks it, builds the WordPress ZIP, signs
   `checksums.txt`, publishes this directory and creates the `promari-sns-share-v<version>` tag and GitHub
   Release with the ZIP, the JavaScript bundle, `checksums.txt`, its signature, and an SBOM. The release
   notes contain the installation snippet with the SRI hash.

Other components retain their existing versions. A published tag never moves; publish a new version instead.
