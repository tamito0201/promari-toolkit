# Contributing

## Development environment

| Requirement | Version | Purpose |
|---|---|---|
| Node.js | 22.6 or later | Direct TypeScript execution for tests; esbuild for bundling |
| Python | 3.13 or later | `tools/config.py`, using only the standard library |
| PHP | 8.1 or later | Plugin syntax checks and rendering tests |

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

1. Update the component package version, PHP plugin version, and `web_version` in `config/share_config.example.toml` and record the
   changes in `CHANGELOG.md`.
2. Run `pnpm run build`, then commit the result.
3. Run `tools/publish.sh --dry-run` to review the component release.
4. Run `tools/publish.sh` to publish the namespaced tag, WordPress ZIP, JavaScript, and release notes with SRI.
   Other components retain their existing versions. Never move a published tag.
