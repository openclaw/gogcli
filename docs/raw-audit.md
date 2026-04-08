# `gog <group> raw` — Sensitive Field Audit

This document records the security audit performed before shipping the
`gog <group> raw <id>` subcommands, which dump the canonical Google API
response as JSON for programmatic / LLM consumption.

## Redaction rule

`raw` applies field-level redaction **only when the user did not explicitly
name a field via `--fields`**. Rationale:

- The default (implicit `fields=*` for Drive, or no mask for the other
  APIs) pulls in capability URLs and third-party–stashed metadata that
  callers rarely want and can leak if piped into an LLM, shared in a bug
  report, or committed to a repo.
- When a caller writes `--fields "id,name,thumbnailLink"`, they named
  `thumbnailLink` deliberately. Redacting a user-named field would be
  surprising and user-hostile.

Summary: **redact what the user didn't ask for; honor what they did.**

## Per-endpoint findings (PR #1 scope)

### 1. `docs.Documents.Get` — `gog docs raw`

REST ref: <https://developers.google.com/docs/api/reference/rest/v1/documents/get>
Go type: <https://pkg.go.dev/google.golang.org/api/docs/v1#Document>

| Field | Risk | Default handling |
|---|---|---|
| `inlineObjects.*.embeddedObject.imageProperties.contentUri` | Short-lived (~30 min) bearer-style authenticated image URL | Redact |
| `inlineObjects.*.embeddedObject.imageProperties.sourceUri` | May reference private source URLs | Redact |

No credentials, tokens, or OAuth metadata in the response. Document body,
named ranges, suggestions, headers/footers are user content the caller
already has read access to — ship as-is.

### 2. `sheets.Spreadsheets.Get` — `gog sheets raw`

REST ref: <https://developers.google.com/sheets/api/reference/rest/v4/spreadsheets/get>
Go type: <https://pkg.go.dev/google.golang.org/api/sheets/v4#Spreadsheet>

| Field | Risk | Default handling |
|---|---|---|
| `developerMetadata` | Third-party apps may stash arbitrary KV including secrets | **Warn on stderr if present**, do not redact |
| `sheets[].data.rowData.values[].userEnteredValue.formulaValue` (only with `--include-grid-data`) | Formulas can embed API keys via `IMPORTRANGE`, hardcoded tokens in cells | **Warn on stderr when `--include-grid-data` is set**, do not redact |

`--include-grid-data` is **off by default** because grid payloads can be
multi-MB and are the primary leakage vector.

No redaction applied to Sheets output; warnings only. Redacting cell
content would defeat the purpose of a lossless dump.

### 3. `slides.Presentations.Get` — `gog slides raw`

REST ref: <https://developers.google.com/slides/api/reference/rest/v1/presentations/get>
Go type: <https://pkg.go.dev/google.golang.org/api/slides/v1#Presentation>

| Field | Risk | Default handling |
|---|---|---|
| `slides[].pageElements[].image.contentUrl` | Short-lived authenticated image URL (same class as Docs `contentUri`) | Redact |
| `slides[].pageElements[].image.sourceUrl` | Possibly private origin URL | Redact |
| `slides[].pageElements[].video.url` | Drive video refs may carry signed access | Redact |

### 4. `drive.Files.Get` with `fields=*` — `gog drive raw` *(highest risk)*

REST ref: <https://developers.google.com/drive/api/reference/rest/v3/files/get>
Go type: <https://pkg.go.dev/google.golang.org/api/drive/v3#File>

| Field | Risk | Default handling |
|---|---|---|
| `thumbnailLink` | Time-limited signed URL that bypasses normal auth for ~hours. Classic leak vector. | Redact |
| `webContentLink` | Direct download URL; capability URL | Redact |
| `exportLinks` | Per-MIME authenticated export URLs | Redact |
| `resourceKey` | Capability token for link-shared files; effectively a shared secret | Redact |
| `appProperties` | Arbitrary app-stashed KV; apps commonly misuse for secrets | Redact |
| `properties` | Public custom properties, still frequently (mis)used for tokens | Redact |
| `contentHints.thumbnail.image` | Base64 thumbnail bytes; large and unnecessary | Redact |
| `permissions[].emailAddress`, `owners[].emailAddress`, `sharingUser`, `lastModifyingUser`, `trashingUser` | Non-collaborator emails (PII) when `fields=*` enumerates full ACL | Not redacted — caller already has access to the file; enumeration is a conscious `--fields` choice |

**Reminder:** all of the above are redacted **only when `--fields` is not
set**. Passing `--fields "id,name,thumbnailLink"` returns `thumbnailLink`
verbatim — the user named it.

## Cross-cutting observations

- Google APIs never return OAuth access tokens, refresh tokens, or client
  secrets in resource responses. The risk is capability URLs and
  app-stashed custom metadata, not credential disclosure in the API
  contract itself.
- `gog drive raw` is the most dangerous command; the others are modest in
  comparison.
- PR #2 will extend this audit to gmail / calendar / people / contacts /
  tasks / forms before those commands ship.
