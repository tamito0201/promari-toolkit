# Architecture

Three components share **one configuration source (TOML) and one service
catalog (PHP service classes)**.

```mermaid
%%{init: {"theme":"dark", "themeVariables": {"primaryColor":"#3B82F6","primaryTextColor":"#fff","primaryBorderColor":"#60A5FA","lineColor":"#6366F1","secondaryColor":"#10B981","tertiaryColor":"#EC4899"}}}%%
flowchart LR
    T["share_config.toml"] --> G["tools/config.py<br>Validate and generate"]
    P["plugin/src/Service/*.php<br>Logos, colors, URL specifications"] --> G
    G --> J["share.json"]
    G --> D["dist/promari-sns-share.min.js"]
    J --> W["WordPress plugin<br>Server-side rendering"]
    P --> W
    D --> C["CDN: jsDelivr"]
    C --> H["Any HTML page<br>&lt;promari-sns-share&gt;"]
    style T fill:#F59E0B,stroke:#FBBF24,color:#fff
    style P fill:#8B5CF6,stroke:#A78BFA,color:#fff
    style G fill:#3B82F6,stroke:#60A5FA,color:#fff
    style J fill:#10B981,stroke:#34D399,color:#fff
    style D fill:#10B981,stroke:#34D399,color:#fff
    style W fill:#EC4899,stroke:#F472B6,color:#fff
    style C fill:#06B6D4,stroke:#22D3EE,color:#fff
    style H fill:#EC4899,stroke:#F472B6,color:#fff
```

## Two sources of truth

| Source | Location | Generated outputs |
|---|---|---|
| Configuration: what, where, and how to display | `share_config.toml` | `share.json` for the plugin; `generated/defaults.ts` for JavaScript |
| Services: SVG logos, brand colors, and share URL construction | `plugin/src/Service/*.php` | `generated/catalog.ts` for JavaScript |

The generator reads PHP classes with regular expressions and extracts `key()`,
`label()`, `brandColor()`, `icon()`, `action()`, and the
`$this->build('endpoint', ['k' => $request->url, ...])` form in `shareUrl()`.
**JavaScript does not maintain a separate copy of service logos or URL rules.**
PHP changes flow into JavaScript on the next build; `--check` detects drift.

## PHP plugin: layered architecture and SOLID

```text
plugin/
  promari-sns-share.php          Bootstrap: requires and the promari_sns_share() template tag
  src/Domain/                Action and Placement enums; readonly ShareRequest value object
  src/Contracts/             ConfigInterface, ServiceInterface, RendererInterface
  src/Config/JsonConfig      Reads generated configuration and fails closed
  src/Service/               One final class per service; AbstractService builds URLs; ServiceRegistry
  src/Share/TextFormatter    Template expansion and UTM formatting
  src/Render/                ButtonRenderer (HTML), Styles (CSS / JavaScript)
  src/Integration/Widget     WordPress sidebar widget
  src/Plugin.php             Composition root and WordPress hook wiring
```

| Principle | Application |
|---|---|
| **S**: single responsibility | Separate configuration loading, URL construction, formatting, rendering, and wiring |
| **O**: open / closed | Add a `ServiceInterface` implementation and register it through `promari_sns_share_services` |
| **L**: substitution | Renderers rely on the six-method service contract, not concrete service classes |
| **I**: interface segregation | Themes use the template tag or shortcode without knowing renderer internals |
| **D**: dependency inversion | Rendering and formatting depend on `ConfigInterface`, not the JSON representation |

The plugin uses PHP 8.1 features: strict types, readonly properties, enums,
`match`, named arguments, first-class callables, and `array_map` / `array_filter`
pipelines.

## Web Components: four TypeScript layers

```text
web/src/
  domain/          Types, ShareRequest, Service, policies: pure logic, no DOM
  application/     BuildShareBar view model, HandleShareClick, ports: use cases
  infrastructure/  Browser ports for clipboard, sharing, popups, toasts, tracking, scroll; AttributeConfig
  presentation/    PromariSnsShareElement, view (HTML), styles (CSS)
  index.ts         Composition root
  generated/       catalog.ts and defaults.ts: generated, not committed
```

Dependencies point from **presentation to application to domain**.
Infrastructure implements the interfaces in application `ports.ts`. Domain and
application tests in `web/test/` run without a DOM, using plain objects as ports.

### Why four layers rather than MVVM?

MVVM is especially useful when a framework supplies automatic ViewModel-to-View
binding. Building that machinery for a mostly stateless Web Component would add
unnecessary complexity. The useful separation remains: `BuildShareBar` creates
a view model and `view.ts` renders it. A React or Vue wrapper can replace the
presentation layer without rewriting the domain logic.

### Data flow

1. `AttributeConfig.readConfig` merges attributes and JSON configuration into defaults.
2. `BuildShareBar` selects services (including native capability and floating-bar
   filtering), builds UTM URLs, and calls `Service.shareUrl` to create a view model.
3. `view.renderHtml` and `styles.buildCss` populate the Shadow DOM.
4. `HandleShareClick` uses `decideClick` to choose copy, native share, popup, or
   normal navigation, then calls the relevant ports and emits a CustomEvent.

### Security and performance

- Analytics are exposed as events to the host page; the component does not send
  telemetry requests to a remote server.
- URL components are encoded and rendered HTML is escaped.
- Shadow DOM isolates component styles from host-page styles.
- The minified bundle is approximately 16 KB, has no runtime library dependencies,
  and is loaded as a deferred module.

## Generator: Python 3.13 and the standard library

`tools/config.py` uses frozen, slotted dataclasses, PEP 695 type parameters,
`StrEnum`, `match`, `pathlib`, and `tomllib`. It has no third-party Python
dependencies. Validation combines small pure helpers (`fields`, `ensure`,
`one_of`, and `within`); errors identify both the setting and the problem.
Writes use temporary files and `os.replace` for atomic replacement.

## Quality gates: `pnpm run verify`

| Check | Purpose |
|---|---|
| `tsc --noEmit` with strict, noUncheckedIndexedAccess, and exactOptionalPropertyTypes | Type consistency |
| `node --test` (24 tests) | URLs, UTM parameters, service selection, click decisions, and attribute merging |
| Python `unittest` (9 tests) | TOML validation, fail-closed behavior, deployment / check convergence, and PHP catalog extraction |
| `render_test.php` (16 checks) | Configured HTML, CSS, and JavaScript rendering with WordPress-free stubs |
| `config.py --check` | Distribution files match regenerated outputs |
