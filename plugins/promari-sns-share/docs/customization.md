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

### Let readers write about the article on Qiita, Zenn, note, Medium, or Ameba Blog

```toml
[share]
destinations = ["x", "note", "qiita", "zenn", "medium"]
[share.secondary]
ameba = true
[share.messages]
composed = "Copied a link to the article. Paste it into the editor."
```

`note` opens note's own share screen, and `ameba` opens Ameba Blog's editor with the title and a
link card already filled in. Qiita, Zenn, and Medium have no share entry point, so a click copies text
that the service turns into a card and opens the editor in a new tab (see
[ADR-0004](adr/0004-link-card-drafts.md)).

### Declare how a service turns a link into a card

```toml
# A fictional service whose editor turns a URL on its own line into a card.
key = 'example'
label = 'Example'
brand_color = '#EF4056'
action = 'compose'
endpoint = 'https://example.com/new'
icon = '<svg viewBox="0 0 24 24" aria-hidden="true" focusable="false"><path fill="currentColor" d="..."/></svg>'
draft = "{title}\n\n{url}\n"
draft_format = 'text'
compose_hint = 'Press Enter after the URL to make a card'
```

If the service's editor reads its body from the query instead, use `action = 'open'`, send the draft
in `[params]` (for example `body = 'draft'`), and use `draft_format = 'html'` when the body is HTML.

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
- `action` is `open`, `copy`, `native`, or `compose`.
- `compose` is for services with no share entry point: `endpoint` is the editor URL and must start
  with `https://`, and `[params]` must be absent (even an empty table is an error). A click copies the
  page title and URL, then opens the editor in a new tab (see [ADR-0003](adr/0003-compose-action.md)).
- `[params]` maps query parameter names to request fields (`url`, `title`, `text`,
  `via`, `site`, `hashtagsCsv`, `draft`). An empty `endpoint` with no `[params]` means the
  destination has no dialog.
- `draft` and `draft_format` (optional, always together) declare how the service writes a link card
  or embed for the article (see [ADR-0004](adr/0004-link-card-drafts.md)). The placeholders are
  `{title}`, `{url}`, `{description}`, `{image}`, and `{host}`; any other placeholder or a lone brace is
  an error. `draft_format = 'html'` HTML-escapes each value, and `'text'` inserts values as they are and
  drops leading blank lines. A `compose` destination copies its draft; an `open` destination must send it
  with `draft` in `[params]` (a draft that is never sent is an error), and never opens in a popup, since
  it opens an editor. Values are cut to 100 characters (title) and 60 (description), and a URL that
  carries a draft is kept at 2,900 characters or fewer by shortening the card. `copy` and `native` cannot
  declare a draft.
- `compose_hint` (optional, `compose` only, one line) follows the success notice, for example to tell
  readers to press Enter after pasting.
- Unknown fields are errors.

Unsupported formats cause `--write` to fail validation.
