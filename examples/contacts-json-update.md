# Updating Contacts from JSON

The `contacts update` command now supports updating contacts from JSON input, allowing you to modify any field supported by the Google People API.

## Usage

### Update from a file
```bash
# 1. Get current contact as JSON
gog contacts get people/c123456 --json > contact.json

# 2. Edit contact.json to add/modify fields
# Example: Add a URL
jq '.contact.urls += [{"value": "https://example.com", "type": "profile"}]' contact.json > updated.json

# 3. Update the contact
gog contacts update people/c123456 --from-file=updated.json
```

### Update from stdin
```bash
# Pipe modifications directly
gog contacts get people/c123456 --json | \
  jq '.contact.urls += [{"value": "obsidian://open?vault=main&file=Contact", "type": "profile"}]' | \
  gog contacts update people/c123456 --from-file=-
```

## Supported Fields

The JSON input supports all fields from the Google People API, including:

- `names` - Contact names
- `emailAddresses` - Email addresses
- `phoneNumbers` - Phone numbers
- `urls` - Website URLs and deep links
- `biographies` - Notes/biography text
- `addresses` - Physical addresses
- `birthdays` - Birthday information
- `organizations` - Company/organization details

## Input Format

The command accepts two JSON formats:

1. **Wrapped format** (from `gog contacts get --json`):
```json
{
  "contact": {
    "resourceName": "people/c123456",
    "names": [...],
    "emailAddresses": [...]
  }
}
```

2. **Direct Person object**:
```json
{
  "resourceName": "people/c123456",
  "names": [...],
  "emailAddresses": [...]
}
```

## Examples

### Add a website URL
```bash
gog contacts get people/c123 --json | \
  jq '.contact.urls = [{"value": "https://example.com", "type": "homePage"}]' | \
  gog contacts update people/c123 --from-file=-
```

### Add a note/biography
```bash
gog contacts get people/c123 --json | \
  jq '.contact.biographies = [{"value": "Met at conference 2026"}]' | \
  gog contacts update people/c123 --from-file=-
```

### Add Obsidian deep link
```bash
gog contacts get people/c123 --json | \
  jq '.contact.urls += [{"value": "obsidian://open?vault=notes&file=People/John%20Doe", "type": "profile"}]' | \
  gog contacts update people/c123 --from-file=-
```

## Benefits

- **Supports all Google People API fields** without needing individual CLI flags
- **Future-proof** - new API fields work automatically
- **Scriptable** - easy to automate with `jq` or other JSON tools
- **Batch operations** - process multiple contacts with scripts
- **Complex updates** - modify nested structures easily

## Note

This feature was implemented to provide full access to the Google People API without requiring individual CLI flags for every possible field. User is responsible for ensuring the JSON structure matches the Google People API format.
