# Changelog

This component follows [Semantic Versioning](https://semver.org/).

## 2.0.2

### Fixed

- The default variant declared `--accent` on `:host`, so a page could override the accent
  color by setting `--accent` on the element, an entry point that was never documented. The
  property is now declared inside the component; the `accent` attribute remains the only
  way to change it. The circular variant still has fixed colors.

## 2.0.1

### Changed

- Restructure the Web Component into layered architecture with DDD building blocks. The
  domain now owns the `ShareServiceRepository` and `ShareGateways` interfaces, and
  infrastructure implements them, so infrastructure depends only on the domain.
- Organize the domain into value objects (`model/`), domain services (`service/`),
  the repository interface (`repository/`), and gateway interfaces (`gateway/`).
- `HandleShareClick` returns the observed outcome instead of calling a notifier; the
  presentation layer shows the copy message. Attribute reading and floating-bar
  visibility moved to presentation, where the element's input and display belong.
- The architecture test now rejects imports from infrastructure into application.

No public attribute, method, event, or rendered output changed.

## 2.0.0

### Changed

- **Breaking:** the WordPress plugin now emits `<promari-sns-share>` and loads the pinned
  bundle instead of rendering buttons server-side. Set `web_output` and `web_url` so the
  plugin knows where the bundle lives. Template tags, the shortcode, the widget, automatic
  placement, and the floating bar keep working.
- **Breaking:** `ServiceInterface` replaces `shareUrl()` with `endpoint()` and `params()`.
  Service classes now declare where to send and which fields to send; they no longer build
  URLs. `AbstractService`, `ButtonRenderer`, `Styles`, `TextFormatter`, `ShareRequest`,
  `RendererInterface`, and `ServiceRegistry` are gone, and the
  `promari_sns_share_services` filter with them.
- One implementation builds share URLs. PHP and the component can no longer disagree,
  so the parity checks between them were removed along with the duplicated logic.

### Fixed

- Percent-encode `!'()*` as RFC 3986 requires; `encodeURIComponent` leaves them literal.
- Expand share text in a single pass, so a placeholder inside a title stays literal.
- Make the click switch exhaustive; an unhandled decision now fails to compile.

### Added

- Enforced four-layer boundaries in the Web Component: a dependency check that reads type
  imports and re-exports, and a type check that compiles the inner two layers without DOM
  or Node types.

## 1.1.0

- Add circular sharing controls with an overflow menu and host-managed like state.
- Keep reaction persistence outside the Web Component and expose a typed state method and like request event.

## [1.0.0] - 2026-09-17

### Added

- Promari SNS Share as the first independently versioned plugin in Promari Toolkit.
- The `<promari-sns-share>` Web Component with Facebook, X, LINE, Hatena Bookmark,
  LinkedIn, email, copy-link, and native sharing.
- A PHP 8.1 WordPress plugin, configurable placements, floating bars, UTM
  parameters, and the `promari-sns-share` CustomEvent for analytics.
- Validated TOML configuration and generated PHP / JavaScript service metadata.
- TypeScript, Python, and PHP tests with reproducible distribution checks.
- Component-specific release tags, a pinned CDN URL, and a WordPress ZIP asset.
