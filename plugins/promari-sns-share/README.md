# Promari SNS Share

Social share buttons for Facebook, X, LINE, Hatena Bookmark, LinkedIn, email,
copying links, and the device's native share sheet, styled like the official
widgets **without third-party iframes or SDKs**. Use the Web Component on any
HTML page, or install the WordPress plugin to place it and load the pinned bundle.

[![CI](https://github.com/tamito0201/promari-toolkit/actions/workflows/ci.yml/badge.svg)](https://github.com/tamito0201/promari-toolkit/actions/workflows/ci.yml)
[![jsDelivr](https://data.jsdelivr.com/v1/package/gh/tamito0201/promari-toolkit/badge)](https://www.jsdelivr.com/package/gh/tamito0201/promari-toolkit)
![License: MIT](https://img.shields.io/badge/license-MIT-blue.svg)

```html
<script type="module" src="https://cdn.jsdelivr.net/gh/tamito0201/promari-toolkit@promari-sns-share-v2.0.2/plugins/promari-sns-share/dist/promari-sns-share.min.js"></script>
<promari-sns-share></promari-sns-share>
```

This renders three primary buttons (Facebook, X, and LINE), followed by smaller
icons for Hatena Bookmark, LinkedIn, email, copying the URL, and native sharing
on supported devices. Labels are configurable; the bundled defaults are Japanese.

## Why Promari SNS Share?

| Official widgets (iframes / SDKs) | Promari SNS Share |
|---|---|
| Separate third-party scripts and cookie considerations | One approximately 16 KB script; no third-party SDK requests |
| Clicks inside cross-origin iframes are inaccessible to your page | Observe clicks through a `data-share` attribute and a `CustomEvent` |
| Provider-specific iframe and script permissions in CSP | No provider iframe or SDK permissions needed |
| Limited control over labels, colors, shapes, and ordering | Configure every option through TOML or element attributes |
| Widgets appear after remote scripts load | One pinned script; the component renders locally in Shadow DOM |

The default styling follows the official widget dimensions (small: 20px high
with 11px bold text; large: 28px with 13px text), brand colors, and corner shapes
(pill-shaped for X). The goal is a familiar appearance with observable clicks
and less loading overhead.

## Installation

### A. Any website: Web Components and CDN

```html
<script type="module" src="https://cdn.jsdelivr.net/gh/tamito0201/promari-toolkit@promari-sns-share-v2.0.2/plugins/promari-sns-share/dist/promari-sns-share.min.js"></script>

<!-- Defaults: Facebook / X / LINE plus five secondary channels -->
<promari-sns-share></promari-sns-share>

<!-- Customize with attributes -->
<promari-sns-share services="x,facebook" secondary="copy,native" heading="Share this article" size="large" shape="pill" accent="#0a66c2"></promari-sns-share>

<!-- Mobile floating bar, shown after scrolling 400px -->
<promari-sns-share placement="floating" after="400"></promari-sns-share>

<!-- Configure any option through JSON -->
<promari-sns-share config='{"text":{"hashtags":["promari"],"via":"promari_jp"},"utm":{"enabled":true},"behavior":{"popup":false}}'></promari-sns-share>
```

Pin `@promari-sns-share-v2.0.2` to select this component's release independently.
Release tags are immutable. `tools/publish.sh` prints an installation snippet
with the matching Subresource Integrity (SRI) hash. jsDelivr serves the script.

### B. WordPress plugin

```bash
git clone https://github.com/tamito0201/promari-toolkit.git
cp promari-toolkit/plugins/promari-sns-share/config/share_config.example.toml <your-repository>/.config/share_config.toml
# Set plugin_output, and set web_output and web_url so the plugin can load the bundle.
python3 promari-toolkit/plugins/promari-sns-share/tools/config.py --config <your-repository>/.config/share_config.toml --write
wp plugin activate promari-sns-share
```

Once activated, the `<promari-sns-share>` element is inserted before and after
content for the post types listed in `post_types`, and the plugin loads the
pinned bundle from `web_url`. The plugin decides where the element appears; the
component builds the share URLs. To place it in a theme template:

```php
<?php if (function_exists('promari_sns_share')) promari_sns_share(); ?>
```

The plugin also provides the `[promari_sns_share]` shortcode, a **Promari SNS Share**
widget, and a configurable mobile floating bar. See [Integration](docs/integration.md).

## Configuration: one TOML file

Copy [`config/share_config.example.toml`](config/share_config.example.toml) to
the target site and edit its values. The template explains every option.
**Unknown keys, incorrect types, out-of-range values, and unknown services cause
validation errors** rather than silently falling back to defaults.

```toml
[share]
services = ["facebook", "x", "line"] # Primary buttons, in display order
heading = "SHARE"

[share.labels]
facebook = "Share"
x = "Post"
line = "Share"

[share.appearance]
size = "small"            # small (20px) / large (28px)
label_style = "icon_text" # icon_text / icon / text
shape = "official"        # official / pill / rounded / square

[share.placements]
post_types = ["post"]
article_top = true
article_bottom = true
sidebar = true
floating = true           # Mobile floating bar

[share.secondary]         # Secondary channels, shown as small icons
hatena = true
linkedin = true
email = true
copy = true
native = true             # Web Share API, on supported devices only

[share.text]
title_template = "{title} | {site}"
hashtags = ["promari"]
via = "promari_jp"

[share.utm]
enabled = true
source = "{service}"      # utm_source=facebook / x / line ...
```

See the [Configuration reference](docs/configuration.md) for all options and
accepted values, including labels and accessibility messages.

## Analytics

Buttons carry an attribute such as `data-share="facebook"`. Each click emits a
`promari-sns-share` `CustomEvent` with `detail: { service, url, placement }`.
For GA4, forward it as a `share` event:

```js
document.addEventListener('promari-sns-share', (e) => {
  gtag('event', 'share', { method: e.detail.service, content_type: 'article', item_id: e.detail.url });
});
```

See [Analytics](docs/analytics.md) for tracking methods and limitations.

## Architecture

- **PHP plugin:** Domain (value objects and enums), Contracts, Service (one class
  per service), Share (pure formatting logic), Render, and Plugin (composition
  root). Requires PHP 8.1 or later.
- **Web Components:** TypeScript with domain, application, infrastructure, and
  presentation layers. Dependencies point inward; clipboard, popup, and DOM
  operations go through ports.
- **Generator:** Python 3.13 or later, using only the standard library. Validates
  TOML and extracts logos, colors, and URL specifications from PHP service
  classes for JavaScript. **PHP is the single source of service metadata.**

See [Architecture](docs/architecture.md) for diagrams and dependency direction,
and [Customization](docs/customization.md) for recipes and adding services.

## Development

From the repository root, run `cd plugins/promari-sns-share` before the commands below.

```bash
pnpm install --frozen-lockfile
pnpm run verify   # Types, TypeScript tests, Python tests, PHP tests, distribution drift
pnpm run build    # Regenerate dist/promari-sns-share.min.js
tools/publish.sh --dry-run
```

Requirements: Node.js 22.6 or later (direct TypeScript execution), Python 3.13
or later, and PHP 8.1 or later. See [Contributing](CONTRIBUTING.md).

## License

MIT. The SVG logos are trademarks of their respective owners; follow each
owner's brand guidelines when using them.

### Circular sharing with likes

```html
<promari-sns-share variant="circle" like caption="この気づきを、誰かにも。"
  services="x,line,facebook,copy,native" secondary="hatena,linkedin,email"></promari-sns-share>
```

The component renders the complete reaction row, including the like button.
Listen for `promari-sns-share-like` (`detail.liked` is the requested state), persist
the change in your host application, then call
`element.setLikeState({liked, count, busy: false, message: ''})` with the confirmed
server state. Initialize this state after the custom element is defined. Likes
remain disabled until the host initializes them. No persistence endpoint or
credentials are embedded in the public library. On devices without Web Share,
the share button opens the additional destinations.
