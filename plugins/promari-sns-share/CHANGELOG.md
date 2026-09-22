# Changelog

This component follows [Semantic Versioning](https://semver.org/).

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
