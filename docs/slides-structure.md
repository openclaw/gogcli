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

## Native elements

Create editable shapes and lines on an existing slide:

```bash
gog slides element create-shape <presentationId> <slideId> \
  --type ROUND_RECTANGLE --x 24 --y 24 --width 180 --height 80
gog slides element create-line <presentationId> <slideId> \
  --category STRAIGHT --x 50 --y 150 --width 240 --height 40
```

Geometry defaults to points; pass `--unit EMU` for English Metric Units.
Supplying `--object-id` makes later scripted mutations deterministic. Custom
object IDs must use the Slides API's 5-50 character object-ID format.

Move, resize, shear, or rotate an element with an affine transform:

```bash
gog slides element transform <presentationId> <objectId> \
  --translate-x 12 --translate-y 6
gog slides element transform <presentationId> <objectId> \
  --scale-x 1.25 --scale-y 1.25
gog slides element transform <presentationId> <objectId> --rotate 15
```

Transforms are relative by default. `--apply-mode ABSOLUTE` replaces the
existing transform. Rotation is around the element origin and cannot be mixed
with explicit scale or shear flags in the same request.

Style shape fills/outlines or line strokes:

```bash
gog slides element style <presentationId> <shapeId> \
  --fill-color '#3367d6' --outline-color '#ffffff' --outline-weight 2
gog slides element style <presentationId> <lineId> --kind line \
  --outline-color '#ea4335' --outline-dash DASH
```

Colors accept `#RGB` or `#RRGGBB`. Use `--fill-transparent` or
`--outline-transparent` to remove rendering for that property.

Stack, group, annotate, and delete elements:

```bash
gog slides element z-order <presentationId> <objectId>... --operation BRING_TO_FRONT
gog slides element group <presentationId> <objectId> <objectId>... --group-id <groupId>
gog slides element ungroup <presentationId> <groupId>...
gog slides element alt-text <presentationId> <objectId> \
  --title "Chart" --description "Quarterly revenue by region"
gog slides element delete <presentationId> <objectId> --force
```

`z-order` targets must share one slide and must not be grouped. `group` needs at
least two ungrouped elements on one slide; Slides does not permit every element
kind to be grouped. Passing an empty `--title=` or `--description=` clears that
alt-text field. Element deletion is destructive and requires confirmation or
`--force` in non-interactive use. Every mutation supports `--dry-run --json`.
