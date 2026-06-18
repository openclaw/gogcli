# Slides introspection

Use the structured Slides commands when an edit needs stable object IDs or exact
text ranges. `gog slides raw` remains the lossless API escape hatch.

## Presentation metadata

`slides info` combines Drive file metadata with Slides-native metadata:

```bash
gog slides info <presentationId> --json
```

The `presentation` object includes slide count, page size in points, locale,
masters and their theme colors, and layouts.

## Slide structure

The default `read-slide` output includes shape text, table-cell text, image
content/source URLs, and speaker notes. Add `--detail --json` for normalized
elements:

```bash
gog slides read-slide <presentationId> <slideId> --detail --json
```

Detailed elements are flattened in visual hierarchy order. Group children carry
`parentObjectId`. `geometry` is the element's axis-aligned bounding box in
points after its group and element transforms are applied. Text runs retain the
Slides API's UTF-16 `startIndex`/`endIndex`, style, links, paragraph style, and
bullet metadata. Foreground/background colors are also normalized as `#RRGGBB`
or `theme:NAME`; API-omitted inherited style fields remain omitted.

Rendered image `contentUrl` values are short-lived and requester-scoped.
`sourceUrl` is included when the presentation retains the original insertion
URL.

## Locate editable text

`slides locate` searches shapes and table cells without changing the deck:

```bash
gog slides locate <presentationId> "Quarterly revenue" --all --json
gog slides locate <presentationId> "Approved" --page <slideId> --match-case
```

Each match includes the slide and element object IDs plus exact UTF-16 range.
Table matches also include zero-based row and column indexes. By default only
the first match is returned; use `--all` or `--occurrence N` to select results,
and `--fail-empty` for automation that requires a match.
