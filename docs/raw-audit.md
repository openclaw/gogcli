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

## Per-endpoint Findings: Workspace Content

### 1. `docs.Documents.Get` — `gog docs raw`

REST ref: <https://developers.google.com/docs/api/reference/rest/v1/documents/get>
Go type: <https://pkg.go.dev/google.golang.org/api/docs/v1#Document>

| Field | Risk | Default handling |
|---|---|---|
| `inlineObjects.*.embeddedObject.imageProperties.contentUri` | Short-lived (~30 min) bearer-style authenticated image URL | **Ship as-is** |
| `inlineObjects.*.embeddedObject.imageProperties.sourceUri` | May reference private source URLs | **Ship as-is** |

**Why not redact:** the Docs API has no field mask, so the principled
"redact only what the user didn't name" rule has no escape hatch.
These image URIs are short-lived (~30 min) and require the caller's
auth anyway — substantially lower risk than Drive's `thumbnailLink`.
The lossless guarantee is valued more than the marginal hardening.

No credentials, tokens, or OAuth metadata in the response.

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
| `slides[].pageElements[].image.contentUrl` | Short-lived authenticated image URL (same class as Docs `contentUri`) | **Ship as-is** |
| `slides[].pageElements[].image.sourceUrl` | Possibly private origin URL | **Ship as-is** |
| `slides[].pageElements[].video.url` | Drive video refs may carry signed access | **Ship as-is** |

Same rationale as Docs: no field mask, short-lived URLs, auth-gated,
lower risk than Drive. Lossless guarantee preferred.

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

## Per-endpoint Findings: Other Services

### 5. `gmail.Users.Messages.Get` — `gog gmail raw`

REST ref: <https://developers.google.com/gmail/api/reference/rest/v1/users.messages/get>
Go type: <https://pkg.go.dev/google.golang.org/api/gmail/v1#Message>

The subcommand name "raw" collides with Gmail's native `format=raw`
(base64url-encoded RFC822 blob). `gog gmail raw` defaults to
`format=FULL` (full parsed Message struct) and exposes `--format
full|metadata|minimal|raw` for users who want Gmail's RAW. Help text
documents both senses.

| Field | Risk | Default handling |
|---|---|---|
| `payload.body.data` (base64url) | Email body; user already has read access | Ship as-is |
| `payload.headers` | May contain `Received-SPF`, `DKIM-Signature`, routing metadata | Ship as-is |
| `raw` (when `--format=raw`) | Full RFC822 source including original attachments | Ship as-is — user asked for it |

No credential leakage risk. Caller already holds the Gmail scope.

### 6. `calendar.Events.Get` — `gog calendar raw`

REST ref: <https://developers.google.com/calendar/api/v3/reference/events/get>
Go type: <https://pkg.go.dev/google.golang.org/api/calendar/v3#Event>

| Field | Risk | Default handling |
|---|---|---|
| `attendees[].email` | Attendee emails (PII) | Ship as-is — caller already on the event ACL |
| `conferenceData.entryPoints[].uri` | Meeting URLs (Meet/Zoom) including passwords in params | Ship as-is — user asked for the event |
| `extendedProperties.private` / `extendedProperties.shared` | App-stashed KV; third-party apps may use for secrets | Ship as-is (same rationale as Sheets `developerMetadata`) |

No redaction. The risk surface here is fundamentally the same as
simply reading the event in the Calendar UI.

### 7. `people.People.Get` — `gog people raw` / `gog contacts raw`

REST ref: <https://developers.google.com/people/api/rest/v1/people/get>
Go type: <https://pkg.go.dev/google.golang.org/api/people/v1#Person>

Both `gog people raw` and `gog contacts raw` call the same underlying
`people.Get` endpoint. Requires `--person-fields` (Google's field mask
for the People API, required on every request).

| Field | Risk | Default handling |
|---|---|---|
| `emailAddresses`, `phoneNumbers`, `addresses`, `biographies` | PII; user's own contacts | Ship as-is |
| `userDefined[]` | Arbitrary KV custom fields; may store secrets | Ship as-is |
| `metadata.sources[].profileMetadata.userTypes` | Account type disclosure | Ship as-is |

### 8. `tasks.Tasks.Get` — `gog tasks raw`

REST ref: <https://developers.google.com/tasks/reference/rest/v1/tasks/get>
Go type: <https://pkg.go.dev/google.golang.org/api/tasks/v1#Task>

| Field | Risk | Default handling |
|---|---|---|
| `notes` | User-entered free text | Ship as-is |
| `links[].link` | External URLs attached to a task | Ship as-is |

No sensitivity concerns beyond the caller's own task data.

### 9. `forms.Forms.Get` — `gog forms raw`

REST ref: <https://developers.google.com/forms/api/reference/rest/v1/forms/get>
Go type: <https://pkg.go.dev/google.golang.org/api/forms/v1#Form>

| Field | Risk | Default handling |
|---|---|---|
| `items[].questionItem.question.grading` | Correct answers for graded forms | Ship as-is — caller is the form owner |
| `linkedSheetId` | ID of the responses spreadsheet | Ship as-is |

No redaction. The form owner already has access to everything here.

## Cross-cutting observations

- Google APIs never return OAuth access tokens, refresh tokens, or client
  secrets in resource responses. The risk is capability URLs and
  app-stashed custom metadata, not credential disclosure in the API
  contract itself.
- `gog drive raw` is the most dangerous command; the others are modest in
  comparison.
- This audit covers all currently shipped `raw` subcommands.
