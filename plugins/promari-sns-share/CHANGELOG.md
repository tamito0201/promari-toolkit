# Changelog

This component follows [Semantic Versioning](https://semver.org/).

## 4.2.0

### Added

- Writing destinations introduce the article the way each service makes a link card or embed, measured
  in a signed-in browser on 2026-10-01 (docs/adr/0004-link-card-drafts.md):
  - `ameba` changes from `compose` to `open`: Ameba Blog's entry editor reads `entry_title` and
    `entry_text` from the query, so a click opens it with the page title and an Ameba link card (HTML
    with the title, description, host, and image) already filled in. Nothing is copied.
  - `qiita` copies "title, blank line, URL, blank line"; the blank lines make Qiita's preview show a card.
  - `zenn` copies the title and Zenn's card notation `@[card](URL)`.
  - `medium` copies the URL alone and adds "貼り付けたあと Enter を押すと、記事のカードになります" to the
    success notice; Medium makes a card only from a URL alone on its line followed by Enter.
- Destination files accept optional `draft` and `draft_format` (`text` or `html`): a template with the
  placeholders `{title}`, `{url}`, `{description}`, `{image}`, and `{host}`. `html` HTML-escapes each value.
  Generation fails closed on unknown placeholders or lone braces, a `draft` without `draft_format` (or the
  reverse), a draft on `copy` or `native`, an `open` draft that `[params]` never sends, and `draft` sent in
  `[params]` without a template.
- The request field `draft` for `[params]`, so an `open` destination can send its draft in the share URL.
- Optional `compose_hint` for `compose` destinations, shown after the success notice.
- The page description (`og:description`, else `meta name="description"`) and image (`og:image`, resolved
  to an absolute http(s) URL) are read for link cards. The element accepts `description` and `image`
  attributes to override them.
- Link-card limits: the title is cut to 100 characters and the description to 60 (ending with "…"), and a
  share URL that carries a draft stays at 3,500 characters or fewer. When it would be longer, the
  description is shortened and then left out, then the image, and last the title is shortened. Ameba
  limits the whole request (URL plus cookies): signed in with about 1,800 characters of cookies, a
  5,105-character URL passed and 5,140 got 400 (signed out: 302 up to 6,000, 400 at 8,000). The 3,500
  budget leaves room for about 1,600 more characters of cookies, and keeps the image for a promari.jp
  title of about 70 characters; the image is left out only from a Japanese title of about 100 characters.

### Changed

- The default `share.messages.composed` is now 「記事のリンクをコピーしました。投稿画面に貼り付けてください」
  (was 「タイトルとURLをコピーしました。…」), because Medium now copies the URL alone. Configured messages
  are unchanged.
- An `open` destination that sends a draft opens the service's editor, so it never opens in a popup,
  whatever `behavior.popup` says (the reason ADR-0003 gave for `compose`).
- In-page toasts stay longer for longer text (1.8 s up to 6 s), so a compose hint can be read.
- Internal: `ShareDestination` fills in drafts through the domain services `DraftTemplate` and
  `LinkCardPolicy`; `SharedPage` and `ShareRequest` carry `description` and `image`;
  `ShareActionPolicy.canOpenInPopup()` takes `{ href, sendsDraft }`. Destinations without the new fields
  generate the same catalog entries as 4.1.0.

## 4.1.0

### Added

- Destinations for writing about the article: `note`, `qiita`, `zenn`, `medium`, and `ameba`
  (Ameba Blog). Logos come from simple-icons 16.33.0 (CC0 1.0). None is enabled in the example
  configuration; add them to `share.destinations` or `share.secondary`.
- `note` uses note's official share screen (`https://note.com/intent/post`) with `url` and `hashtags`.
- New action `compose` for services without a share entry point (Qiita, Zenn, Medium, Ameba Blog).
  A click puts "page title, line break, shared URL" (with the same UTM parameters as other
  destinations) on the clipboard and opens the service's editor in a new tab. Both effects start in
  the click's synchronous turn so popup blockers allow the tab. The tab always opens as a new tab,
  regardless of `behavior.popup` and `behavior.open_in_new_tab`. See docs/adr/0003-compose-action.md.
- Optional messages `share.messages.composed` and `share.messages.compose_failed`, with Japanese
  defaults. Existing configuration files remain valid.
- Destination files with `action = 'compose'` fail closed unless `endpoint` starts with `https://`,
  and reject a `[params]` table even when it is empty.

### Changed

- The circular share bar (`variant="circle"`) wraps its buttons, so ten buttons fit at 320px and
  390px without horizontal scrolling.
- `compose` destinations render as `<button>` in both share bars; the default bar's styles apply to
  buttons as well as links.
- Internal: whether a button may be a link, and whether its action is following the link, is decided once
  in the domain (`ShareActionPolicy.followsLink()` / `linkable()`) and carried in the view model; the
  views no longer compare action names. Opening a new tab is its own domain port (`NewTabGateway`,
  implemented by `BrowserNewTab`) instead of a second method on `ShareWindowGateway`. The compose text
  comes from the destination value object (`ShareDestination.composeDraft()`). Markup and behavior of
  `open`, `copy`, and `native` are unchanged.

## 4.0.0

### Changed

- **Breaking (custom destinations only):** destinations are declared as data, one
  `destinations/<key>.toml` per destination (`key`, `label`, `brand_color`, `action`,
  `endpoint`, `icon`, `[params]`), and read with `tomllib`. The PHP classes in
  `plugin/src/Destination/`, `Contracts\ShareDestinationInterface`, and `Domain\ShareAction`
  are removed: they were never loaded at runtime and were parsed with regular expressions,
  a leftover from when PHP built share URLs. Move a custom destination by copying its
  return values into a TOML file (see docs/customization.md).
- Destination files fail closed: unknown fields, a `key` that differs from the file name,
  an unknown `action`, or an unknown request field in `[params]` stop generation.
- The generated catalog and `dist/promari-sns-share.min.js` are byte-identical to 3.2.0;
  sites need no change beyond the version string.

## 3.2.0

### Added

- `setLikeState()` accepts `count: null` for an unknown count (not fetched yet, or the fetch
  failed). The count field then shows "—" with the accessible name "いいねの件数は未取得", instead
  of a 0 that cannot be told apart from a confirmed zero. The state type is exported as `LikeState`.
- The count starts as unknown ("—") until the host passes a count. Negative, fractional, and
  non-number counts are still ignored.

## 3.1.0

### Changed

- Build the object graph with a DI container (InversifyJS 8) in the new `web/src/composition/`
  (`ShareContainer`, `InjectionTokens`, `TypedBinding`). The four layers stay free of container code
  and keep constructor injection; bindings use factories, not decorators or `reflect-metadata` lookups.
  Tokens carry their bound type, so a dependency listed in the wrong order is a compile error.
- The click use case is bound as a per-element factory, so activity events keep going to the element that was clicked.
- The architecture test allows external packages only in `composition` (`inversify`) and checks that
  `index.ts` goes through `ShareContainer` rather than choosing implementations itself.
- Runtime dependencies: `inversify` and its peer `reflect-metadata` (loaded by `@inversifyjs/container`).
  The bundle grows from about 28 KB to about 116 KB (31 KB gzip).

No public attribute, event, or configuration key changed.

## 3.0.0

Breaking release. Names now describe what each thing is, without keeping the 2.x names.

### Breaking

- Configuration: `share.services` → `share.destinations`,
  `share.placements.floating_services` → `share.placements.floating_destinations`,
  and the UTM placeholder `{service}` → `{destination}`.
- Element attribute `services` → `destinations`.
- Click event detail `{service, url, placement}` → `{destination, url, placement}`.
- PHP: `Contracts\ServiceInterface` → `Contracts\ShareDestinationInterface`,
  `Service\XxxService` → `Destination\XxxDestination`, `Domain\Action` → `Domain\ShareAction`.
  Custom destinations go in `plugin/src/Destination/*Destination.php`.

### Changed

- A share target is called a destination throughout: `ShareDestination`,
  `ShareDestinationRepository`, `InMemoryShareDestinationRepository`, `DestinationSelectionPolicy`.
- Infrastructure classes are named after the technology, not the domain interface:
  `BrowserClipboard`, `BrowserNativeShare`, `BrowserPopupWindow`, `BrowserSharedPage`,
  `CustomEventShareActivityPublisher`. "Gateway" names only domain interfaces
  (`ShareWindowGateway`, `SharedPageGateway`, ...). The architecture test rejects infrastructure files named `*Gateway`.
- Domain services: `ShareActionPolicy.decide()`, `ShareTextFormatter.format()`, `UtmParameterPolicy.apply()`.
- Application: `ShareButtonCatalog` (was `DisplayCatalog`) and `ShareSettings` (was `ShareConfig`).
- Presentation: `ShareSettingsAttributeReader`, `CircularShareBarView`, `ShareBarStylesheet`,
  `FloatingBarVisibility`, `HtmlEscaper`.
- `ShareTypes.ts` is split into one file per concept.

Rendered markup and styling are unchanged.

## 2.0.3

### Changed

- Make each layer class-based with consistent naming: classes and interfaces start with an
  uppercase letter, methods with a lowercase letter, and each file is named after its class
  (except `index.ts` and generated data). Use cases expose `execute()`
  (`BuildShareBarUseCase`, `HandleShareClickUseCase`); domain services are classes with static
  methods (`ClickPolicy.decide()`, `ServiceSelectionPolicy.select()`, `ShareTextPolicy.fill()`,
  `UtmPolicy.apply()`, `UriEncoder.encode()`); infrastructure classes implement the domain
  interfaces (`BrowserClipboardGateway`, `WebShareGateway`, `PopupWindowGateway`,
  `CustomEventActivityPublisher`, `BrowserPageContext`, `SpecShareServiceRepository`).
- Split the gateway interfaces into one file each and add `PageContextGateway`.
- The architecture test also checks that file names in the four layers start with an uppercase letter.

No public attribute, method, event, or rendered output changed.

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
