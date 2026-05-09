package backup

import (
	"fmt"
	"os"
	"path/filepath"
)

func writeBackupReadme(repo string) error {
	path := filepath.Join(repo, "README.md")
	if _, err := os.Stat(path); err == nil {
		return nil
	}

	const body = `# backup-gog

Encrypted Git backup for Google account data exported by gog.

This repository is written by ` + "`gog backup push`" + `. It is safe to keep on
GitHub because service payloads are encrypted before Git sees them.

## Layout

` + "```text" + `
README.md
manifest.json
data/<service>/<account-hash>/...
` + "```" + `

` + "`manifest.json`" + ` is cleartext and contains format version, export time,
public age recipients, service names, account hashes, shard paths, row counts,
encrypted byte sizes, and plaintext hashes used for verification. Email bodies,
subjects, senders, Drive filenames, contacts, event titles, and other private
Google data stay inside encrypted ` + "`*.jsonl.gz.age`" + ` shards.

## Security Model

Shard contents are deterministic JSONL, gzip-compressed with a fixed timestamp,
and encrypted with age for every configured public recipient. The local
` + "`~/.gog/age.key`" + ` identity is required to decrypt.

Git can still see manifest metadata: export time, public recipients, service
names, account hashes, shard paths, encrypted byte sizes, plaintext shard
hashes, backup cadence, and which encrypted shards changed. Git cannot read
Google content without an age identity.

Anyone who can push to this repository can replace encrypted backup data with
different data encrypted to your public recipient. Keep repository write access
restricted and review unexpected backup commits. If an age identity is
compromised, remove its public recipient and push a new backup; old Git history
may still contain shards decryptable by the compromised key.

## Push

` + "```bash" + `
gog backup push --services gmail
` + "```" + `

The command pulls/rebases this checkout, exports selected Google services,
writes encrypted shards, updates the manifest, commits, and pushes this
repository.

## Verify

` + "```bash" + `
gog backup verify
` + "```" + `

` + "`verify`" + ` decrypts every shard with the local age identity and verifies the
manifest hashes and row counts. It does not restore or write Google data.

## Recovery

Install gog, clone this repo to the path in ` + "`~/.gog/backup.json`" + `,
restore the local age identity file, then run:

` + "```bash" + `
gog backup verify
` + "```" + `

Do not commit the age identity. Only public ` + "`age1...`" + ` recipients belong in
config; ` + "`AGE-SECRET-KEY-...`" + ` values must stay local or in a password manager.
`
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		return fmt.Errorf("write backup readme: %w", err)
	}

	return nil
}
