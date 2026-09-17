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

1. Create `plugin/src/Service/ExampleService.php` and implement the six methods
   required by `ServiceInterface`.

```php
<?php
declare(strict_types=1);

namespace PromariSnsShare\Service;

use PromariSnsShare\Domain\Action;
use PromariSnsShare\Domain\ShareRequest;

final class ExampleService extends AbstractService
{
    public function key(): string { return 'example'; }
    public function label(): string { return 'Save to Example'; }
    public function shareUrl(ShareRequest $request): string
    {
        return $this->build('https://example.com/share', ['url' => $request->url, 'title' => $request->title]);
    }
    public function icon(): string { return '<svg viewBox="0 0 24 24" aria-hidden="true" focusable="false"><path fill="currentColor" d="..."/></svg>'; }
    public function brandColor(): string { return '#EF4056'; }
    public function action(): Action { return Action::Open; }
}
```

2. Add `new ExampleService()` to `ServiceRegistry::builtin()` to bundle it.
   For a theme-only extension, add it through the `promari_sns_share_services` filter.
3. Add `example` to TOML `services` or `secondary`.
4. Run `python3 tools/config.py --write`. The generator extracts its logo,
   color, and URL specification from PHP and includes them in the Web Component.

Requirements:

- `key()` contains only lowercase English letters and digits.
- `icon()` starts with `<svg ...>` and uses `fill="currentColor"` for CSS coloring.
- `brandColor()` returns a `#rrggbb` color.
- `shareUrl()` uses `$this->build('endpoint', [...])` or `return $request->url;`,
  the forms recognized by the generator.

Unsupported formats cause `--write` to fail validation.
