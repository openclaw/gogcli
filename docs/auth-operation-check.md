# Diagnose one operation without executing it

```sh
gog auth check-operation sheets.datasource.refresh --json
gog auth check-operation docs.write --resource DOC_ID --probe --json
```

The command resolves the supplied command path against the current CLI and reports gog command-policy decisions. Aliases are normalized. Unknown commands fail; known commands without a modeled diagnostic return unknown requirements.

Operation-specific checks currently cover `docs.cat`, `docs.write`, `sheets.get`, `sheets.update`, `sheets.datasource.refresh`, `drive.get`, and `drive.upload`. Stored OAuth scopes are reported as recorded-present, recorded-missing, or unknown. They are not treated as proof of current token validity or effective permissions. Service-account and direct-access-token grants cannot be established from stored OAuth metadata.

Without `--probe`, there are no Google API requests. With `--probe --resource`, metadata-only GETs test the selected resource through the relevant service; write operations also inspect Drive's canEdit capability when available. No document text is fetched. `--dry-run` suppresses probes.

Results distinguish permitted local policy, recorded scopes, successful resource reads, reported edit capability, and untested execution. A successful GET or canEdit=true never proves that a requested write will succeed. Protected ranges, API-specific policies, BigQuery IAM, and Connected Sheets execution remain separate unknowns. Google 403 errors are preserved rather than guessed to mean only missing OAuth scopes.

The diagnostic itself returns success when a report is produced; automation must inspect check statuses. Use `auth doctor --check` separately to test refresh-token exchange.
