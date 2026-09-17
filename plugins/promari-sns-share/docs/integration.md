# Integration guide

## 1. Static HTML and any CMS

```html
<head>
  <script type="module" src="https://cdn.jsdelivr.net/gh/tamito0201/promari-toolkit@promari-sns-share-v1.0.0/plugins/promari-sns-share/dist/promari-sns-share.min.js"></script>
</head>
<body>
  <article>...</article>
  <promari-sns-share heading="Share this article"></promari-sns-share>
  <promari-sns-share placement="floating"></promari-sns-share>
</body>
```

- `type="module"` defers execution without blocking HTML parsing, so the script
  can go in `<head>`.
- The URL and title come from `<link rel="canonical">` and `document.title`.
  Override them with `url="..." title="..."`.
- The site name comes from `<meta property="og:site_name">` and supplies `{site}`.

### Pin content with Subresource Integrity (SRI)

```html
<script type="module"
  src="https://cdn.jsdelivr.net/gh/tamito0201/promari-toolkit@promari-sns-share-v1.0.0/plugins/promari-sns-share/dist/promari-sns-share.min.js"
  integrity="sha384-..." crossorigin="anonymous"></script>
```

Replace the placeholder with the integrity value printed by `tools/publish.sh`
when publishing. Release notes can include the same snippet. Do not use a fixed
SRI hash with a moving version such as `@1`, because its contents can change.

### Content Security Policy

Allow `https://cdn.jsdelivr.net` in `script-src` when loading from the CDN.
No provider `frame-src` permissions are needed because the component uses no
iframes. Share dialogs use `window.open`; set `popup="false"` for normal links.

## 2. WordPress

### Installation

```bash
git clone https://github.com/tamito0201/promari-toolkit.git
cp promari-toolkit/plugins/promari-sns-share/config/share_config.example.toml <site-repository>/.config/share_config.toml
```

Edit `[workflow]` in `.config/share_config.toml`:

```toml
[workflow]
target = "."
plugin_output = "public_html/wp-content/plugins/promari-sns-share" # WordPress plugin directory
web_output = "" # Leave empty for WordPress-only installations.
web_url = ""
```

```bash
python3 promari-toolkit/plugins/promari-sns-share/tools/config.py --config <site-repository>/.config/share_config.toml --write
wp plugin activate promari-sns-share
```

The generator installs the plugin and `assets/config/share.json`. After changing
configuration, rerun `--write` and deploy the updated files.

### Placements

| Placement | Enable with | Location |
|---|---|---|
| Automatic insertion before / after content | `placements.article_top` / `article_bottom`, plus `post_types` | `the_content` filter, main loop only |
| Template tag | Always available | `<?php promari_sns_share(['placement' => 'inline']); ?>` |
| Shortcode | Always available | `[promari_sns_share]` or `[promari_sns_share url="..." title="..."]` |
| Widget | `placements.sidebar` | Appearance > Widgets > Promari SNS Share |
| Mobile floating bar | `placements.floating` | `wp_footer`; hidden at widths of 769px and above |

If your theme already has a sharing partial such as `sns.php`, replace its
contents with:

```php
<?php if (function_exists('promari_sns_share')) promari_sns_share(); ?>
```

The `function_exists` check keeps the theme working when the plugin is disabled.

### Avoid duplicate buttons

Enabling `article_top` or `article_bottom` automatically inserts buttons for
eligible `post_types`. Calling `promari_sns_share()` in the same template may render
a second set. Choose automatic insertion or manual placement for that location.

### Tracking integration

Each button carries `tracking.attribute` (default: `data-share`). Track clicks
on `a[data-share]` and read its value (`facebook`, `x`, and so on) as the service
identifier. See [Analytics](analytics.md).

### Supported versions

PHP 8.1 or later, using readonly properties, enums, and first-class callables.
The plugin has been tested with WordPress 6.x.

## 3. React / Next.js

```tsx
// For example, in app/layout.tsx.
<Script type="module" src="https://cdn.jsdelivr.net/gh/tamito0201/promari-toolkit@promari-sns-share-v1.0.0/plugins/promari-sns-share/dist/promari-sns-share.min.js" strategy="afterInteractive" />

// Article component.
export const ShareBar = ({ url, title }: { url: string; title: string }) => (
  // @ts-expect-error Register the custom element in your JSX types to remove this directive.
  <promari-sns-share url={url} title={title} heading="SHARE" />
);
```

For typed JSX, add `promari-sns-share` to your React JSX `IntrinsicElements` with
`React.HTMLAttributes<HTMLElement>` plus optional `url`, `title`, `services`,
and `placement` string properties. React 19 and later support passing custom
element attributes directly.

## 4. Vue / Nuxt

```vue
<template>
  <promari-sns-share :url="url" :title="title" services="x,facebook,line" />
</template>
```

Configure the Vue compiler's `isCustomElement` option in your Vite setup as
`(tag) => tag === 'promari-sns-share'`.

## 5. Astro / Hugo / Jekyll / Wix / STUDIO

Use the static HTML approach wherever the platform accepts custom HTML: load
the script in the header and place `<promari-sns-share>` in the article template.

## 6. Runtime requirements

| Feature | Requirement or fallback |
|---|---|
| Browser | ES2022, Custom Elements, and Shadow DOM |
| JavaScript disabled | Web Components do not render. The WordPress plugin renders HTML on the server, so ordinary share links still work |
| Web Share API | `native` appears only where `navigator.share` is available |
| Clipboard | If `navigator.clipboard` is unavailable, the URL is shown in a prompt |
