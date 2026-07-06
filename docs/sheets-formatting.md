# Sheets Formatting

read_when:
- Adding or reviewing Google Sheets formatting commands.
- Using conditional formatting or alternating color banding from automation.

`gog sheets format` applies direct cell formatting. Use the advanced formatting
commands when the spreadsheet should keep applying styling as data changes.

For direct formatting, `--format-json` is enough for ordinary formats; gog
infers the Sheets field mask from the JSON:

```bash
gog sheets format "$spreadsheet_id" 'Sheet1!A1:C1' \
  --format-json '{"textFormat":{"bold":true},"backgroundColor":{"red":0.85,"green":0.9,"blue":1}}'
```

Use `--format-fields` when you want an explicit field mask or need to force a
specific nested field:

```bash
gog sheets format "$spreadsheet_id" 'Sheet1!A1:C1' \
  --format-json '{"textFormat":{"bold":false}}' \
  --format-fields textFormat.bold
```

## Data Validation

Set dropdown, checkbox, number, date, range, or custom-formula validation:

```bash
gog sheets validation set "$spreadsheet_id" 'Sheet1!B2:B100' \
  --type ONE_OF_LIST --value Open --value Done --strict
gog sheets validation set "$spreadsheet_id" 'Sheet1!C2:C100' --type BOOLEAN
gog sheets validation set "$spreadsheet_id" 'Sheet1!D2:D100' \
  --type NUMBER_BETWEEN --value 1 --value 100
gog sheets validation set "$spreadsheet_id" 'Sheet1!E2:E100' \
  --type ONE_OF_RANGE --value 'Lists!A2:A20'
```

Inspect or clear rules:

```bash
gog sheets validation get "$spreadsheet_id" 'Sheet1!B2:E100' --json
gog sheets validation clear "$spreadsheet_id" 'Sheet1!B2:E100'
```

Google Sheets stores table dropdowns on the table column rather than ordinary
cells. When a target fully selects a table-managed dropdown column, pass
`--filtered-rows-included` to set or clear it. Clearing converts that table
column to text. Validation-only copy/paste preserves table-managed dropdown
definitions when copying from a table column into ordinary cells.

## Cell Links

Write a single whole-cell link, multiple independently linked text runs, or a
batch of cells:

```bash
gog sheets links set "$spreadsheet_id" 'Sheet1!A1' \
  https://example.com "Project"
gog sheets links set "$spreadsheet_id" 'Sheet1!B1' \
  --runs-json '[{"text":"Docs","uri":"https://docs.example.com"},{"text":" / "},{"text":"Issues","uri":"https://issues.example.com"}]'
gog sheets links set "$spreadsheet_id" \
  --cells-json '[{"cell":"Sheet1!C1","url":"mailto:owner@example.com","text":"Owner"}]'
```

Read links back from a range:

```bash
gog sheets links get "$spreadsheet_id" 'Sheet1!A1:C1' --json
```

The read command emits one result per URL, so a rich-text cell can appear more
than once with the same A1 address.

## Conditional Formats

Add a rule to a sheet-qualified range:

```bash
gog sheets conditional-format add "$spreadsheet_id" 'Sheet1!A2:C' \
  --type text-eq \
  --expr done \
  --format-json '{"backgroundColor":{"red":0.85,"green":0.94,"blue":0.82}}'
```

Supported rule shortcuts:

- `text-eq`, `text-contains`, `text-starts-with`, `text-ends-with`
- `number-eq`, `number-gt`, `number-gte`, `number-lt`, `number-lte`
- `blank`, `not-blank`
- `custom-formula`

Use `--format-fields` when the JSON contains zero or false values that must be
sent explicitly:

```bash
gog sheets conditional-format add "$spreadsheet_id" 'Sheet1!A2:C' \
  --type custom-formula \
  --expr '=$C2=TRUE' \
  --format-json '{"textFormat":{"bold":false}}' \
  --format-fields textFormat.bold
```

Create color-scale gradients with Sheets API `GradientRule` JSON:

```bash
gog sheets conditional-format add "$spreadsheet_id" 'Sheet1!D2:D100' \
  --gradient-rule-json '{"minpoint":{"type":"MIN","colorStyle":{"rgbColor":{"red":1,"green":1,"blue":1}}},"maxpoint":{"type":"MAX","colorStyle":{"rgbColor":{"red":0.2,"green":0.7,"blue":0.2}}}}'
```

`--gradient-rule-json` cannot be combined with boolean-rule flags such as
`--type`, `--expr`, `--format-json`, or `--format-fields`.

List rules:

```bash
gog sheets conditional-format list "$spreadsheet_id" --json
gog sheets conditional-format list "$spreadsheet_id" --sheet Sheet1
```

Remove one rule by index, or all rules from a sheet:

```bash
gog sheets conditional-format clear "$spreadsheet_id" --sheet Sheet1 --index 0 --force
gog sheets conditional-format clear "$spreadsheet_id" --sheet Sheet1 --all --force
```

`clear --all` deletes from the highest index down so lower indexes do not shift
under the batch request.

## Banding

Apply default alternating row colors:

```bash
gog sheets banding set "$spreadsheet_id" 'Sheet1!A1:C20'
```

Override row or column banding with Sheets API `BandingProperties` JSON:

```bash
gog sheets banding set "$spreadsheet_id" 'Sheet1!A1:C20' \
  --row-properties-json '{"firstBandColorStyle":{"rgbColor":{"red":1,"green":1,"blue":1}},"secondBandColorStyle":{"rgbColor":{"red":0.96,"green":0.98,"blue":1}}}'
```

List and clear banded ranges:

```bash
gog sheets banding list "$spreadsheet_id" --json
gog sheets banding clear "$spreadsheet_id" --id 123456 --force
gog sheets banding clear "$spreadsheet_id" --sheet Sheet1 --all --force
```

## Command Pages

- [`gog sheets validation`](commands/gog-sheets-validation.md)
- [`gog sheets validation get`](commands/gog-sheets-validation-get.md)
- [`gog sheets validation set`](commands/gog-sheets-validation-set.md)
- [`gog sheets validation clear`](commands/gog-sheets-validation-clear.md)
- [`gog sheets links get`](commands/gog-sheets-links-get.md)
- [`gog sheets links set`](commands/gog-sheets-links-set.md)
- [`gog sheets conditional-format`](commands/gog-sheets-conditional-format.md)
- [`gog sheets conditional-format add`](commands/gog-sheets-conditional-format-add.md)
- [`gog sheets conditional-format list`](commands/gog-sheets-conditional-format-list.md)
- [`gog sheets conditional-format clear`](commands/gog-sheets-conditional-format-clear.md)
- [`gog sheets banding`](commands/gog-sheets-banding.md)
- [`gog sheets banding set`](commands/gog-sheets-banding-set.md)
- [`gog sheets banding list`](commands/gog-sheets-banding-list.md)
- [`gog sheets banding clear`](commands/gog-sheets-banding-clear.md)
