# Sheets Formatting

read_when:
- Adding or reviewing Google Sheets formatting commands.
- Using conditional formatting or alternating color banding from automation.

`gog sheets format` applies direct cell formatting. Use the advanced formatting
commands when the spreadsheet should keep applying styling as data changes.

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

- [`gog sheets conditional-format`](commands/gog-sheets-conditional-format.md)
- [`gog sheets conditional-format add`](commands/gog-sheets-conditional-format-add.md)
- [`gog sheets conditional-format list`](commands/gog-sheets-conditional-format-list.md)
- [`gog sheets conditional-format clear`](commands/gog-sheets-conditional-format-clear.md)
- [`gog sheets banding`](commands/gog-sheets-banding.md)
- [`gog sheets banding set`](commands/gog-sheets-banding-set.md)
- [`gog sheets banding list`](commands/gog-sheets-banding-list.md)
- [`gog sheets banding clear`](commands/gog-sheets-banding-clear.md)
