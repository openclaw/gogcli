# Slides structure

Create native, editable slides without rendering an image first:

```bash
gog slides new-slide <presentationId>
gog slides new-slide <presentationId> --layout TITLE_AND_BODY --index 1
```

Without a layout flag, Google creates a blank slide. `--layout` accepts the
Slides predefined layouts. A presentation theme can remove or modify a
predefined layout; Google returns an error when the selected layout is not
available in the active master.

For an exact theme layout, read its object ID from `slides info --json` and use
`--layout-id`:

```bash
gog slides info <presentationId> --json
gog slides new-slide <presentationId> --layout-id <layoutId>
```

`--layout` and `--layout-id` are mutually exclusive. `--index` is zero-based;
omitting it appends the slide.

## Duplicate and move

Find stable slide IDs, then duplicate or reorder them:

```bash
gog slides list-slides <presentationId> --json
gog slides duplicate-slide <presentationId> <slideId>
gog slides duplicate-slide <presentationId> <slideId> --to-index 2
gog slides move-slide <presentationId> <slideId> --to-index 0
```

Without `--to-index`, Google places a duplicate immediately after its source.
With `--to-index`, duplication and positioning happen in one batch update.
Move indexes are zero-based and refer to the presentation order before the move,
matching the Slides API. An index can range from zero through the slide count.

Use `--dry-run --json` to inspect the exact batch request without contacting
Google, and `slides list-slides --json` to verify the resulting order.
