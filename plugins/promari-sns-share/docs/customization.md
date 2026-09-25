# Customization

Use TOML for WordPress or attributes for Web Components. Adding a new destination
is the main customization that requires source-code changes.

## Recipes

### Official widget styling (default)

```toml
[share]
destinations = ["facebook", "x", "line"]
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
destinations = ["x"]
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

### Attribute incoming traffic by destination in GA4

```toml
[share.utm]
enabled = true
source = "{destination}" # utm_source=facebook / x / line ...
medium = "social"
campaign = "share"
```

### Show X and LINE in a top floating bar

```toml
[share.placements]
floating = true
floating_position = "top"
floating_destinations = ["x", "line"]
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

## Add a destination

The following fictional destination illustrates the required structure. Replace
the example endpoint and SVG path with those of the destination you are integrating.

1. Create `destinations/example.toml`. The file declares the destination; it never builds a URL.

```toml
# Example: a fictional destination.
key = 'example'
label = 'Save to Example'
brand_color = '#EF4056'
action = 'open'
endpoint = 'https://example.com/share'
icon = '<svg viewBox="0 0 24 24" aria-hidden="true" focusable="false"><path fill="currentColor" d="..."/></svg>'

[params]
url = 'url'
title = 'title'
```

2. Add `example` to TOML `destinations` or `secondary`.
3. Run `python3 tools/config.py --write`. The generator reads every `destinations/*.toml`
   and includes its logo, color, endpoint, and fields in the bundle.
   Publish a new version so sites that pin a tag receive it.

Requirements:

- `key` contains only lowercase English letters and digits and matches the file name.
- `icon` starts with `<svg ...>` and uses `fill="currentColor"` for CSS coloring.
- `brand_color` is a `#rrggbb` color.
- `action` is `open`, `copy`, or `native`.
- `[params]` maps query parameter names to request fields (`url`, `title`, `text`,
  `via`, `site`, `hashtagsCsv`). An empty `endpoint` with no `[params]` means the
  destination has no dialog.
- Unknown fields are errors.

Unsupported formats cause `--write` to fail validation.
