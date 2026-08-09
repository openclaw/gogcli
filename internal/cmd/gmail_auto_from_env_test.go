package cmd

import (
	"encoding/json"
	"testing"
)

func TestGmailAutoFromAddressedAliasEnvDefaults(t *testing.T) {
	t.Setenv("GOG_GMAIL_AUTO_FROM_ADDRESSED_ALIAS", "1")

	tests := []struct {
		name string
		args []string
	}{
		{name: "reply", args: []string{"gmail", "reply", "m1", "--body", "hello"}},
		{name: "reply all", args: []string{"gmail", "reply-all", "m1", "--body", "hello"}},
		{name: "draft create", args: []string{"gmail", "drafts", "create", "--to", "you@example.com", "--subject", "subject", "--body", "hello"}},
		{name: "draft update", args: []string{"gmail", "drafts", "update", "d1", "--subject", "subject", "--body", "hello"}},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			args := append([]string{"--json", "--dry-run"}, tc.args...)
			result := executeWithTestRuntime(t, args, nil)
			if result.err != nil {
				t.Fatalf("Execute: %v\nstderr=%q", result.err, result.stderr)
			}
			var output struct {
				Request struct {
					AutoFromAddressedAlias bool `json:"auto_from_addressed_alias"`
				} `json:"request"`
			}
			if err := json.Unmarshal([]byte(result.stdout), &output); err != nil {
				t.Fatalf("decode: %v\nout=%q", err, result.stdout)
			}
			if !output.Request.AutoFromAddressedAlias {
				t.Fatalf("environment default was not applied: %s", result.stdout)
			}
		})
	}
}
