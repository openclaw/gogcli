# Changelog

## 0.24.1 - Unreleased

### Added

- Photos: add an explicit-opt-in Google Photos Picker workflow for creating selection sessions, waiting for completion, listing chosen media, and downloading selected files. (#754)
- Docs: add persisted, revision-locked request batches for composing supported mutations locally and submitting them atomically, with explicit split and partial-recovery modes. (#755)
- CLI: remove the separate `gog agent` and `exit-codes` helpers; expose stable exit codes and effective automation safety state through `gog schema --json`, and summarize the contract in root help. (#677)
- CLI: add Git-style `gog help <command>`, make explicit output flags override environment defaults, validate color and JSON-only transforms before command execution, report early usage errors on stderr, and reject contradictory schema plain output.
- Docs: prevent multi-paragraph Markdown range replacements from inheriting the matched paragraph's heading or list style. (#756) — thanks @sebsnyk.
- Docs: preserve nested Markdown list levels as native bullets inside imported and updated table cells. (#749) — thanks @sebsnyk.
- Gmail: add explicit `gmail archive --thread` semantics so IDs from thread search can archive every message in each thread. (#752) — thanks @sebsnyk.
- Drive/Docs: add persisted polling for Drive changes and Docs comments, with bounded runs, filters, retry-safe cursors, and sequential JSON hooks. (#690, #751)
- Drive: expose shortcut targets in JSON and human-readable folder reports without changing stable `--plain` columns, classify shortcuts distinctly, keep tree scans from following folder targets, and add `drive shortcut create`. (#763)

## 0.24.0 - 2026-06-11

### Added

- Calendar: add repeatable `--attachment` to `calendar update` for replacing or clearing event attachments. (#738) — thanks @TreyLawrence.
- Sheets: add `sheets validation` get/set/clear commands for dropdown, checkbox, number, date, range, and custom-formula rules, and preserve table-managed dropdowns during validation-only copy/paste. (#710) — thanks @chrischall.
- Sheets: add table-aware `sheets delete-dimension` for deleting row or column spans while preserving intersecting table objects and remaining data. (#711) — thanks @chrischall.
- Docs: add direct `docs table-row`, `docs table-column`, `docs table-merge`, and `docs table-unmerge` commands with index, header-text, all-table, and tab-aware selection. (#686) — thanks @sebsnyk.
- Docs: add `docs named-range` create/list/delete/replace commands for durable, tab-aware document anchors. (#692) — thanks @sebsnyk.
- Gmail: report attached filenames and byte sizes in JSON results for send and draft create/update. (#716) — thanks @chrischall.
- Gmail: add `gmail watch pull` for Pub/Sub pull subscription consumers with hook retry support. (#700) — thanks @joshp123.
- Docs: add `--tab` and `--all-tabs` to `docs raw` for inspecting specific or complete multi-tab document content. (#697) — thanks @sebsnyk.
- Docs: add tab-aware table, image, heading, and paragraph enumerators with structured and plain output. (#719) — thanks @sebsnyk.
- Docs: style locally rendered fenced Markdown blocks with Roboto Mono, dark-green text, and existing paragraph shading. (#676, #724) — thanks @TurboTheTurtle.
- Docs: add `docs insert-image --url` for inserting public HTTPS images directly without Drive upload or temporary public sharing. (#675) — thanks @sebsnyk.
- Docs: expose paragraph emptiness and text-run ranges, styles, and links in `docs paragraphs list --json`. (#734) — thanks @sebsnyk.
- Docs: add opt-in `--check-orphans` to Markdown replacement writes so open comments whose quoted text would disappear block the mutation with orphaned exit code 11. (#691) — thanks @sebsnyk.
- Drive: add `drive revisions list|get` for paged revision metadata and provider export links. (#672) — thanks @aaroneden.

### Fixed

- Auth: bind browser, manual, remote, and account-manager OAuth exchanges with S256 PKCE; unfinished pre-PKCE manual flows must restart at step 1. (#693, #725) — thanks @TurboTheTurtle.
- Docs: reset inherited text styles before applying Markdown find-replace formatting so leading bold spans and later inline styles stay paired correctly. (#735) — thanks @sebsnyk.
- Docs: accept leading-dash Markdown list values in `docs cell-update --content` and reject nonempty Markdown that produces no editable cell text. (#733) — thanks @sebsnyk.
- Docs: keep inline Markdown find-replace fragments inside their existing paragraph unless the replacement explicitly ends with a newline. (#736) — thanks @sebsnyk.
- Docs: render HTML `<br>` variants as line breaks inside Markdown table cells while preserving protected literals. (#730) — thanks @sebsnyk.
- Docs: avoid duplicate empty paragraphs adjacent to Markdown headings while preserving body paragraph spacing. (#717, #720) — thanks @TurboTheTurtle.
- Auth: repair duplicate macOS Keychain writes for legacy and subject token aliases without weakening primary token persistence. (#718, #721) — thanks @TurboTheTurtle.

## 0.23.0 - 2026-06-09

### Added

- CI: enforce a pinned Go dead-code check and remove the unreachable helpers it identified. (#714) — thanks @vincentkoc.
- Chat: add repeatable `--attach` to `chat messages send` for sending local files with Google Chat messages. (#694) — thanks @omothm.
- Docs: add `docs comments locate` to resolve comment quotes to Docs API index ranges and report orphaned comments. (#687) — thanks @sebsnyk.
- Docs: add `docs find-range` to map matched text to Docs API UTF-16 index ranges. (#682) — thanks @sebsnyk.
- Docs: add `--at`, `--occurrence`, and `--match-case` anchors to `docs insert`, `docs delete`, `docs update`, `docs insert-person`, and `docs insert-page-break`. (#683) — thanks @sebsnyk.
- Docs: add `--link` and `--no-link` to `docs format` for setting or clearing hyperlinks on matched text. (#684) — thanks @sebsnyk.
- Sheets: add `sheets links set` to write single-cell, multi-link rich-text, and batch hyperlinks. (#713) — thanks @chrischall.
- Slides: add `slides insert-image` to place a positioned, sized local image on an existing slide. (#695) — thanks @Czaruno.

### Fixed

- Docs: avoid duplicate empty paragraphs around Markdown tables written into a specific tab. (#715) — thanks @sebsnyk.
- Sheets: prevent accidental table data loss by requiring explicit `--discard-data` for `sheets table delete`, matching the Sheets API's destructive table-delete semantics. (#709) — thanks @chrischall.

## 0.22.0 - 2026-06-07

### Added

- Docs: add `--code` to `docs format` and plain-text `docs write` for the existing monospace grey code style. (#685) — thanks @sebsnyk.
- Drive/Docs: add `--since` to `drive comments list` and `docs comments list` for server-side comment modified-time filtering. (#688) — thanks @sebsnyk.
- Gmail: add `--thread-id` to `gmail drafts create` and `gmail drafts update` so drafts can reply within a thread using the latest message headers. (#673, #674) — thanks @chrischall.

### Fixed

- Docs: preserve nested list levels when writing markdown into a specific tab with `docs write --replace --markdown --tab`. (#696)
- Docs: fix `docs export --tab` tab resolution against the live Docs API field mask. (#696)
- Docs: strip Pandoc-style explicit heading anchors like `{#slug}` from rendered markdown headings and resolve matching same-document links. (#703)
- Docs: render GFM `~~strikethrough~~` spans in the local markdown writer used by `docs write --tab --markdown`. (#702)
- Docs: batch table-cell writes for `docs write --tab --markdown` to avoid per-cell Docs API quota bursts on table-heavy documents. (#699) — thanks @sebsnyk.
- Gmail: preserve existing `gmail drafts update` attachments when `--attach` is omitted, add `--clear-attachments` for intentional removal, and keep `--attach` as explicit replacement. (#680, #681) — thanks @chrischall.

## 0.21.0 - 2026-06-01

### Added

- MCP: add a typed, allowlisted `gog mcp` stdio server with read-only defaults and explicit write-tool opt-in. (#637) — thanks @auroracapital.
- Docs: add `docs table-column-width` to set fixed native table column widths or reset columns to evenly distributed sizing. (#631) — thanks @sebsnyk.

### Fixed

- Agent/MCP: fix command-allowlist docs so examples include `mcp`, keep wildcard tool selectors shell-safe, and report public dry-run op paths for service-account, Calendar, Forms, Meet, Sheets named ranges, and Docs/Sheets/Slides copy commands.
- Auth/credentials: return usage errors for unknown `--services` values and invalid service-account key JSON, keep `auth keep --dry-run` file-free, and make `zoom auth setup --dry-run` emit a redacted no-write plan.
- Backup/config: make backup init/export/push validation fail before repository or OAuth side effects, keep `backup init --dry-run` and `--no-push` offline/local-only, preserve semantic manifest counts during verify/export, and return usage errors for invalid config keys or values.
- CLI: preserve command-local `--fields` API masks, keep `open --type` shortcuts from rewriting unsupported URLs into malformed Google editor links, and stop advertising `ads` as an API command service while retaining it as an auth-only scope.
- Validation: consistently return usage exit code 2 before auth, API-key lookup, dry-run success, or mutation for invalid list limits, empty IDs/queries, malformed dates/timezones/recurrence, invalid enum flags, invalid resource paths, malformed JSON, unsupported formats, immutable labels, and unsafe local-file Markdown image references across Admin, Calendar, Chat, Contacts, Docs, Drive, Drive Activity, Drive Labels, Forms, Gmail, Groups, Keep, Maps, People, Photos, Search Console, Sheets, Slides, Tasks, Time, and YouTube.
- Dry-run safety: validate Gmail/Contacts/Chat/Admin email inputs, Drive share targets, Drive changes watch URLs/expiration, Docs comment anchors, Sheets chart anchors/ranges, and Search Console sitemap URLs before reporting dry-run success.
- JSON output: return empty arrays instead of null for empty Calendar conflicts, Classroom lists, Forms responses/watches, Gmail settings/filter/thread-attachment results, People relations, blank Sheets ranges, blank Slides text/image lists, and empty YouTube list responses.
- Calendar: make `calendar conflicts` check all calendars by default, reject explicit one-calendar conflict checks, reject unsupported all-day/date-only Out of Office payloads locally, and return usage errors when response/propose-time actions cannot be applied.
- Contacts: warm the People API contact-search cache before contact, other-contact, and Gmail `--from-contact` searches; resolve `contacts raw <email>` and `people raw <email>` to contact resources; and use an other-contact-safe read mask for other-contact list/search.
- Classroom: reject unfiltered `classroom invitations list` locally, report the canonical hyphenated dry-run op for `guardian-invitations create`, and normalize empty list output.
- Docs: validate `docs sed`, `docs cell-style`, and `docs table-column-width` table targets locally; reject malformed sed expressions; and fail Markdown writes with local image references that must be public HTTPS URLs.
- Drive: validate download/export/upload combinations before API calls, validate comment/permission limits, validate reporting/audit/bulk scan bounds, and reject invalid Drive Label field values including fractional integers, malformed dates/users, malformed `--fields-json`, and trailing JSON tokens.
- Gmail: validate vacation, auto-forward, forwarding, send-as, delegation, filter forwarding, compose headers, message formats, batch-modify labels, history cursors, tracked-send setup options, and immutable label operations locally; keep `gmail track setup --dry-run` offline and make tracking setup/status/key rotation honor `--json` without leaking generated secrets.
- Maps: validate mode, units, and reverse-geocode coordinates before API-key lookup, and share a generic Maps/Places API-key setup error with Calendar Places commands.
- Sheets: infer `sheets format --format-fields` from `--format-json`, validate update/append/table values and JSON specs locally, reject invalid field masks/ranges/anchors/type flags, and reject explicit negative freeze counts instead of treating `-1` as an unset sentinel.
- Slides: make local-image insertion/replacement use stable Drive download URLs and retry while sharing propagates, avoid invalid speaker-notes `deleteText` requests on blank notes pages, make notes/slide deletion commands return valid JSON, and require `--force` for non-interactive slide deletion.
- YouTube: let `activities list --channel-id`, `playlists list --channel-id`, and `channels list --id` honor `--account` OAuth; filter `youtube search list --type` to requested resource kinds; and validate blank IDs/type lists/chart regions before auth or API-key setup.

## 0.20.0 - 2026-05-30

### Fixed

- Gmail: keep label IDs case-sensitive during label resolution and duplicate-name checks while still matching label names case-insensitively.
- Gmail: clarify that `gmail drafts delete` permanently deletes drafts and cannot be recovered. (#656, #659) — thanks @chrischall.
- Sheets: add `--inherit-from-before` to `sheets insert` so callers can choose whether inserted rows/columns inherit formatting from the preceding or following neighbor. (#655, #658) — thanks @chrischall.
- Sheets: add `sheets copy-paste` / `fill` for range-level CopyPasteRequest fills of values, formulas, formatting, and related paste types. (#661, #663) — thanks @chrischall.
- CLI: add `--enable-commands-exact` / `GOG_ENABLE_COMMANDS_EXACT` for strict command allowlists without prefix expansion. (#652) — thanks @jason-allen-oneal.
- Auth: update stored OAuth scope metadata from observed granted scopes during refresh so `auth list` reflects newly usable services. (#649)
- Docs: preserve paragraph-separating blank lines when replacing a single tab from Markdown. (#644)
- Docs: add `docs cell-update` for non-destructive table-cell content replacement by table, row, and column. (#646)
- Docs: add `docs update --markdown` and `--replace-range` for formatted insertion and range replacement. (#642) — thanks @rel.
- Gmail: pause watch push Gmail API fetches per account while a 429 Retry-After circuit is open. (#643)
- YouTube: let `videos list` and `comments list` use OAuth when `--account` is supplied, preserving the API-key fallback for unauthenticated public reads. (#664)
- YouTube: add `youtube search list` / `yt search ls` for YouTube Data API search across videos, channels, and playlists. (#650, #651) — thanks @BRO3886.
- Gmail: add `gmail search --from-contact` to resolve a contact's email addresses into a `from:(...)` OR query. (#657) — thanks @chrischall.
- Docs: add named `--page-size` presets for `docs write` and `docs page-layout`. (#640) — thanks @sebsnyk.
- Docs: add smart-chip insertion commands for person, Drive file, and date chips. (#638) — thanks @sebsnyk.
- Docs: add `docs cell-style` for table-cell background color and inline cell text styling. (#645) — thanks @sebsnyk.
- Docs: add `docs insert-image` to upload a local image, temporarily share it for Docs insertion, and revoke the public permission afterward. (#648) — thanks @sebsnyk.
- Docs: update the bundled `gog` agent skill to preserve broad user OAuth scopes during reauth and rely on command guards for scoped execution.

## 0.19.0 - 2026-05-22

### Added

- Auth: store Google OAuth `client_secret` values in the keyring by default while leaving only client metadata on disk; legacy plaintext credentials still read and `auth credentials set --insecure` preserves the old write shape. (#596)
- Auth: add `auth credentials set --expand-env` for strict environment placeholder expansion in OAuth client JSON. (#599)
- Auth: let `auth import` seed an initial access token and expiry, and round-trip cached access tokens through token export/import. (#598)
- CLI: add XDG kind-aware config/data/state/cache paths with `GOG_HOME`, per-kind `GOG_*_DIR` overrides, and `--home` while preserving legacy auth/keyring/service-account reads. (#621, #622) — thanks @alexminza.
- Docs: add explicit `--page-width`, `--page-height`, and page margin flags to `docs write` and `docs page-layout`, keeping `--pageless` width unchanged unless requested. (#629, #630) — thanks @sebsnyk.

### Fixed

- People: fall back to token identity when `gog me` / `gog whoami` hit a disabled People API on the OAuth client project. (#460, #461)
- Docs: drop all-whitespace Markdown table header rows during Docs markdown writes, and rewrite same-document `#heading-slug` links to native Google Docs heading links after Drive markdown import. (#632, #633) — thanks @sebsnyk.
- Gmail: include attachment metadata in `gmail messages search --include-body --json` results. (#620)
- Auth: let `auth service-account set` read service account keys from stdin (`--key=-` or `--key-stdin`) or an environment variable (`--key-env`). (#600)
- Auth: serialize file-keyring reads and writes with a shared lock so concurrent `gog` processes cannot observe partial keyring entries or clobber multi-key token updates. (#597)
- Release: verify the OpenClaw Homebrew tap checkout when checking `gogcli` formula assets.

## 0.18.0 - 2026-05-22

### Added

- Docs: add `VISION.md` with project fit, discussion, and live-test merge guidance.
- Calendar: add --with-zoom / --regenerate-zoom / --remove-zoom that create, regenerate, and remove Zoom meetings and attach the join URL + meeting ID + passcode to the Calendar event description. Google's Calendar API rejects conferenceData writes asserting `conferenceSolution.key.type="addOn"` from non-Workspace-Marketplace OAuth clients, so the description-mode integration is the path that round-trips through Google's storage; trade-off is no native "Join with Zoom" conference card. (#589, #590) — thanks @alexisperumal and @mvanhorn.
- Auth: add gog zoom auth setup / doctor for Zoom S2S OAuth credential storage. (#590) — thanks @mvanhorn.
- Drive: add `--action=resolve|reopen` to `drive comments reply` and sibling `drive comments resolve|reopen` verbs (also `docs comments reopen`) to post a reply that atomically flips the parent comment's resolved state via the Drive API's `Reply.action` field. Avoids the previous workaround of `drive comments delete` (which destroys review-thread context) for batch-resolving inline doc-review feedback. (#623) — thanks @sebsnyk.
- Sheets: add `gog sheets batch-update <spreadsheetId> --data-json ...` for updating multiple value ranges in one Sheets API request. Alias: `batch`. (#601) — thanks @Tsopic.
- Docs: add `gog docs insert-page-break <docId> [--index N | --at-end] [--tab=STRING]` to insert a Google Docs page break directly via `InsertPageBreakRequest` — markdown has no native page-break construct, so this is the only path for multi-page deliverables. Aliases: `page-break`, `pb`. (#604)
- Docs: add `gog docs page-layout <docId> [--layout=pageless|pages]` to toggle the page layout of an existing Google Doc via `updateDocumentStyle` on `documentFormat.documentMode`. Sibling to the existing `--pageless` flag on `docs create`/`write`/`update` for the case where the doc was created upstream (e.g. by Drive markdown conversion) without the desired layout. Defaults to `pageless`. Aliases: `set-page-layout`, `page-setup`. (#593)
- Docs: add `--heading-level N` (1..6 shortcut) and `--named-style NAME` (full enum) to `gog docs format` so existing paragraphs can be promoted to `HEADING_1`..`HEADING_6`, `TITLE`, `SUBTITLE`, or `NORMAL_TEXT`. Both set `paragraphStyle.namedStyleType` on the existing UpdateParagraphStyle request and compose cleanly with `--alignment` / `--line-spacing`. (#605)
- Sheets: add `gog sheets reorder-tab <spreadsheetId> --tab=<name|sheetId> --to=N` to move a tab to a specific 0-based position via `updateSheetProperties` with field mask `index`. `--tab` accepts a title or a numeric sheet ID; `--to=0` is force-sent so the leftmost target reaches the wire as `"index":0`. Aliases: `move-tab`, `reorder-sheet`, `move-sheet`. (#603)
- Docs: add `gog docs insert-table <docId> --rows N --cols M [--index N | --at-end] [--values-json [[...]]] [--tab=STRING]` to insert a native Google Docs table directly via `InsertTableRequest`, bypassing the markdown writer. `--values-json` takes a JSON 2D string array whose dimensions must match `--rows`x`--cols`. Empty `--values-json` produces an empty table structure. (#602)
- Docs: `gog docs write --replace --markdown --tab=<tab>` now performs a whole-tab re-render — the targeted tab's existing body is wiped via `DeleteContentRange` and the markdown is re-rendered locally via the same Docs `batchUpdate` path used by `--append --markdown`, so other tabs are untouched. Previously this combination errored because Drive's markdown converter operates on entire documents only. (#595)

### Fixed

- Docs: make generated command references ignore local keyring config so `make ci` stays clean across developer machines.
- CLI: harden backup writes, config/credentials atomic saves, keyring write verification, line input buffering, disabled-API hints, JSON transform number handling, and untrusted-content wrapping after ClawPatch review.
- CLI: bound retry request replay buffering, recover failed async backup pushes, ignore global git commit signing in backup snapshots, and protect account manager OAuth redirects with CSRF checks.
- Release: update the Homebrew handoff to publish through `openclaw/tap`.
- Version: `gog --version` now reports an informative fallback (for example, `v0.17.0-dev`) when built from source with plain `go build` instead of returning `dev`.
- Docs: `gog docs insert` now defaults to end-of-doc when `--index` is omitted, instead of always inserting at position 1 (which silently reversed iterative inserts across multiple calls). Pass `--index 1` explicitly to keep the previous behaviour. (#606)
- Docs: `docs write --append --markdown` with three or more markdown tables in a single render no longer drifts the per-table insertion offset by one character per table — the trailing punctuation of the paragraph immediately before the third (and any subsequent) table is preserved instead of being split into a standalone paragraph after the table. (#607)
- Docs: `docs write --append --markdown` now expands inline markdown markers (`**bold**`, `*italic*`, `` `code` ``, `[link](url)`) inside table cells into character runs, matching the behaviour outside of tables — previously the markers rendered as literal characters because the table inserter bypassed the inline-formatting pass. (#608)
- Docs: markdown empty-header table rows (e.g. `|   |   |`) no longer collide with the separator detection — previously `docs write --append --markdown` swallowed both the empty header and the real `|---|---|` separator, leaving the last data row re-parsed as a literal pipe paragraph after the table. (#609)
- Docs: `docs write --append --markdown` no longer silently drops tables with `insert native table: table not found near index N`. The native-table inserter's post-write search used a ±2 code-unit window, but the Docs API's actual table StartIndex can drift further (auto-newline + placeholder paragraph combine to a several-unit shift); the search now picks the closest forward Table element with matching dimensions and a small backward tolerance instead. The `docs create --file --markdown` path was unaffected because it uses Drive's native markdown import end-to-end. (#592) — thanks @sebsnyk.
- Docs: `docs write --append --markdown` now renders bullet lists as native `BULLET` paragraphs (via `CreateParagraphBullets`) and fenced code blocks as a single contiguous shaded paragraph (joining lines with vertical-tab soft breaks). Previously bullets came through as `NORMAL_TEXT` paragraphs with a literal `•` glyph in the text run, and each code-block line became its own one-line `Courier New` paragraph with no paragraph-level background. (#594) — thanks @sebsnyk.

## 0.17.0 - 2026-05-15

### Added

- `slides create-from-markdown`: import slidey-flavored decks with per-slide YAML frontmatter (`layout:`, `content:`), `## Notes` speaker notes, Font Awesome icon shortcodes, mermaid diagrams, `::cols::`/`::col2::`/`::col3::`/`::right::` columns, and `::boxes::`/`::arrows::` icon-row blocks. New flags: `--fa-style`, `--mmdc`, `--strict`, `--keep-temp-images`, `--no-notes` — thanks @njreid.
- Calendar: add `calendar events --sort=start|end|summary|calendar` and `--order=asc|desc` so `--all` output can be returned chronologically across calendars instead of per-calendar API iteration order. Also documents `now` in the `--from`/`--to` help strings (already accepted by `timeparse`) — the relative form agents need when planning "from now on" — thanks @gado-ships-it.
- Calendar: add `calendar events --location` to include event locations in table output. Embedded newlines in the location string are collapsed so multi-line addresses still render on one row — thanks @gado-ships-it.
- Auth: add `gog auth import --client --email` with `--refresh-token-stdin`, `--refresh-token-file`, or `--refresh-token-env` for non-interactive token import without exposing secrets in argv — thanks @jcarnegie.
- Drive: add `drive share --notify` for invite targets that require a Drive notification email.
- Calendar: keep `calendar appointments` as an explicit diagnostic because the Calendar API still rejects `eventTypes=appointmentSchedule`. (#329)
- CLI: add nested `docs tabs ...` and `forms questions ...` aliases for consistent sub-item command patterns while preserving existing flat commands. (#433)
- Drive: add `drive audit sharing|user` plus guarded `drive bulk remove-public|update-role` permission operations with dry-run and confirmation support. (#336)
- Drive: add `drive labels file list|apply|remove` alongside Drive Labels API v2 discovery. (#339)
- Maps: add directions, distance matrix, geocode, and reverse-geocode commands alongside Places search/details. (#571)
- Photos: add read-only `photos list|search|get|download` for app-created Google Photos media. (#381)

### Fixed

- CLI: make mutating dry-runs for contacts, Docs, Drive, Meet, and Slides stop before auth/API calls while still validating local inputs; harden live smoke tests for self-sharing, disabled Meet, Gmail filter labels, and forced batch deletes.
- CLI: make `drive upload`, `drive bulk remove-public/update-role`, `calendar subscribe`, `docs clear`, `slides create-from-markdown`, `slides insert-text`, `slides replace-text`, `auth tokens import`, and Gmail tracking key rotation dry-runs use the standard parseable dry-run envelope without auth/API access.
- CLI: keep additional Docs write/update/delete/format/find-replace, Drive mkdir/changes, Gmail label create/modify, and Slides add/delete/replace/update-notes dry-runs offline before auth/API calls.
- CLI: give destructive Classroom, Gmail, Keep, and Tasks dry-runs stable JSON operation names and structured request payloads instead of prose `op` values with null requests.
- CLI: keep Docs tab edits, Sheets tab deletes, Drive deletes, comment deletes, auth removals, Gmail delegate/watch removals, Classroom guardian deletes, and other-contact deletes dry-run parseable without auth/API access.
- CLI: make dry-runs for Gmail label edits, Sheets table deletes, Sheets banding/conditional clears, and Forms deletes stop before auth/API calls, and make Forms dry-runs validate choice, scale, quiz, and empty update inputs locally.
- CLI: make dry-runs for Calendar secondary calendars, Forms create/publish/watch/move, Gmail label delete, Sheets table append/clear, Sheets named-range edits, Apps Script create, and Slides template creation stay offline before auth/API calls.
- Calendar: keep `calendar create/update --dry-run` with `--location-search` or `--place-id` offline before Places API lookup while still validating the requested lookup.
- CLI: make dry-runs for Admin group/user/org-unit edits, Contacts delete, Docs tab export, Drive tab download/share/unshare, and Gmail watch renew stay offline before auth/API calls; redact Admin user create passwords in dry-run output.
- Auth: keep fresh OAuth saves working even when old file-keyring token entries are unreadable, and clarify that `--services all` means all user OAuth services while Workspace-only services use service accounts.
- Auth: include Chat reaction scopes in `--services chat` and keep the generated auth scope table freshness-tested.
- Auth: keep the accounts manager bound to loopback addresses, generate callback URLs from the actual listener host, and avoid deleting renamed-account tokens before replacements are stored.
- Gmail: reject off-palette `gmail labels style` colors locally instead of forwarding an opaque Gmail API error.
- Drive: make `drive share --dry-run` stop before permission creation for user and domain shares, including `--notify`.
- Forms: make `forms create --description` apply the description with a follow-up batch update, and preserve zero-valued indexes in `forms move-question`.
- Analytics: show Analytics Admin/Data API enablement hints instead of an Admin SDK hint for GA API-disabled errors.
- Docs: make `docs find-replace --format markdown` strip inline and block Markdown markers while preserving nested bold/italic/code/link formatting in the inserted Google Doc content. (#586) — thanks @sebsnyk.
- YouTube: preserve API-key authentication when wrapping requests with the retry transport, so public `youtube`/`yt` reads no longer fail as unregistered callers. (#578) — thanks @adityarya24.
- Docs: update OAuth success/accounts GitHub links to the `openclaw/gogcli` repository. (#561)

## 0.16.0 - 2026-05-10

### Added

- Admin: expand `admin users create` with GAM-style aliases, generated passwords, suspended/archived creation, recovery contact fields, and password hash metadata; add `admin users delete` for cleanup.
- Admin: add `admin orgunits` commands to list, inspect, create, update, and delete Workspace organizational units.
- Sites: add Drive-backed `sites` commands to list, search, inspect, and open New Google Sites. (#574) — thanks @thewilloftheshadow.
- Analytics/Search Console: add GA4 `analytics accounts|report` plus Search Console site, search analytics, and sitemap commands. (#402) — thanks @haresh-seenivasagan.
- Gmail: add `gmail send --body-html-file` for sending HTML email bodies from files without shell command substitution. (#575) — thanks @toruvieI.
- YouTube: add `youtube` (alias `yt`) command group for YouTube Data API v3 — list activities, videos, playlists, comment threads, and channels; API key via config `youtube_api_key` or `GOG_YOUTUBE_API_KEY`; OAuth for `--mine` with `gog auth add ... --services youtube`. (#313) — thanks @satputekuldip.
- Forms: add quiz grading flags to `forms add-question` so choice and short-answer questions can set answer keys and point values when created. (#570) — thanks @dbernaltbn.
- Calendar: resolve event locations through Places API with `--location-search` / `--place-id`, storing the resolved Place ID in private extended properties. (#140 / #138) — thanks @salmonumbrella.
- Drive: add `drive changes` start-token/list/watch/stop commands for incremental sync and webhook automation. (#335)
- Drive: add `drive activity query` for Drive Activity API v2 audit trails with item, folder, time, and action filters. (#337)
- CLI: add `--wrap-untrusted` / `GOG_WRAP_UNTRUSTED` to mark fetched JSON/raw
  free-text fields with external untrusted-content wrappers for agent/LLM use. (#577) — thanks @VACInc.
- Meet: add `meet create/get/update/end/history/participants` commands for Google Meet meeting spaces and conference records. (#468) — thanks @regaw-leinad.
- Forms: add `forms publish` to publish/unpublish existing forms and return the responder URL for automated form creation flows. (#565 / #564) — thanks @bogdanovich.

### Fixed

- Auth: make `auth service-account status` show `stored`, a clear missing-key message, and the exact setup hint when no service-account key is configured.
- Admin: retry the post-create state update so `admin users create --suspended` and `--archived` remain reliable while the Admin SDK finishes provisioning the new user.
- Calendar: make `calendar update --with-meet` idempotent when an event already has conference data, add explicit `--regenerate-meet`, and show `recurringEventId` in plain event output. (#576 / #573) — thanks @alexisperumal and @NodeJSmith.
- Release: fail closed when macOS signing secrets are missing and verify Darwin release assets are not ad-hoc signed, so Homebrew upgrades keep stable Keychain trust. (#569) — thanks @aaroneden.
- Auth: list one row per OAuth client when the same account is authorized under multiple clients, and let `auth list --client` filter that token bucket. (#563) — thanks @UnPractical91.
- Docs: clarify how to pass file-keyring environment into headless OpenClaw/systemd agent processes. (#566) — thanks @chsbusch-dot.
- Docs: avoid infinite loops when local Markdown parsing ends on Thai, CJK, emoji, or other multi-byte runes. (#560 / #559) — thanks @ninyawee.
- Agent skill: replace stale bundled `gog` skill paths with the current docs and auth package locations. (#558 / #557) — thanks @WadydX.
- CI: run the docs validation gate in GitHub Actions and the local `make ci` target. (#562 / #561) — thanks @WadydX.

## 0.15.0 - 2026-05-05

### Added
- Export exact Google API JSON when the normal CLI view is too lossy: `docs raw`, `sheets raw`, `slides raw`, `drive raw`, `gmail raw`, `calendar raw`, `people raw`, `contacts raw`, `tasks raw`, and `forms raw`, with `--pretty`, safer Drive defaults, Sheets grid-data warnings, and a raw-output security audit. (#495, #496) — thanks @karbassi.
- Audit Drive storage without changing files: `drive tree`, `drive du`, and `drive inventory` now report folder contents, sizes, and inventory data for cleanup/review workflows. (#116) — thanks @rohan-patnaik.
- Find duplicate contacts safely: `contacts dedupe` is preview-only, matches by email/phone by default, supports opt-in name matching, and emits JSON/table merge plans without applying changes. (#116) — thanks @rohan-patnaik.
- Read Gmail messages in agent-safe form: `gmail get --sanitize-content` / `--safe` and `gmail thread get --sanitize-content` return sanitized content without exposing raw Gmail payloads in JSON. (#238, #220) — thanks @urasmutlu.
- Ship official container images: release tags now publish a non-root GHCR Docker image, with file-keyring docs for container automation. (#539, #444) — thanks @HuckOps and @rdehuyss.
- Request custom Drive fields: `drive ls --fields` and `drive get --fields` pass Drive API field masks for data beyond the default JSON set. (#495) — thanks @karbassi.
- Format Google Docs from the CLI: `docs format` and plain-text `docs write` formatting flags cover fonts, colors, bold/italic/underline/strikethrough, alignment, and line spacing. (#479) — thanks @mmaghsoodnia.
- Manage Google Docs tabs: `docs add-tab`, `docs rename-tab`, `docs delete-tab`, plus tab-scoped Markdown append and find-replace flows. (#547, #541) — thanks @chopenhauer and @donbowman.
- Work with structured Google Sheets tables: `sheets table` list/get/create/delete, `sheets table append`, and header-safe `sheets table clear`. (#470) — thanks @Pedrohgv.
- Format Sheets visually: `sheets conditional-format` and `sheets banding` add rule-based formatting and alternating color banded ranges. (#378) — thanks @codBang.
- Add Meet links to existing calendar events with `calendar update --with-meet`. (#538) — thanks @alexisperumal.
- Move calendar events between calendars with `calendar move` / `calendar transfer`, including organizer changes. (#448) — thanks @markusbkoch.
- Export Gmail filters as Gmail WebUI-importable Atom XML, while keeping API JSON export via `--format json`. (#174) — thanks @gwpl.
- Build safer agent binaries with baked `agent-safe`, `readonly`, and `full` safety profiles, fail-closed command filtering, filtered help/schema output, docs, and build tooling. (#366, #239) — thanks @drewburchfield.
- Use gog from coding agents more safely with the bundled `gog` skill for JSON-first Google Workspace automation. (#353, #451) — thanks @TimPietrusky and @sluramod.

### Fixed
- Make full-mailbox backups survive large Gmail exports by promoting completed checkpoint shards into the final manifest and byte-splitting fallback message shards before GitHub rejects oversized blobs.
- Make backup exports more resumable and fault-tolerant by streaming decrypted shards, preserving Gmail Markdown mirrors, handling very large JSONL rows, and writing Markdown fallbacks for malformed MIME messages instead of aborting.
- Keep agent safety profiles harder to patch by compiling baked policies into generated hash switches instead of embedding raw allow/deny YAML strings. (#540) — thanks @drewburchfield.
- Show correct versions for `go install ...@tag` binaries by inferring module versions from Go build info when linker metadata is absent. (#545, #544) — thanks @joshavant.
- Accept the documented `calendar events list` / `ls` selector forms, including positional calendar IDs, `--cal`, `--calendars`, and `--all`. (#546) — thanks @BCudeOpenClaw.
- Keep `docs find-replace --dry-run` read-only while still reporting match counts, and allow empty replacement strings to delete matches safely. (#542) — thanks @chrismdp.

## 0.14.0 - 2026-04-28

### Added
- Backup: add `gog backup` with age-encrypted Git shards, Gmail labels/raw message export, Calendar/Contacts/Tasks/Drive metadata adapters, manifest status, full decrypt-and-verify, shard `cat`, local plaintext export, docs, and security-focused regression coverage.
- Backup: expand `gog backup push --services all` with Drive content export/download, Gmail settings, native Workspace Docs/Sheets/Slides/Form data, Apps Script projects, Chat, Classroom, best-effort optional service error shards, and plaintext Drive file export.
- Backup: extend `--services all` with Drive permissions/comments/revisions, Calendar ACL/settings/colors, contact groups, Cloud Identity groups, Workspace Admin Directory users/groups/members, Keep notes, and local Gmail message caching for resumable full-mailbox fetches.
- Backup: add `gog backup export --gmail-format markdown` for local readable Gmail mirrors with Markdown notes and extracted attachment files.
- Gmail: add `gmail messages search --body-format html` for returning HTML message bodies when `--include-body` is used. (#520) — thanks @alexknowshtml.
- Contacts: add `contacts export` for vCard 4.0 `.vcf` exports by resource, email/name search, or all contacts, including best-effort label categories. (#519, #500) — thanks @dinakars777.
- Docs: add experimental `docs export --tab` / `drive download --tab` to export a single Google Docs tab as PDF, DOCX, text, Markdown, or HTML. (#535) — thanks @johnbenjaminlewis.
- Slides: add `slides insert-text` and `slides replace-text` for editing existing slide text elements and replacing template tokens. (#521) — thanks @chrissanchez-iops.
- Drive: add `drive search --drive` and `--parent` for scoping search to a shared drive or folder. (#525) — thanks @LeanSheng.
- Calendar: add `--start-timezone` / `--end-timezone` to `calendar create` and `calendar update` for preserving named IANA event timezones when RFC3339 inputs only carry numeric offsets. (#422)
- Contacts: include birthdays in `contacts list` and `contacts search` text and JSON output. (#441)
- Auth: add `gog auth doctor` to diagnose keyring backend/password drift, unreadable file-keyring tokens, and refresh-token failures such as Workspace `invalid_rapt`. (#377, #338)
- Backup: bound individual Drive content exports with `--drive-content-timeout` so one stuck Google export records an encrypted error row instead of blocking the full backup.
- Backup: add Gmail message-list checkpoints, streaming shard construction, and stderr progress counters so full-mailbox backups can resume cleanly after interruption without keeping every raw message in RAM.
- Backup: push encrypted incomplete Gmail checkpoint commits during long cached fetches so day-scale mailbox backups have offsite progress before the final manifest is committed.
- Backup: push Gmail checkpoint commits through a single ordered background queue so cached fetches continue while GitHub uploads run.
- Slides docs: document the Markdown structure accepted by `slides create-from-markdown`. (#497)
- Google API: expose a reusable authenticated HTTP client for commands that need custom HTTP policies. (#534) — thanks @johnbenjaminlewis.

### Fixed
- Auth: keep `gog auth list` and `gog auth tokens list` useful when one file-keyring token cannot be decrypted; unreadable entries are now reported instead of aborting the whole listing. (#377)
- Auth: time out Linux D-Bus keyring write operations and report when OAuth completed but saving the refresh token failed, so manual auth no longer looks like a stuck paste when token persistence is blocked. (#130)
- Auth: store Google OIDC `sub` claims with OAuth tokens and migrate matching subject-keyed accounts when a Google email rename is reauthorized. (#504)
- Gmail: build outbound `Date` headers with the configured timezone so replies do not inherit a wrong host-local offset. (#514, #472) — thanks @dinakars777.
- Gmail: auto-fill draft reply subjects from the original message when `gmail drafts create --reply-to-message-id` omits `--subject`. (#488) — thanks @jbowerbir.
- Gmail: fall back to the People profile name for primary-account `From` headers when Gmail send-as settings omit a display name. (#431) — thanks @moeedahmed.
- Gmail: expose reply threading headers in default `gmail get --format metadata` output and fail explicit reply targets that cannot provide a `Message-ID`. (#528, #512) — thanks @solomonneas.
- Gmail: apply Gmail system-label filters for searches like `in:spam is:unread` so thread, message, and batch message searches do not return read spam. (#449)
- Gmail: preserve renewed watch expiration fields when a long-running `gmail watch serve` process records push delivery state after `gmail watch renew` runs separately. (#526)
- Gmail: reuse the shared paginated list runner for thread and message search so `--all`, `--page`, text, and JSON output stay consistent.
- Gmail: clarify that `gmail batch delete` is permanent and point default-scope workflows at `gmail trash`. (#151)
- Drive/Docs/Sheets/Slides: treat `--out -` as stdout for downloads and exports instead of creating `-`/`-.ext` files; reject `--json --out -` to keep byte streams parseable. (#286)
- Docs: deprecate editing-command `--tab-id` in favor of `--tab`, and resolve tab titles to canonical tab IDs before mutations. (#533) — thanks @johnbenjaminlewis.
- Docs: convert Markdown formatting for `docs write --append --markdown` instead of appending raw Markdown syntax. (#530, #272) — thanks @eric-x-liu.
- Docs: include available tab names when `docs cat --tab` / structure lookup cannot find the requested tab. (#532) — thanks @johnbenjaminlewis.
- Docs: size remote Markdown images consistently for `docs write --replace --markdown` by reusing the Docs image insertion path after Drive conversion, and return a clear error for local image paths that the Docs API cannot fetch directly. (#518) — thanks @vinothd-oai.
- Drive: print large upload progress to stderr while keeping JSON output parseable. (#529)
- Drive: include `hasThumbnail` and `thumbnailLink` in `drive ls`, `drive search`, and `drive get` JSON responses. (#486) — thanks @gtapps.
- Drive: include `driveId` in `drive ls`, `drive search`, and `drive get` field masks so Shared Drive files can be identified in JSON output. (#524) — thanks @LeanSheng.
- Calendar: display `calendar events` times and JSON local fields in the calendar timezone instead of preserving arbitrary event offsets. (#493)
- Email tracking: add versioned tracking-key rotation so new pixels use the current key while old tracking ids keep decrypting through prior keys. (#293)
- Email tracking: deduplicate repeated pixel opens and cap recorded opens per IP per hour to reduce D1 abuse from replay or high-volume requests. (#294)
- Email tracking: add daily Worker retention cleanup for open rows older than 90 days and cap admin `/opens` responses at 500 rows. (#292)
- Email tracking: make `gmail track setup --deploy` reusable with existing D1 databases and valid temporary Wrangler configs.
- Backup: split Gmail checkpoint commits by row count and plaintext byte size so large messages stay below GitHub's blob limit.
- Secrets: time out macOS Keychain read/write/list operations with a clear recovery hint instead of hanging indefinitely when a permission prompt cannot surface. (#515, #513) — thanks @sardoru.
- Secrets: encode file-backend key names so stored tokens work on Windows, while still reading/removing legacy raw entries. (#527, #502) — thanks @solomonneas.
- CLI: show direct Google Cloud API enablement links and matching `auth add --services ...` hints when Google returns API-not-enabled errors.
- Install docs: document Windows release ZIP/PATH setup and clarify that source builds require the Go version declared in `go.mod`, not Ubuntu 24.04's Go 1.22 package. (#157, #135)
- Auth docs: clarify that consumer Gmail refresh tokens expire after 7 days when the OAuth app remains External + Testing, and that publishing the personal OAuth app is the long-lived-token path. (#121)
- CI: pin GitHub Actions workflow dependencies to immutable commit SHAs. (#288)

## 0.13.0 - 2026-04-20

### Highlights
- Gmail: safer sending and richer message workflows, with no-send guardrails, forwarding, autoreplies, full-body search output, label styling, and better MIME/body handling. (#454, #482, #447, #457, #476, #477, #511) — thanks @veteranbv, @spencer-c-reed, @GodsBoy, @iskw9973, @shashankkr9, @yeager, and @dinakars777.
- Drive/Docs/Slides: smoother content round-trips with Markdown-to-Docs upload conversion, restored Markdown replace writes, rendered slide thumbnails, commenter sharing, and better Docs sed formatting. (#487, #501, #498, #443, #483) — thanks @johnbenjaminlewis, @twilsher, @gianpaj, @pavelzak, and @bill492.
- Sheets: chart management lands, including list/inspect/create/update/delete and a chart-range fix for sheet ID 0. (#434) — thanks @andybergon.
- Calendar: create secondary calendars and get more predictable timezone/day-bound behavior. (#455, #492, #509, #510) — thanks @alexknowshtml, @RaphaelRUzan, and @dinakars777.
- Auth and agent safety: credential cleanup, Google Ads auth, keyring namespace overrides, command denylists, and safer send-operation controls. (#473, #264, #463, #218, #173, #454) — thanks @yamagucci, @ufkhan97, @mkurz, @EricYangTL, @spookyuser, and @veteranbv.

### Added
- Gmail: add `--gmail-no-send`, `GOG_GMAIL_NO_SEND`, `gmail_no_send`, and per-account `config no-send` guards for blocking send operations. (#454) — thanks @veteranbv.
- Gmail: add `gmail forward` / `gmail fwd` to forward a message with optional note, verified send-as alias, and original attachments. (#482) — thanks @spencer-c-reed.
- Gmail: add `gmail autoreply` to reply once to matching messages, label the thread for dedupe, and optionally archive/mark read.
- Gmail: add `gmail messages search --full` to print complete message bodies instead of truncating text output. (#447) — thanks @GodsBoy.
- Gmail: add `gmail labels style` to update user label colors and list/message visibility. (#457) — thanks @iskw9973.
- Drive: convert Markdown uploads to Google Docs and strip leading YAML frontmatter by default, with `--keep-frontmatter` to opt out. (#487) — thanks @johnbenjaminlewis.
- Drive: allow `drive share --role commenter` for comment-only sharing. (#443) — thanks @pavelzak.
- Drive: show owner email in `drive ls` and `drive search` table output. (#458) — thanks @laihenyi.
- Slides: add `slides thumbnail` / `slides thumb` to fetch rendered slide thumbnail URLs or download PNG/JPEG images. (#498) — thanks @gianpaj.
- Sheets: add `sheets chart` to list, inspect, create, update, and delete embedded charts. (#434) — thanks @andybergon.
- Sheets: add `add-sheet`, `rename-sheet`, and `delete-sheet` tab aliases plus `sheets add-tab --index`. (#442) — thanks @alexknowshtml.
- Calendar: add `calendar create-calendar` / `new-calendar` to create secondary calendars with description, timezone, and location. (#455) — thanks @alexknowshtml.
- Auth: add `auth credentials remove` to delete stored OAuth client credentials and associated refresh tokens. (#473) — thanks @yamagucci.
- Auth: add `ads` as an auth service for Google Ads API tokens. (#264) — thanks @ufkhan97.
- Secrets: allow `GOG_KEYRING_SERVICE_NAME` to override the keyring namespace. (#463) — thanks @mkurz.
- Agent safety: allow dotted command paths in `--enable-commands` and add `--disable-commands` / `GOG_DISABLE_COMMANDS` denylist support. (#218, #173) — thanks @EricYangTL and @spookyuser.
- Contacts: add `--gender` to `contacts create` and `contacts update`, and include gender in `contacts get` text output. (#438) — thanks @klodr.
- Chat: make `chat spaces find` use case-insensitive substring matching by default, with `--exact` for legacy exact lookup. (#506) — thanks @mvanhorn.

### Fixed
- Calendar: avoid ambiguous timezone guessing from offset-only event times, preserve timezones for focus-time events, and use exclusive next-midnight bounds for full-day ranges. (#492, #509, #510) — thanks @RaphaelRUzan and @dinakars777.
- Gmail: preserve sent and received body content by using quoted-printable plain text, non-`7bit` non-ASCII HTML, and safer UTF-8 charset handling. (#476, #477, #511) — thanks @shashankkr9, @yeager, and @dinakars777.
- Docs: restore `docs write --replace --markdown` conversion and preserve sed formatting ranges, UTF-16 offsets, and `&` whole-match replacements. (#501, #483) — thanks @twilsher and @bill492.
- Sheets: preserve valid chart ranges that target sheet ID 0 while still remapping sample-style zero IDs when the spreadsheet has no zero-ID sheet. (#434) — thanks @andybergon.
- Auth: remove stale aliases and account-client mappings from config when `auth remove` deletes an account. (#467) — thanks @mvanhorn.
- Contacts: reject all individual update flags when `contacts update --from-file` is used. (#439) — thanks @klodr.
- Tasks: clear task due dates when `tasks update --due=` is provided. (#507) — thanks @dinakars777.
- CLI: generate native zsh completions without relying on `bashcompinit`. (#481) — thanks @piiq.
- Windows: expand `~\...` paths and run the integration live-test wrapper through PowerShell. (#452) — thanks @gagradebnath.
- Tracking: prefer file-stored tracking secrets over stale keyring values unless keyring storage is configured. (#469) — thanks @alexuser.
- Time parsing: accept `tues`, `thur`, and `thurs` as weekday expressions. (#440) — thanks @sjhddh.

## 0.12.0 - 2026-03-09

### Highlights
- Admin: full Workspace Admin users/groups coverage for common directory operations. (#403) — thanks @dl-alexandre.
- Auth: new headless/cloud auth paths with ADC, direct access tokens, custom callbacks, proxy-safe loopback settings, and extra-scope controls. (#357, #419, #227, #398, #421) — thanks @tengis617, @mmkal, @cyberfox, @salmonumbrella, and @peteradams2026.
- Docs: much stronger document editing and export flow with tab targeting, richer find-replace, pageless mode, and native Markdown/HTML export. (#330, #305, #300, #282, #141) — thanks @ignacioreyna, @chparsons, @shohei-majima, @fprochazka, and @in-liberty420.
- Sheets: spreadsheet editing/formatting expands significantly with named ranges, tab management, notes, find-replace, formatting controls, inserts, links, and format inspection. (#278, #309, #430, #341, #320, #203, #374, #284) — thanks @TheCrazyLex, @JulienMalige, @andybergon, @Shehryar, @omothm, and @nilzzzzzz.
- Calendar: aliases, subscribe, and selector parity make multi-calendar workflows much easier. (#393, #327, #319) — thanks @salmonumbrella and @cdthompson.
- Forms/Slides/Keep: forms management + watches, slides from templates, and first write/delete coverage for Keep. (#274, #273, #413) — thanks @alexknowshtml, @penguinco, and @jgwesterlund.

### Added
- Admin: add Workspace Admin Directory commands for users and groups, including user list/get/create/suspend and group membership list/add/remove. (#403) — thanks @dl-alexandre.
- Auth: add Application Default Credentials mode via `GOG_AUTH_MODE=adc` for Workload Identity, Cloud Run, and local `gcloud` ADC flows without stored OAuth refresh tokens. (#357) — thanks @tengis617.
- Auth: add `--access-token` / `GOG_ACCESS_TOKEN` for direct access-token auth in headless or CI flows, bypassing stored refresh tokens. (#419) — thanks @mmkal.
- Auth: add `auth add|manage --listen-addr` plus `--redirect-host` for browser OAuth behind proxies or remote loopback forwarding. (#227) — thanks @cyberfox.
- Auth: add `auth add --redirect-uri` for manual/remote OAuth flows, so custom callback hosts can be reused across the printed auth URL, state cache, and code exchange. (#398) — thanks @salmonumbrella.
- Auth: add `--extra-scopes` to `auth add` for appending custom OAuth scope URIs beyond the built-in service scopes. (#421) — thanks @peteradams2026.
- Docs: add `--tab-id` to editing commands so write/update/insert/delete/find-replace can target a specific Google Docs tab. (#330) — thanks @ignacioreyna.
- Docs: extend `docs find-replace` with `--first`, `--content-file`, Markdown replacement, inline image insertion, and image sizing syntax. (#305) — thanks @chparsons.
- Docs: add `--pageless` to `docs create`, `docs write`, and `docs update` to switch documents into pageless mode after writes. (#300) — thanks @shohei-majima.
- Docs: add native Google Docs Markdown export via `docs export --format md`. (#282) — thanks @fprochazka.
- Docs: add native Google Docs HTML export via `docs export --format html`. (#141) — thanks @in-liberty420.
- Sheets: add named range management (`sheets named-ranges`) and let range-based Sheets commands accept named range names where GridRange-backed operations are needed. (#278) — thanks @TheCrazyLex.
- Sheets: add `add-tab`, `rename-tab`, and `delete-tab` commands for managing spreadsheet tabs, with delete dry-run/confirmation guardrails. (#309) — thanks @JulienMalige.
- Sheets: add `merge`, `unmerge`, `number-format`, `freeze`, `resize-columns`, and `resize-rows` commands for spreadsheet layout/format control. (#320) — thanks @Shehryar.
- Sheets: add `sheets update-note` / `set-note` to write or clear cell notes across a range. (#430) — thanks @andybergon.
- Sheets: add `sheets find-replace` to replace text across a spreadsheet or a specific tab, with exact-match, regex, and formula search options. (#341) — thanks @Shehryar.
- Sheets: add `sheets insert` to insert rows/columns into a sheet. (#203) — thanks @andybergon.
- Sheets: add `sheets create --parent` to place new spreadsheets in a Drive folder. (#424) — thanks @ManManavadaria.
- Sheets: add `sheets read-format` to inspect `userEnteredFormat` / `effectiveFormat` per cell. (#284) — thanks @nilzzzzzz.
- Sheets: add `sheets links` (alias `hyperlinks`) to list cell links from ranges, including rich-text links. (#374) — thanks @omothm.
- Forms: add form update/question-management commands plus response watch create/list/delete/renew, with delete-question validation and confirmation guardrails. (#274) — thanks @alexknowshtml.
- Slides: add `create-from-template` with `--replace` / `--replacements`, dry-run support, and template placeholder replacement stats. (#273) — thanks @penguinco.
- Calendar: add `calendar alias list|set|unset`, and let calendar commands resolve configured aliases before API/name lookup. (#393) — thanks @salmonumbrella.
- Calendar: let `calendar freebusy` / `calendar conflicts` accept `--cal`, names, indices, and `--all` like `calendar events`. (#319) — thanks @salmonumbrella.
- Calendar: add `calendar subscribe` (aliases `sub`, `add-calendar`) to add a shared calendar to the current account’s calendar list. (#327) — thanks @cdthompson.
- Gmail: add `gmail send --signature`, `--signature-from`, and `--signature-file` to append Gmail send-as or local signatures before sending. (#180, #183) — thanks @kesslerio and @salmonumbrella.
- Gmail: add `watch serve --history-types` filtering (`messageAdded|messageDeleted|labelAdded|labelRemoved`) and include `deletedMessageIds` in webhook payloads. (#168) — thanks @salmonumbrella.
- Gmail: add `gmail labels rename` to rename user labels by ID or exact name, with system-label guards and wrong-case ID safety. (#391) — thanks @adam-zethraeus.
- Gmail: add `gmail messages modify` for single-message label changes, complementing thread- and batch-level modify flows. (#281) — thanks @zerone0x.
- Gmail: add `gmail filters export` to dump filter definitions as JSON to stdout or a file for backup/script workflows. (#119) — thanks @Jeswang.
- Keep: add `keep create` for text/checklist notes and `keep delete` for note removal. (#413) — thanks @jgwesterlund.
- Contacts: support `--org`, `--title`, `--url`, `--note`, and `--custom` on create/update; include custom fields in get output with deterministic ordering. (#199) — thanks @phuctm97.
- Contacts: add `--relation type=person` to contact create/update, include relations in text `contacts get`, and cover relation payload updates. (#351) — thanks @karbassi.
- Contacts: add `--address` to contact create/update and include addresses in text `contacts get`. (#148) — thanks @beezly.
- Drive: add `drive ls --all` (alias `--global`) to list across all accessible files; make `--all` and `--parent` mutually exclusive. (#107) — thanks @struong.
- Chat: add `chat messages reactions create|list|delete` to manage emoji reactions on messages; `chat messages react <message> <emoji>` as a shorthand for creating reactions; `reaction` is an alias for `reactions`. (#426) — thanks @fernandopps.
- Tasks: add `--recur` / `--recur-rrule` aliases for repeat materialization, including RRULE `INTERVAL` support for generated occurrences. (#408) — thanks @salmonumbrella.

### Fixed
- Google API: use transport-level response-header timeouts for API clients while keeping token exchanges bounded, so large downloads are not cut short by `http.Client.Timeout`. (#425) — thanks @laihenyi.
- Timezone: embed the IANA timezone database so Windows builds can resolve calendar timezones correctly. (#388) — thanks @visionik.
- Auth: persist rotated OAuth refresh tokens returned during API calls so later commands keep working without re-auth. (#373) — thanks @joshp123.
- Auth: allow pure service-account mode when the configured subject matches the service account itself, instead of forcing domain-wide delegation impersonation. (#399) — thanks @carrotRakko.
- Auth: keep Keep-only service-account fallback isolated to Keep commands so other Google services do not accidentally pick it up. (#414) — thanks @jgwesterlund.
- Auth: add `--gmail-scope full|readonly`, and disable `include_granted_scopes` for readonly/limited auth requests to avoid Drive/Gmail scope accumulation. (#113) — thanks @salmonumbrella.
- Auth: preserve scope-shaping flags in the remote step-2 replay guidance for `auth add --remote`. (#427) — thanks @doodaaatimmy-creator.
- Calendar: preserve full RRULE values and recurring-event timezones during updates so recurrence edits don’t lose BYDAY lists or hit missing-timezone API errors. (#392) — thanks @salmonumbrella.
- Calendar: let recurring `calendar update --scope=future` and `calendar delete --scope=future` start from an instance event ID by resolving the parent series first. (#319) — thanks @salmonumbrella.
- Calendar: use `Calendars.Get` for timezone lookups so service-account flows don’t 404 on `calendarList/primary`. (#325) — thanks @markwatson.
- Calendar: hide cancelled/deleted events from `calendar events` list output by explicitly setting `showDeleted=false`. (#362) — thanks @sharukh010.
- Calendar: reject ambiguous calendar-name selectors for `calendar events` instead of guessing. (#131) — thanks @salmonumbrella.
- Calendar: respond patches only attendees to avoid custom reminders validation errors. (#265) — thanks @sebasrodriguez.
- Calendar: force-send `minutes=0` for `--reminder popup:0m` so zero-minute popup reminders survive Google Calendar API JSON omission rules. (#316) — thanks @salmonumbrella.
- Calendar: clarify that RFC3339 `--from/--to` timestamps must include a timezone while keeping date and relative-time help intact. (#409) — thanks @dbhurley.
- Gmail: add a fetch delay in `watch serve` so History API reads don't race message indexing. (#397) — thanks @salmonumbrella.
- Gmail: preserve the selected `--client` during `watch serve` push handling instead of falling back to the default client. (#411) — thanks @chrysb.
- Gmail: allow Workspace-managed send-as aliases with empty verification status in `send` and `drafts create`. (#407) — thanks @salmonumbrella.
- Gmail: fall back to `MimeType` charset hints when `Content-Type` headers are missing so GBK/GB2312 message bodies decode correctly. (#428) — thanks @WinnCook.
- Gmail: `drafts update --quote` now picks a non-draft, non-self message from thread fallback (or errors clearly), avoiding self-quote loops and wrong reply headers. (#394) — thanks @salmonumbrella.
- Gmail: preserve `Cc` metadata output in plain `gmail get --format metadata` even when Gmail returns uppercase `CC` headers. (#343) — thanks @salmonumbrella.
- Gmail: `gmail archive|read|unread|trash` convenience commands now honor `--dry-run` and emit action-specific dry-run ops. (#385) — thanks @yeager.
- Gmail: retry transient `failedPrecondition` errors during `gmail filters create` and return the existing matching filter on duplicate creates, so reruns stay idempotent.
- Sheets: harden `sheets format` against `boarders` typo (JSON and field mask), with clearer error messages. (#284) — thanks @nilzzzzzz.
- Sheets: force-send empty note values so `sheets update-note --note ''` reliably clears notes via the API. (#341) — thanks @Shehryar.
- Contacts: send the required `copyMask` when deleting "other contacts", avoiding People API 400 errors. (#384) — thanks @rbansal42.
- Groups: include required label filters in transitive group searches so `groups list` doesn’t 400 on Cloud Identity. (#315) — thanks @salmonumbrella.
- Sheets: make `sheets metadata --plain` emit real TSV tab delimiters, with regression coverage for plain tabular sheet output. (#298) — thanks @mahsumaktas.
- CLI: show root help instead of a parse error when `gog` is run with no arguments. (#342) — thanks @cstenglein.
- CLI: include the current partial token in fish shell completion so `gog __complete` sees the active word under the cursor. (#123) — thanks @GiGurra.

### Security & Reliability
- Secrets: verify keyring token writes by reading them back, so macOS headless Keychain failures return an actionable error instead of silently storing 0 bytes. (#270) — thanks @zerone0x.
- Secrets: respect empty `GOG_KEYRING_PASSWORD` (treat set-to-empty as intentional; avoids headless prompts). (#269) — thanks @zerone0x.
- Security: require confirmation before public Drive shares, Gmail forwarding filters, and Gmail delegate grants in no-input/agent flows. (#317) — thanks @salmonumbrella.
- Security: redact stored Gmail watch webhook bearer tokens in `gmail watch status` text and JSON output unless `--show-secrets` is set. (#136) — thanks @paveg.

### Tooling & Docs
- Docs: update install docs to use the official Homebrew core formula (`brew install gogcli`). (#361) — thanks @zeldrisho.
- Contacts: fix grouped parameter types in CRUD helpers to restore builds on newer Go toolchains. (#355) — thanks @laihenyi.
- CI: validate release tags and quote the checkout ref in the release workflow to block tag-script injection on manual releases. (#299) — thanks @salmonumbrella.
- Build: refresh the dependency stack to Go 1.26.1, current Go indirects, GitHub Actions v6/v7 pins, and current Cloudflare worker dependencies.
- Keep: request the writable Keep service-account scope now that note create/delete is supported. (#413) — thanks @jgwesterlund.

## 0.11.0 - 2026-02-15

### Added
- Apps Script: add `appscript` command group (create/get projects, fetch content, run deployed functions).
- Forms: add `forms` command group (create/get forms, list/get responses).
- Docs: add `docs comments` for listing and managing Google Doc comments. (#263) — thanks @alextnetto.
- Sheets: add `sheets notes` to read cell notes. (#208) — thanks @andybergon.
- Gmail: add `gmail send --quote` to include quoted original message in replies. (#169) — thanks @terry-li-hm.
- Drive: add `drive ls|search --no-all-drives` to restrict queries to "My Drive" for faster/narrower results. (#258)
- Contacts: update contacts from JSON via `contacts update --from-file` (PR #200 — thanks @jrossi).

### Fixed
- Drive: make `drive delete` move files to trash by default; add `--permanent` for irreversible deletion. (#262) — thanks @laihenyi.
- Drive/Gmail: pass through Drive API filter queries in `drive search`; RFC 2047-encode non-ASCII display names in mail headers (`From`/`To`/`Cc`/`Bcc`/`Reply-To`). (#260) — thanks @salmonumbrella.
- Calendar: allow opting into attendee notifications for updates and cancellations via `calendar update|delete --send-updates all|externalOnly|none`. (#163) — thanks @tonimelisma.
- Calendar: fall back to fixed-offset timezones (`Etc/GMT±N`) for recurring events when given RFC3339 offset datetimes; harden Gmail attachment output paths and cache validation; honor proxy defaults for Google API transports. (#228) — thanks @salmonumbrella.
- Auth: manual OAuth flow uses an ephemeral loopback redirect port (avoids unsafe/privileged ports in browsers). (#172) — thanks @spookyuser.
- Gmail: include primary display name in `gmail send` From header when using service account impersonation (domain-wide delegation). (#184) — thanks @salmonumbrella.
- Gmail: when `gmail attachment --out` points to a directory (or ends with a trailing slash), combine with `--name` and avoid false cache hits on directories. (#248) — thanks @zerone0x.
- Drive: include shared drives in `drive ls` and `drive search`; reject `drive download --format` for non-Google Workspace files. (#256) — thanks @salmonumbrella.
- Drive: validate `drive download --format` values and error early for unknown formats. (#259)

## 0.10.0 - 2026-02-14

### Added
- Docs/Slides: add `docs update` markdown formatting + table insertion, plus markdown-driven slides creation and template-based slide creation. (#219) — thanks @maxceem.
- Slides: add add-slide/list-slides/delete-slide/read-slide/update-notes/replace-slide for image decks, including --before insertion and --notes '' clear behavior. (#214) — thanks @chrismdp.
- Docs: add tab support (`docs list-tabs`, `docs cat --tab`, `docs cat --all-tabs`) and editing commands (`docs write|insert|delete|find-replace`). (#225) — thanks @alexknowshtml.
- Docs: add `docs create --file` to import Markdown into Google Docs with inline image support and hardened temp-file cleanup. (#244) — thanks @maxceem.
- Drive: add `drive upload --replace` to update files in-place (preserves `fileId`/shared link). (#232) — thanks @salmonumbrella.
- Drive: add upload conversion flags `--convert` (auto) and `--convert-to` (`doc|sheet|slides`). (#240) — thanks @Danielkweber.
- Drive: share files with an entire Workspace domain via `drive share --to domain`. (#192) — thanks @Danielkweber.
- Gmail: add `--exclude-labels` to `watch serve` (defaults: `SPAM,TRASH`). (#194) — thanks @salmonumbrella.
- Gmail: add `gmail labels delete <labelIdOrName>` with confirm + system-label guardrails and case-sensitive ID handling. (#231) — thanks @Helmi.
- Contacts: support `contacts update --birthday` and `--notes`; unify shared date parsing and docs. (#233) — thanks @rosssivertsen.

### Fixed
- Live tests: make `scripts/live-test.sh` and `scripts/live-chat-test.sh` CWD-safe (repo-root aware builds and sourcing).
- Calendar: interpret date-only and relative day `--to` values as inclusive end-of-day while keeping `--to now` as a point-in-time bound. (#204) — thanks @mjaskolski.
- Auth: improve remote/server-friendly manual OAuth flow (`auth add --remote`). (#187) — thanks @salmonumbrella.
- Gmail: avoid false quoted-printable detection for already-decoded URLs with uppercase hex-like tokens while still decoding unambiguous markers (`=3D`, chained escapes, soft breaks). (#186) — thanks @100menotu001.
- Sheets: preserve TSV tab delimiters for `sheets get --plain` output. (#212) — thanks @salmonumbrella.
- CLI: land PR #201 with conflict-resolution fixes for `--fields` rewrite, calendar `--all` paging, schema command-path parsing, and case-sensitive Gmail watch exclude-label IDs. (#201) — thanks @salmonumbrella.
- Secrets: set keyring item labels to `gogcli` so macOS security prompts show a clear item name. (#106) — thanks @maxceem.

## 0.9.0 - 2026-01-22

### Highlights

- Auth: multi-org login with per-client OAuth credentials + token isolation. (#96)

### Added

- Calendar: show event timezone and local times; add --weekday output. (#92) — thanks @salmonumbrella.
- Gmail: show thread message count in search output. (#99) — thanks @jeanregisser.
- Gmail: message-level search with optional body decoding. (#88) — thanks @mbelinky.

### Fixed

- Auth: fix Gmail search example in auth success template. (#89) — thanks @rvben.
- CLI: remove redundant newlines in text output for calendar, chat, Gmail, and groups commands. (#91) — thanks @salmonumbrella.
- Gmail: include primary account display name in send From header when available. (#93) — thanks @salmonumbrella.
- Keyring: persist OAuth tokens across Homebrew upgrades. (#94) — thanks @salmonumbrella.
- Docs: update Gmail command examples in README. (#95) — thanks @chrisrodz.
- Contacts: include birthdays in contact get output. (#102) — thanks @salmonumbrella.
- Calendar: force custom reminders payload to send UseDefault=false. (#100) — thanks @salmonumbrella.
- Gmail: add read alias + default thread get. (#103) — thanks @salmonumbrella.

## 0.8.0 - 2026-01-19

### Added

- Chat: spaces, messages, threads, and DM commands (Workspace only). (#84) — thanks @salmonumbrella.
- People: profile lookup, directory search, and relations commands. (#84) — thanks @salmonumbrella.

### Fixed

- Chat: normalize thread IDs and show a clearer error for consumer accounts. (#84)

## 0.7.0 - 2026-01-17

### Highlights

- Classroom: full command suite (courses, roster, coursework/materials, announcements, topics, invitations, guardians, profiles) plus course URLs. (#73) — thanks @salmonumbrella.
- Calendar: propose-time command and enterprise event types (Focus Time/Out of Office/Working Location). (#75) — thanks @salmonumbrella.
- Gmail: attachment details in `gmail get` (humanized sizes + JSON fields). (#83) — thanks @jeanregisser.

### Added

- Auth: permission upgrade UI in the account manager + missing service icons. (#73) — thanks @salmonumbrella.
- CLI: auth aliases, `time now`, `--enable-commands` allowlist, and day-of-week JSON fields. (#75) — thanks @salmonumbrella.
- Tasks: repeat schedules + `tasks get` command. (#75) — thanks @salmonumbrella.

### Fixed

- Calendar: propose-time decline sends updates, default events to primary, and improved error guidance. (#75)
- Gmail: resync on stale history 404s and skip missing message fetches without masking non-404 failures. (#70) — thanks @antons.
- Gmail: include `gmail.settings.sharing` scope for filter operations to avoid 403 insufficientPermissions. (#69) — thanks @ryanh-ai.
- Auth: request Gmail settings scopes so settings commands work reliably.
- Auth: account manager upgrade respects managed services and skips Keep OAuth scopes. (#73) — thanks @salmonumbrella.
- Classroom: normalize assignee updates + fix grade update masks; scan pages when filtering coursework/materials by topic; add leave confirmation. (#73, #74) — thanks @salmonumbrella.
- Tasks: normalize due dates to RFC3339 so date-only inputs work reliably (including repeat).
- Timezone: honor `--timezone local` and allow env/config defaults for Gmail + Calendar output. (#79) — thanks @salmonumbrella.
- CLI: enable shell completions and stop flag suggestions after `--`. (#77) — thanks @salmonumbrella.
- Groups: friendlier Cloud Identity errors for consumer accounts and missing scopes.

### Build

- Deps: update Go modules and JS worker dev deps; bump pinned dev tools; switch WSL to v5.

### Tests

- Live: add `scripts/live-test.sh` wrapper and expand smoke coverage across services.
- Calendar: add integration tests for propose-time.
- Gmail: add attachment output tests for `gmail get`.
- Classroom: add integration smoke tests and command coverage.
- Drive: expand `drive drives` coverage (formatting + query/paging params).
- Auth: use `net.ListenConfig.Listen` in tests to satisfy newer lint.

## 0.6.1 - 2026-01-15

### Added

- Gmail: `--body-file` for `send`, `drafts create`, and `drafts update` (use `-` for stdin) to send multi-line plain text.
- Drive: `gog drive drives` lists shared drives (Team Drives). (#67) — thanks @pasogott.
- Sheets: `gog sheets format` applies cell formatting via `--format-json` + `--format-fields`. (#72) — thanks @nilzzzzzz.

### Changed

- Tasks: `gog tasks list` now defaults to `--show-assigned`. (#59) — thanks @tompson.

## 0.6.0 - 2026-01-11

### Added

- Auth: Workspace service accounts (domain-wide delegation) for all services via `gog auth service-account ...` (preferred when configured). (#54) — thanks @pvieito.

### Fixed

- Keep: use `keep.readonly` scope (service account). (#64) — thanks @jeremys.
- Sheets: `gog auth add --services sheets --readonly` now includes Drive read-only scope so `gog sheets export` works. (#62)

### Tests

- Auth: expand scope matrix regression tests for `--readonly` and `--drive-scope`. (#63)

## 0.5.4 - 2026-01-10

### Changed

- Gmail: allow drafts without a recipient; drafts update preserves existing `To` when `--to` is omitted. (#57) — thanks @antons.

### Added

- Auth: `gog auth add --readonly` and `--drive-scope` for least-privilege tokens. (#58) — thanks @jeremys.

### Fixed

- Paths: expand leading `~` in user-provided file paths (e.g. `--out "~/Downloads/file.pdf"`). (#56) — thanks @salmonumbrella.
- Calendar: accept ISO 8601 timezones without colon (e.g. `-0800`) and add `gog calendar list` alias. (#56) — thanks @salmonumbrella.

## 0.5.3 - 2026-01-10

### Fixed

- CLI: infer account when `--account`/`GOG_ACCOUNT` not set (uses keyring default or single stored token).

## 0.5.2 - 2026-01-10

### Fixed

- Release builds: embed version/commit/date so `gog --version` is correct (Homebrew/tap installs too).

## 0.5.1 - 2026-01-09

### Added

- Build: Windows arm64 release binary.

## 0.5.0 - 2026-01-09

### Highlights

- Email open tracking: `gog gmail send --track` + `gog gmail track ...` (Cloudflare Worker backend; optional per-account setup + `--track-split`) (#35) — thanks @salmonumbrella.
- Calendar parity + Workspace: recurrence rules/reminders, Focus Time/OOO/Working Location event types, workspace users list, and Groups/team helpers (#41) — thanks @salmonumbrella.
- Auth + config: JSON5 `config.json`, improved `gog auth status`, `gog auth keyring ...`, and refresh token validation via `gog auth list --check`.
- Secrets UX: safer keyring behavior (headless Linux guard; keychain unlock guidance).
- Keep: Workspace-only Google Keep support — thanks @koala73.

### Features

- Calendar:
  - `gog calendar create|update --rrule/--reminder` for recurrence rules and custom reminders — thanks @salmonumbrella.
  - `gog calendar update --add-attendee ...` to add attendees without losing existing RSVP state.
  - Workspace users list + timezone-aware time windows and flags like `--week-start`.
- Gmail:
  - `gog gmail thread attachments` list/download attachments (#27) — thanks @salmonumbrella.
  - `gog gmail thread get --full` shows complete bodies (default truncates) (#25) — thanks @salmonumbrella.
  - `gog gmail labels create`, reply-all support, thread search date display, and thread-id replies.
  - `gog gmail get --json` includes flattened headers, `unsubscribe`, and extracted `body` (for `--format full`).
  - `gog gmail settings ...` reorg + filter operations now request the right settings scope (thanks @camerondare).
- Keep: list/search/get notes and download attachments (Workspace only; service account via `gog auth keep ...`) — thanks @koala73.
- Contacts: `gog contacts other delete` for removing other contacts (thanks @salmonumbrella).
- Drive: comments subcommand.
- Sheets: `sheets update|append --copy-validation-from ...` copies data validation (#29) — thanks @mahmoudashraf93.
- Auth/services:
  - `docs` service support + service metadata/listing (thanks @mbelinky).
  - `groups` service support for Cloud Identity (Workspace only): `gog auth add <email> --services groups`.
  - `gog auth keyring <auto|keychain|file>` writes `keyring_backend` to `config.json`.
  - `GOG_KEYRING_BACKEND={auto|keychain|file}` to force a backend (use `file` to avoid Keychain prompts; pair with `GOG_KEYRING_PASSWORD`).
- Docs: `docs info`/`docs cat` now use the Docs API (Drive still used for exports/copy/create).
- Build: linux_arm64 release target.

### Fixed

- Calendar: recurring event creation now sets an IANA `timeZone` inferred from `--from/--to` offsets (#53) — thanks @visionik.
- Secrets:
  - Headless Linux no longer hangs on D-Bus; auto-fallback to file backend and timeout guidance for edge cases (fixes #45) — thanks @salmonumbrella.
  - Keyring backend normalization/validation and clearer errors — thanks @salmonumbrella.
  - macOS Keychain: detect “locked” state and offer unlock guidance.
- Auth: OAuth browser flow now finishes immediately after callback; manual OAuth paste accepts EOF; verify requested account matches authorized email; store tokens under the real account email (Google userinfo).
- Auth: `gog auth tokens list` filters non-token keyring entries.
- Gmail: watch push dedupe/historyId sync improvements; List-Unsubscribe extraction; MIME normalization + padded base64url support (#52) — thanks @antons.
- Gmail: drafts update preserves thread/reply headers when updating existing drafts (#55) — thanks @antons.

### Changed

- CLI: help output polish (grouped by default, optional full expansion via `GOG_HELP=full`); colored headings/command names; more flag aliases like `--output`/`--output-dir` (#47) — thanks @salmonumbrella.
- Homebrew/DX: tap installs GitHub release binaries (macOS) to reduce Keychain prompt churn; remove pnpm wrapper in favor of `make gog` targets; `make gog <args>` works without `ARGS=`.
- Auth: `gog auth add` now defaults to `--services user` (`--services all` remains accepted for backwards compatibility).

## 0.4.2 - 2025-12-31

- Gmail: `thread modify` subcommand + `thread get` split (#21) — thanks @alexknowshtml.
- Auth: refreshed account manager + success UI (#20) — thanks @salmonumbrella.
- CLI: migrate from Cobra to Kong (same commands/flags; help/validation wording may differ slightly).
- DX: tighten golangci-lint rules and fix new findings.
- Security: config/attachment/export dirs now created with 0700 permissions.

## 0.4.1 - 2025-12-28

- macOS: release binaries now built with cgo so Keychain backend works (no encrypted file-store fallback / password prompts; Issue #19).

## 0.4.0 - 2025-12-26

### Added

- Resilience: automatic retries + circuit breaker for Google API calls (429/5xx).
- Gmail: batch ops + settings commands (autoforward, delegates, filters, forwarding, send-as, vacation).
- Gmail: `gog gmail thread --download --out-dir ...` for saving thread attachments to a specific directory.
- Calendar: colors, conflicts, search, multi-timezone time.
- Sheets: read/write/update/append/clear + create spreadsheets.
- Sheets: copy spreadsheets via Drive (`gog sheets copy ...`).
- Drive: `gog drive download --format ...` for Google Docs exports (e.g. Sheets to PDF/XLSX, Docs to PDF/DOCX/TXT, Slides to PDF/PPTX).
- Drive: copy files (`gog drive copy ...`).
- Docs/Slides/Sheets: dedicated export commands (`gog docs export`, `gog slides export`, `gog sheets export`).
- Docs: create/copy (`gog docs create`, `gog docs copy`) and print plain text (`gog docs cat`).
- Slides: create/copy (`gog slides create`, `gog slides copy`).
- Auth: browser-based accounts manager (`gog auth manage`).
- DX: shell completion (`gog completion ...`) and `--verbose` logging.

### Fixed

- Gmail: `gog gmail attachment` download now works reliably; avoid re-fetching payload for filename inference and accept padded base64 responses.
- Gmail: `gog gmail thread --download` now saves attachments to the current directory by default and creates missing output directories.
- Sheets: avoid flag collision with global `--json`; values input flag is now `--values-json` for `sheets update|append`.

### Changed

- Internal: reduce duplicate code for Drive-backed exports and tabular/paging output; embed auth UI templates as HTML assets.

## 0.3.0 - 2025-12-26

### Added

- Calendar: `gog calendar calendars` and `gog calendar acl` now support `--max` and `--page` (JSON includes `nextPageToken`).
- Drive: `gog drive permissions` now supports `--max` and `--page` (JSON includes `nextPageToken`).

### Changed

- macOS: stop trying to modify Keychain ACLs (“trust gog”); removed `GOG_KEYCHAIN_TRUST_APPLICATION`.
- BREAKING: remove positional/legacy flags; normalize paging and file output flags.
- BREAKING: replace `--output` with `--json` and `--plain` (and env `GOG_OUTPUT` with `GOG_JSON`/`GOG_PLAIN`).
- BREAKING: destructive commands now require `--force` in non-interactive contexts (or they prompt on TTY).
- BREAKING: `gog calendar create|update` uses `--from/--to` (removed `--start/--end`).
- BREAKING: `gog gmail send|drafts create` uses `--reply-to-message-id` (removed `--reply-to` for message IDs) and `--reply-to` (removed `--reply-to-address`).
- BREAKING: `gog gmail attachment` uses `--name` (removed `--filename`).
- BREAKING: Drive: `drive ls` uses `--parent` (removed positional `folderId`), `drive upload` uses `--parent` (removed `--folder`), `drive move` uses `--parent` (removed positional `newParentId`).
- BREAKING: `gog drive download` uses `--out` (removed positional `destPath`).
- BREAKING: `gog auth tokens export` uses `--out` (removed positional `outPath`).
- BREAKING: `gog auth tokens export` uses `--overwrite` (removed `--force`).

## 0.2.1 - 2025-12-26

### Fixed

- macOS: reduce repeated Keychain password prompts by trusting the `gog` binary by default (set `GOG_KEYCHAIN_TRUST_APPLICATION=0` to disable).

## 0.2.0 - 2025-12-24

### Added

- Gmail: watch + Pub/Sub push handler (`gog gmail watch start|status|renew|stop|serve`) with optional webhook forwarding, include-body, and max-bytes.
- Gmail: history listing via `gog gmail history --since <historyId>`.
- Gmail: HTML bodies for `gmail send` and `gmail drafts create` via `--body-html` (multipart/alternative when combined with `--body`, PR #16 — thanks @shanelindsay).
- Gmail: `--reply-to-address` (sets `Reply-To` header, PR #16 — thanks @shanelindsay).
- Tasks: manage tasklists and tasks (`lists`, `list`, `add`, `update`, `done`, `undo`, `delete`, `clear`, PR #10 — thanks @shanelindsay).
### Changed

- Build: `make` builds `./bin/gog` by default (adds `build` target, PR #12 — thanks @advait).
- Docs: local build instructions now use `make` (PR #12 — thanks @advait).

### Fixed

- Secrets: keyring file-backend fallback now stores encrypted entries in `$(os.UserConfigDir())/gogcli/keyring/` and supports non-interactive via `GOG_KEYRING_PASSWORD` (PR #13 — thanks @advait).
- Gmail: decode base64url attachment/message-part payloads (PR #15 — thanks @shanelindsay).
- Auth: add `people` service (OIDC `profile` scope) so `gog people me` works with `gog auth add --services all`.

## 0.1.1 - 2025-12-17

### Added

- Calendar: respond to invites via `gog calendar respond <calendarId> <eventId> --status accepted|declined|tentative` (optional `--send-updates`).
- People: `gog people me` (quick “me card” / `people/me`).
- Gmail: message get via `gog gmail get <messageId> [--format full|metadata|raw]`.
- Gmail: download a single attachment via `gog gmail attachment <messageId> <attachmentId> [--out PATH]`.

## 0.1.0 - 2025-12-12

Initial public release of `gog`: a single Go CLI that unifies Gmail, Calendar, Drive, and Contacts (People API).

### Added

- Unified CLI (`gog`) with service subcommands: `gmail`, `calendar`, `drive`, `contacts`, plus `auth`.
- OAuth setup and account management:
  - Store OAuth client credentials: `gog auth credentials <credentials.json>`.
  - Authorize accounts and store refresh tokens securely via OS keychain using `github.com/99designs/keyring`.
  - List/remove accounts: `gog auth list`, `gog auth remove <email>`.
  - Token management helpers: `gog auth tokens list|delete|export|import`.
- Consistently parseable output:
  - `--output=text` (tab-separated lists on stdout) and `--output=json` (JSON on stdout).
  - Human hints/progress/errors go to stderr.
- Colorized output in rich TTY (`--color=auto|always|never`), automatically disabled for JSON output.
- Gmail features:
  - Search threads, show thread, generate web URLs.
  - Label listing/get (including counts) and thread label modify.
  - Send mail (supports reply headers + attachments).
  - Drafts: list/get/create/send/delete.
- Calendar features:
  - List calendars, list ACL rules.
  - List/get/create/update/delete events and free/busy queries.
- Drive features:
  - List/search/get files, download (including Google Docs export), upload, mkdir, delete, move, rename.
  - Sharing helpers: share/unshare/permissions, and web URL output.
- Contacts / People API features:
  - Contacts list/search/get/create/update/delete.
  - “Other contacts” list/search.
  - Workspace directory list/search (Workspace accounts only).
- Developer experience:
  - Formatting via `gofumpt` + `goimports` (and `gofmt` implicitly) using `make fmt` / `make fmt-check`.
  - Linting via pinned `golangci-lint` with repo config.
  - Tests using stdlib `testing` + `httptest`, with steadily increased unit coverage.
  - GitHub Actions CI running format checks, tests, and lint.
  - `make` builds `./bin/gog` for local dev (`make && ./bin/gog auth add you@gmail.com`).

### Notes / Known Limitations

- Importing tokens into macOS Keychain may require a local (GUI) session; headless/SSH sessions can fail due to Keychain user-interaction restrictions.
- Workspace directory commands require a Google Workspace account; `@gmail.com` accounts will not work for directory endpoints.
