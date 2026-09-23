# Configuration reference

The source of configuration is a single TOML file:
[`config/share_config.example.toml`](../config/share_config.example.toml).
Neither the plugin nor the Web Component reads TOML directly; both consume
outputs produced by `tools/config.py`.

```bash
python3 tools/config.py --config .config/share_config.toml --write # Generate outputs
python3 tools/config.py --config .config/share_config.toml --check # Detect stale outputs
python3 tools/config.py --config .config/share_config.toml --print # Show effective settings
```

**Unknown keys, incorrect types, out-of-range values, and unknown services are
errors.** Validation fails closed instead of silently ignoring invalid settings.

## `[workflow]`: output locations

| Key | Type | Meaning |
|---|---|---|
| `target` | str | Target Git root. Relative paths are based on the parent of the configuration directory when it is named `.config/` or `config/`; otherwise, on the configuration file's directory |
| `plugin_output` | str | Directory for the WordPress plugin, relative to `target`. Empty disables plugin output |
| `web_output` | str | Directory for the minified Web Component bundle. Empty disables output. CDN hosting is recommended |
| `web_url` | str | HTTPS URL serving the bundle. Set both this and `web_output`, or leave both empty |
| `web_version` | str | Semantic version used by `tools/publish.sh` to create a release tag |

## `[share]`: primary buttons

| Key | Type | Meaning |
|---|---|---|
| `services` | list[str] | Primary buttons, ordered from left to right. At least one service; no duplicates. See the bundled services below |
| `heading` | str | Button-group heading, such as `SHARE`. Empty hides it. Not shown in the floating bar |

## `[share.labels]`: button labels

Keys are service identifiers and values are labels. Primary buttons display
them as text; secondary buttons use them for tooltips and accessible names.
Omitted entries use the service's default label. Newlines are not allowed.

The bundled labels are Japanese. To use English labels, override them:

```toml
[share.labels]
facebook = "Share"
x = "Post"
line = "Share"
hatena = "Hatena Bookmark"
linkedin = "LinkedIn"
email = "Send by email"
copy = "Copy URL"
native = "More sharing options"

[share.messages]
copied = "URL copied"
group_label = "Share this article"
```

## `[share.buttons.<name>]`: optional per-button overrides

| Key | Type | Meaning |
|---|---|---|
| `color` | str | `#rrggbb`; empty uses the official brand color |
| `label_style` | str | `icon_text`, `icon`, or `text`; empty inherits `appearance.label_style` |
| `tooltip` | str | Tooltip and accessible name; empty uses the configured label |
| `floating` | bool | Whether to include the button in the floating bar; defaults to true |

## `[share.appearance]`: appearance

| Key | Type | Values | Meaning |
|---|---|---|---|
| `size` | str | `small` / `large` | Primary button height and font size: 20px / 11px for small, 28px / 13px for large |
| `label_style` | str | `icon_text` / `icon` / `text` | Logo and label, logo only, or label only |
| `shape` | str | `official` / `pill` / `rounded` / `square` | Corner style; official uses a pill for X and 3px corners for other services |
| `gap_px` | int | 0–40 | Gap between buttons |
| `font_family` | str | CSS `font-family` | Label font; semicolons and closing braces are forbidden |
| `heading_position` | str | `left` / `top` / `none` | Heading position |
| `secondary_size_px` | int | 20–48 | Secondary icon diameter |
| `secondary_style` | str | `mono` / `brand` / `outline` | Secondary icon style; mono is gray with brand color on hover |

## `[share.text]`: shared text

| Key | Type | Meaning |
|---|---|---|
| `title_template` | str | Text for X, LINE, and email subjects; expands `{title}`, `{site}`, and `{url}` |
| `hashtags` | list[str] | X hashtags without `#`; spaces and commas are forbidden |
| `via` | str | X attribution without `@`; empty omits it |

## `[share.utm]`: UTM parameters

| Key | Type | Meaning |
|---|---|---|
| `enabled` | bool | Whether to append `utm_*` parameters to shared URLs |
| `source` / `medium` / `campaign` / `content` | str | Parameter values; `{service}` expands to the service identifier. Empty values are omitted |

## `[share.behavior]`: behavior

| Key | Type | Values | Meaning |
|---|---|---|---|
| `open_in_new_tab` | bool | | Whether to add `target="_blank"` |
| `popup` | bool | | Open a small share window; without JavaScript, normal link behavior applies |
| `popup_width` | int | 300–1200 | Popup width in pixels |
| `popup_height` | int | 300–1000 | Popup height in pixels |
| `nofollow` | bool | | Add `rel="nofollow"`; `noopener noreferrer` are always included |

## `[share.placements]`: WordPress placements

| Key | Type | Values | Meaning |
|---|---|---|---|
| `post_types` | list[str] | Post type names | Types eligible for automatic insertion and the floating bar. Empty disables automatic insertion |
| `article_top` | bool | | Before article content through `the_content` |
| `article_bottom` | bool | | After article content |
| `sidebar` | bool | | Register the Promari SNS Share widget |
| `floating` | bool | | Show the mobile floating bar; hidden at widths of 769px and above |
| `floating_position` | str | `bottom` / `top` | Floating-bar position |
| `floating_after_px` | int | 100–5000 | Scroll distance before showing the bar |
| `floating_secondary_max` | int | 0–8 | Maximum number of secondary buttons in the bar |
| `floating_hide_near_end` | bool | | Hide the bar near the page end |
| `floating_services` | list[str] | | Primary floating-bar services; empty inherits `services` |

The `promari_sns_share()` template tag and shortcode are always available,
independently of automatic placement settings.

## `[share.secondary]`: secondary channels

Keys are service identifiers and values are booleans. Buttons follow key order.
A service cannot appear in both `services` and `secondary`.

## `[share.tracking]`: tracking

| Key | Type | Meaning |
|---|---|---|
| `attribute` | str | Button attribute name, beginning with `data-`; its value is the service identifier |
| `event_name` | str | Click `CustomEvent` name; detail is `{service, url, placement}` |

## `[share.messages]`: UI messages

| Key | Meaning |
|---|---|
| `copied` | Toast after successfully copying the URL |
| `group_label` | Button group's `aria-label` |

## `[share.style]`: colors

| Key | Type | Meaning |
|---|---|---|
| `accent` | str | `#rrggbb`; heading, focus outline, and copy-success color |
| `floating_background` | str | CSS color for the floating-bar background |

## Bundled services

The label column below gives the English meaning of the bundled labels.
Use `[share.labels]` to select the exact text displayed on your site.

| Identifier | Label meaning | Action | Notes |
|---|---|---|---|
| `facebook` | Share | Share dialog | `#1877F2` |
| `x` | Post | Share dialog | `#000000`; pill-shaped; sends `text`, `hashtags`, and `via` |
| `line` | Share | Share dialog | `#06C755` |
| `hatena` | Hatena Bookmark | Share dialog | `#00A4DE` |
| `linkedin` | LinkedIn | Share dialog | `#0A66C2` |
| `email` | Send by email | `mailto:` | Subject is the shared text; body is the URL |
| `copy` | Copy URL | Clipboard | Falls back to an article link without JavaScript |
| `native` | More sharing options | Web Share API | Visible only where `navigator.share` is available |

See [Customization](customization.md) for adding services.

## Web Component attributes: TOML overrides

| Attribute | Configuration mapping |
|---|---|
| `services` | `share.services`, comma-separated |
| `secondary` | Enabled `share.secondary` services, comma-separated |
| `heading` / `heading-position` | `share.heading` / `appearance.heading_position` |
| `size` / `label-style` / `shape` | `appearance.*` |
| `accent` | `style.accent` |
| `hashtags` / `via` / `title-template` | `text.*` |
| `utm` | `utm.enabled`: `on` / `off` |
| `popup` / `new-tab` | `behavior.popup` / `behavior.open_in_new_tab` |
| `placement` | `inline` (default) / `floating` |
| `after` | `placements.floating_after_px` |
| `url` / `title` | Shared URL and title; defaults to the canonical URL and `document.title` |
| `config` | JSON for any of the settings above; takes highest precedence |

These attributes are the public styling contract. The component does not expose `::part`,
and its internal CSS custom properties are not an API: since 2.0.2, setting `--accent` on
the element no longer changes the color. The circular variant (`variant="circle"`) uses
fixed colors, and `size`, `shape`, `label-style`, and `accent` apply only to the default variant.
