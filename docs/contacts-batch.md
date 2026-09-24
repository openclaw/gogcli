# Batch Contacts

`gog contacts batch` uses the People API's native batch endpoints. Existing
single-contact `get`, `create`, `update`, and `delete` commands keep their input
and output contracts.

```bash
gog contacts batch get people/c123 people/c456 --json
gog contacts batch create --from-file contacts.json --dry-run --json
gog contacts batch create --from-file contacts.json --json
gog contacts batch update --from-file updates.json --dry-run --json
gog contacts batch update --from-file updates.json --json
gog contacts batch delete people/c123 people/c456 --dry-run --json
gog contacts batch delete people/c123 people/c456 --force --json
```

Select the account explicitly with `--account you@example.com`. These commands
use the existing Contacts OAuth scope; they do not require directory access.
`get` reads contact sources only and includes all mutable fields plus metadata.
Email searches belong to the existing `contacts get` and `contacts search`
commands; batch get and delete require exact `people/...` resource names.

## Create from JSON

The input is an array of People API Person objects, with mutable fields only:

```json
[
  {
    "names": [{"givenName": "Ada", "familyName": "Lovelace"}],
    "emailAddresses": [{"value": "ada@example.com"}]
  },
  {
    "names": [{"givenName": "Grace", "familyName": "Hopper"}]
  }
]
```

Do not include `resourceName`, `etag`, or `metadata` when creating contacts.
Use `--from-file -` to read JSON from stdin. File and stdin input is limited to
32 MiB. Duplicate JSON keys, unsupported fields, malformed arrays, and multiple
entries for singleton fields are rejected before any API request.

## Update with etags

Start with `contacts batch get --json`. Copy each contact's `resourceName` and
`metadata.sources` into a map keyed by resource name, and include only fields
you want to replace:

```json
{
  "people/c123": {
    "metadata": {"sources": [{"type": "CONTACT", "id": "123", "etag": "ETAG_FROM_GET"}]},
    "urls": []
  },
  "people/c456": {
    "metadata": {"sources": [{"type": "CONTACT", "id": "456", "etag": "ETAG_FROM_GET"}]},
    "organizations": [{"name": "Example Ltd"}]
  }
}
```

Every update requires a CONTACT source with its original etag. gog preserves
that etag so the API can reject concurrent changes. A profile etag or only a
top-level Person etag is insufficient. On conflict, read the latest contact,
reapply the intended edits, and submit the new snapshot.

Omitted fields stay unchanged. An explicit `[]` or `null` clears a list field.
`memberships` must include a contact group; it cannot be cleared to an empty
list. Contact fields such as `names`, `birthdays`, `biographies`, and `genders`
allow at most one entry.

The People API uses one update mask per request. gog groups contacts with
identical sets of edited fields, then chunks each group. In the example above,
the URL clear and organization edit become separate requests, preserving the
other contact's fields.

## Limits, results, and recovery

Get, create, and update requests contain at most 200 contacts; delete requests
contain at most 500. Requests run sequentially and stop on the first error or
incomplete response. Multiple requests are not one atomic transaction. Mutation
requests are never automatically retried.

JSON output includes `operation`, `requested`, `completed`, and `batches`.
Each batch identifies its `resource_names` or zero-based create `input_indexes`,
its native API `response`, and a `state`:

- `completed`: the request and its expected response were confirmed.
- `unconfirmed`: the request failed or its response was missing, incomplete,
  or included an error. A mutation might already have happened.
- `not_attempted`: gog stopped before submitting this request.

`completed` counts contacts in fully confirmed batches. Successful individual
responses inside an unconfirmed batch remain available in `response`; they do
not inflate that total. Error details from the API are retained. Entries absent
from Google's `contactErrors` map are not treated as proof of a successful
write. Create responses retain API order; no per-contact input/output ordering
guarantee is added.

An incomplete run prints its JSON result and exits nonzero. `--results-only`
preserves the complete result so recovery information is not dropped. Plain
output is TSV with `BATCH`, `STATE`, `COUNT`, `RESOURCES`, and `ERROR` columns;
create results include returned resource names. Progress goes to stderr.

After a timeout, server error, or incomplete mutation response, inspect affected
contacts before rerunning. Repeating a create can produce duplicates. Do not
replay confirmed batches. No automatic resume or rollback is performed.

`--dry-run` validates the complete input and prints every planned request without
authenticating or calling Google. Delete requires confirmation or `--force`.
`--readonly` permits get and blocks mutations. Single-contact allow rules such
as `contacts.update` do not enable `contacts.batch.update`; add the batch command
explicitly or use an intentional broader prefix. Denying `contacts.create`,
`contacts.update`, or `contacts.delete` also blocks its batch counterpart,
including in baked safety profiles.

## Existing deduplication

`contacts dedupe --apply` still previews and confirms its merge, updates the
primary first, and rejects conflicting or unmergeable data. It rechecks redundant
contacts' CONTACT-source etags in groups of at most 200 immediately before each
native batch delete. If any contact changed or a recheck failed, none of that
pending batch is deleted. Google provides no conditional delete endpoint, so
the recheck and delete are separate requests.

Failed applies preserve confirmed progress. JSON adds `complete: false` and,
when a deletion response is uncertain, `unconfirmed_deletions`. Inspect those
exact resources before retrying. `--results-only` preserves that complete
recovery envelope on failure; successful dedupe results retain their existing
group projection. `--resource people/...` continues to constrain
the existing command to explicitly selected contacts.

## API references

- [people.getBatchGet](https://developers.google.com/people/api/rest/v1/people/getBatchGet)
- [people.batchCreateContacts](https://developers.google.com/people/api/rest/v1/people/batchCreateContacts)
- [people.batchUpdateContacts](https://developers.google.com/people/api/rest/v1/people/batchUpdateContacts)
- [people.batchDeleteContacts](https://developers.google.com/people/api/rest/v1/people/batchDeleteContacts)
