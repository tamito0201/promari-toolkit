# Analytics

Observable clicks are a major reason to avoid cross-origin iframe widgets.
Promari SNS Share provides two ways to track them.

## 1. The `data-share` attribute

Each button carries an attribute such as `data-share="facebook"`. Change its
name with `tracking.attribute`. This is useful for server-rendered markup and
tag managers.

- In Google Tag Manager, match click elements against `a[data-share]` and read
  the clicked element's `dataset.share` value.
- For custom tracking:
  `document.addEventListener('click', e => { const a = e.target.closest('[data-share]'); if (a) send(a.dataset.share); })`

Web Components attach the same attribute to links **inside** their Shadow DOM.
An outside `closest` call cannot reach those links, so use the CustomEvent below.

## 2. The `promari-sns-share` CustomEvent

Each click emits an event from the element (Web Components) or `document`
(WordPress). With `bubbles: true, composed: true`, a listener on `document`
can receive events across the Shadow DOM boundary.

```js
document.addEventListener('promari-sns-share', (e) => {
  const { destination, url, placement } = e.detail;
  // Forward to GA4.
  gtag('event', 'share', { method: destination, content_type: 'article', item_id: url, placement });
});
```

The event's `detail` contains:

| Key | Example | Meaning |
|---|---|---|
| `destination` | `x` | Share destination identifier |
| `url` | `https://example.jp/post/` | Page URL before UTM parameters are added |
| `placement` | `article_bottom` | Click location: `article_top`, `article_bottom`, `sidebar`, `floating`, or `inline` |

Change the event name with `tracking.event_name`.

## 3. Attribute incoming traffic with UTM parameters

```toml
[share.utm]
enabled = true
source = "{destination}"
medium = "social"
campaign = "share"
```

The shared URL becomes, for example,
`https://example.jp/post/?utm_source=x&utm_medium=social&utm_campaign=share`.
GA4 can attribute incoming visits to `x / social` in Traffic acquisition.
**Button clicks and incoming visits are different metrics:** one counts clicks
on your buttons, while the other counts visits through shared links.

## 4. Limitations

- Opening a share dialog does not confirm that a post was published; the social
  destinations do not report that outcome to this component.
- Copy-link tracking records the action, not where the link is later pasted.
- The native share sheet does not report which destination app was selected.
- Visitors whose blockers disable GA4 will be absent from GA4 measurements.
