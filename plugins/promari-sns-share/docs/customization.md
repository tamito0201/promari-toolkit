# Customization

Use TOML for WordPress or attributes for Web Components. Adding a new service
is the main customization that requires source-code changes.

## Recipes

### Official widget styling (default)

```toml
[share]
services = ["facebook", "x", "line"]
[share.appearance]
size = "small"
label_style = "icon_text"
shape = "official"
```

### Large, pill-shaped, icon-only buttons

```toml
[share.appearance]
size = "large"
label_style = "icon"
shape = "pill"
```

Web Components: `<promari-sns-share size="large" label-style="icon" shape="pill"></promari-sns-share>`.

### X as the only primary button

```toml
[share]
services = ["x"]
[share.secondary]
facebook = true
line = true
copy = true
```

### Use your own brand color

```toml
[share.buttons.facebook]
color = "#54347e"
[share.buttons.x]
color = "#54347e"
[share.buttons.line]
color = "#54347e"
[share.appearance]
secondary_style = "brand"
```

### Include the site name and hashtags

```toml
[share.text]
title_template = "{title} | {site}"
hashtags = ["promari", "WordPress"]
via = "promari_jp"
```

X receives `text=...&hashtags=promari,WordPress&via=promari_jp`, LINE receives
`text=...`, and email uses the formatted text as its subject.

### Attribute incoming traffic by service in GA4

```toml
[share.utm]
enabled = true
source = "{service}" # utm_source=facebook / x / line ...
medium = "social"
campaign = "share"
```

### Show X and LINE in a top floating bar

```toml
[share.placements]
floating = true
floating_position = "top"
floating_services = ["x", "line"]
floating_secondary_max = 1
```

### Open links in the current tab

```toml
[share.behavior]
popup = false
open_in_new_tab = false
```

### Move or hide the heading

```toml
[share]
heading = "Share this article"
[share.appearance]
heading_position = "top" # Use "none" to hide it.
```

### Match an existing tracking convention

```toml
[share.tracking]
attribute = "data-track-share"
event_name = "share:click"
```

## Add a service

The following fictional service illustrates the required structure. Replace
the example endpoint and SVG path with those of the service you are integrating.

1. Create `plugin/src/Service/ExampleService.php` and implement the methods
   required by `ServiceInterface`. The class declares the service; it never builds a URL.

```php
<?php
declare(strict_types=1);

namespace PromariSnsShare\Service;

use PromariSnsShare\Contracts\ServiceInterface;
use PromariSnsShare\Domain\Action;

final class ExampleService implements ServiceInterface
{
    public function key(): string { return 'example'; }
    public function label(): string { return 'Save to Example'; }
    public function endpoint(): string { return 'https://example.com/share'; }
    /** @return array<string,string> */
    public function params(): array { return ['url' => 'url', 'title' => 'title']; }
    public function icon(): string { return '<svg viewBox="0 0 24 24" aria-hidden="true" focusable="false"><path fill="currentColor" d="..."/></svg>'; }
    public function brandColor(): string { return '#EF4056'; }
    public function action(): Action { return Action::Open; }
}
```

2. Add `example` to TOML `services` or `secondary`.
3. Run `python3 tools/config.py --write`. The generator finds every final class in
   `src/Service/` and includes its logo, color, endpoint, and fields in the bundle.
   Publish a new version so sites that pin a tag receive it.

Requirements:

- `key()` contains only lowercase English letters and digits.
- `icon()` starts with `<svg ...>` and uses `fill="currentColor"` for CSS coloring.
- `brandColor()` returns a `#rrggbb` color.
- `endpoint()` returns a string literal, and `params()` returns an array literal
  mapping query parameter names to request fields (`url`, `title`, `text`, `via`,
  `site`, `hashtagsCsv`). Both empty means the service has no dialog.

Unsupported formats cause `--write` to fail validation.
