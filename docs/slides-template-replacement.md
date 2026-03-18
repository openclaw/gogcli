# Google Slides Template Text Replacement

## Overview

The `gog slides create-from-template` command allows you to create a new Google Slides presentation from a template and automatically replace placeholder text throughout the presentation.

## Basic Usage

```bash
gog slides create-from-template <templateId> <title> \
  --replace "key=value" \
  --replace "another=value"
```

## Features

### 1. Placeholder Format

By default, the command looks for placeholders in the format `{{key}}` in your template:

- In your template, add text like: `{{name}}`, `{{date}}`, `{{company}}`
- The command automatically wraps keys with `{{}}` if not already present
- Use `--exact` flag to match exact strings without `{{}}` wrapping

### 2. Multiple Replacement Sources

#### Command-line flags (for a few replacements):

```bash
gog slides create-from-template 1abc123 "Q1 Report" \
  --replace "quarter=Q1 2026" \
  --replace "revenue=$1.2M" \
  --replace "growth=15%"
```

#### JSON file (for many replacements):

Create a JSON file with your replacements:

```json
{
  "name": "John Doe",
  "title": "Sales Manager",
  "date": "2026-02-15",
  "sales": 125,
  "target": 100,
  "achieved": true
}
```

Then use it:

```bash
gog slides create-from-template 1abc123 "Monthly Report" \
  --replacements replacements.json
```

#### Combining both (flags override file):

```bash
gog slides create-from-template 1abc123 "Report" \
  --replacements base-data.json \
  --replace "date=2026-02-15"  # This overrides "date" from JSON
```

### 3. Type Conversion

When using JSON files, non-string values are automatically converted:
- Numbers: `125` → `"125"`
- Booleans: `true` → `"true"`
- Null: `null` → `""`
- Complex types: JSON-encoded

### 4. Exact String Matching

Use `--exact` to replace arbitrary text without `{{}}` wrapping:

```bash
gog slides create-from-template 1abc123 "Report" \
  --replace "OLD_TEXT=NEW_TEXT" \
  --exact
```

This is useful for templates not using the `{{key}}` convention.

## Examples

### Example 1: Simple Report Generation

Template contains:
- `{{employee_name}}`
- `{{report_date}}`
- `{{sales_total}}`

```bash
gog slides create-from-template 1abc123def456 "January Sales Report" \
  --replace "employee_name=Jane Smith" \
  --replace "report_date=2026-01-31" \
  --replace "sales_total=$45,000"
```

### Example 2: Batch Report Generation

Create `report-data.json`:
```json
{
  "month": "February",
  "year": "2026",
  "sales": "$52,000",
  "target": "$50,000",
  "performance": "104%"
}
```

Generate report:
```bash
gog slides create-from-template 1abc123def456 "February Sales Report" \
  --replacements report-data.json \
  --parent 1xyz789abc123  # Optional: place in specific folder
```

### Example 3: Scripted Bulk Generation

```bash
#!/bin/bash

TEMPLATE_ID="1abc123def456"

# Read CSV and generate presentations
while IFS=, read -r name title date sales; do
  gog slides create-from-template "$TEMPLATE_ID" "Report - $name" \
    --replace "name=$name" \
    --replace "title=$title" \
    --replace "date=$date" \
    --replace "sales=$sales" \
    --json > "output-$name.json"
done < employees.csv
```

## Output

### Text Output

```
Created presentation from template
id              1new456presentation
name            Q1 Report
link            https://docs.google.com/presentation/d/1new456presentation/edit

Replacements:
  quarter       3 occurrences
  revenue       2 occurrences
  growth        1 occurrences
```

### JSON Output

```bash
gog slides create-from-template 1abc123 "Report" \
  --replace "name=John" \
  --json
```

```json
{
  "presentationId": "1new456presentation",
  "name": "Report",
  "link": "https://docs.google.com/presentation/d/1new456presentation/edit",
  "replacements": {
    "name": 3,
    "date": 2,
    "total": 1
  }
}
```

## Tips

1. **Test your template first**: Create a test presentation to verify all placeholders are correctly placed
2. **Use consistent naming**: Stick to `{{key}}` format for clarity
3. **Check replacement counts**: The output shows how many times each placeholder was found
4. **Use JSON for complex data**: Easier to manage many fields or computed values
5. **Save folder IDs**: Use `--parent` to organize generated presentations

## Troubleshooting

### Placeholder not found (0 occurrences)

- Verify the placeholder exists in the template
- Check for typos in placeholder names (case-sensitive by default)
- Ensure `{{}}` wrapping is correct (or use `--exact`)

### Permission denied

- Ensure you have edit access to the template
- Check that the Google Slides API is enabled for your OAuth client

### Template copy failed

- Verify the template ID is correct
- Check that the template is accessible with your account
- Ensure it's a Google Slides presentation (not a document or sheet)

## Advanced Usage

### Preserve some placeholders

To replace only some placeholders and keep others for later editing:

```bash
# Only replace quarter and keep other {{}} placeholders
gog slides create-from-template 1abc123 "Draft Report" \
  --replace "quarter=Q1 2026"
```

### Case-sensitive vs case-insensitive

By default, replacements are case-sensitive. All placeholders and keys must match exactly.

### Whitespace in values

Values are not trimmed, allowing intentional whitespace:

```bash
--replace "description=  Indented text"
```
