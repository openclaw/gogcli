# Slides text editing

Slides text ranges use UTF-16 code-unit indexes. Find exact element IDs and
ranges before changing a deck:

```bash
gog slides locate <presentationId> "Quarterly revenue" --all --json
gog slides read-slide <presentationId> <slideId> --detail --json
```

## Style and links

`style-text`, `link`, and `bullets` target one shape object and a fixed range:

```bash
gog slides style-text <presentationId> <objectId> --range 4:21 \
  --bold --font Georgia --size 24 --text-color '#3366CC'
gog slides style-text <presentationId> <objectId> --range 4:21 --no-bold
gog slides link <presentationId> <objectId> --range 4:21 \
  --url https://example.com/details
gog slides link <presentationId> <objectId> --range 4:21 --clear
gog slides bullets <presentationId> <objectId> --range 0:42 --on \
  --preset BULLET_DISC_CIRCLE_SQUARE
gog slides bullets <presentationId> <objectId> --range 0:42 --off
```

Use `--dry-run --json` to inspect the exact Slides `batchUpdate` request without
auth or API access.

## Replace text safely

`replace-text` requires an explicit scope:

```bash
# One shape. Reads the deck first and uses revision control plus exact ranges.
gog slides replace-text <presentationId> old new --object <objectId>

# One or more slides.
gog slides replace-text <presentationId> old new --page <slideId>

# Deliberately replace across the complete deck.
gog slides replace-text <presentationId> old new --all
```

The unscoped form is rejected. Scripts that previously relied on implicit
deck-wide replacement must add `--all`.
