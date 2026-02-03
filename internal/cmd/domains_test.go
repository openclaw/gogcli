package cmd

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/steipete/gogcli/internal/outfmt"
)

// -----------------------------------------------------------------------------
// DomainsListCmd Tests
// -----------------------------------------------------------------------------

func TestDomainsListCmd(t *testing.T) {
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || !strings.Contains(r.URL.Path, "/domains") {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"domains": []map[string]any{
				{"domainName": "example.com", "isPrimary": true, "verified": true, "creationTime": "1700000000"},
			},
		})
	})
	stubAdminDirectory(t, h)

	flags := &RootFlags{Account: "admin@example.com"}
	cmd := &DomainsListCmd{}

	out := captureStdout(t, func() {
		if err := cmd.Run(testContext(t), flags); err != nil {
			t.Fatalf("Run: %v", err)
		}
	})

	if !strings.Contains(out, "example.com") {
		t.Fatalf("unexpected output: %s", out)
	}
}

// -----------------------------------------------------------------------------
// DomainsGetCmd Tests
// -----------------------------------------------------------------------------

func TestDomainsGetCmd_JSON(t *testing.T) {
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || !strings.Contains(r.URL.Path, "/domains/") {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"domainName":   "example.com",
			"isPrimary":    true,
			"verified":     true,
			"creationTime": "1704067200000", // string for ,string tag
			"domainAliases": []map[string]any{
				{"domainAliasName": "alias1.example.com"},
				{"domainAliasName": "alias2.example.com"},
			},
		})
	})
	stubAdminDirectory(t, h)

	flags := &RootFlags{Account: "admin@example.com"}
	cmd := &DomainsGetCmd{Domain: "example.com"}

	ctx := testContext(t)
	ctx = outfmt.WithMode(ctx, outfmt.Mode{JSON: true})

	out := captureStdout(t, func() {
		if err := cmd.Run(ctx, flags); err != nil {
			t.Fatalf("Run: %v", err)
		}
	})

	if !strings.Contains(out, "example.com") {
		t.Fatalf("expected JSON output with domain, got: %s", out)
	}
	if !strings.Contains(out, "domainName") {
		t.Fatalf("expected JSON output with domainName field, got: %s", out)
	}
}

func TestDomainsGetCmd_Plain(t *testing.T) {
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || !strings.Contains(r.URL.Path, "/domains/") {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"domainName":   "example.com",
			"isPrimary":    true,
			"verified":     true,
			"creationTime": "1704067200000",
		})
	})
	stubAdminDirectory(t, h)

	flags := &RootFlags{Account: "admin@example.com"}
	cmd := &DomainsGetCmd{Domain: "example.com"}

	out := captureStdout(t, func() {
		if err := cmd.Run(testContextWithStdout(t), flags); err != nil {
			t.Fatalf("Run: %v", err)
		}
	})

	if !strings.Contains(out, "Domain:") || !strings.Contains(out, "example.com") {
		t.Fatalf("unexpected output: %s", out)
	}
	if !strings.Contains(out, "Primary:") {
		t.Fatalf("expected Primary field, got: %s", out)
	}
	if !strings.Contains(out, "Verified:") {
		t.Fatalf("expected Verified field, got: %s", out)
	}
}

func TestDomainsGetCmd_WithAliases(t *testing.T) {
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || !strings.Contains(r.URL.Path, "/domains/") {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"domainName":   "example.com",
			"isPrimary":    false,
			"verified":     true,
			"creationTime": "1704067200000",
			"domainAliases": []map[string]any{
				{"domainAliasName": "alias1.example.com"},
				nil, // test nil handling
				{"domainAliasName": "alias2.example.com"},
			},
		})
	})
	stubAdminDirectory(t, h)

	flags := &RootFlags{Account: "admin@example.com"}
	cmd := &DomainsGetCmd{Domain: "example.com"}

	out := captureStdout(t, func() {
		if err := cmd.Run(testContextWithStdout(t), flags); err != nil {
			t.Fatalf("Run: %v", err)
		}
	})

	if !strings.Contains(out, "Aliases:") {
		t.Fatalf("expected Aliases field, got: %s", out)
	}
	if !strings.Contains(out, "alias1.example.com") {
		t.Fatalf("expected alias1, got: %s", out)
	}
	if !strings.Contains(out, "alias2.example.com") {
		t.Fatalf("expected alias2, got: %s", out)
	}
}

func TestDomainsGetCmd_Error(t *testing.T) {
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.NotFound(w, r)
	})
	stubAdminDirectory(t, h)

	flags := &RootFlags{Account: "admin@example.com"}
	cmd := &DomainsGetCmd{Domain: "nonexistent.com"}

	if err := cmd.Run(testContext(t), flags); err == nil {
		t.Fatalf("expected error for nonexistent domain")
	}
}

func TestDomainsGetCmd_MissingAccount(t *testing.T) {
	flags := &RootFlags{}
	cmd := &DomainsGetCmd{Domain: "example.com"}

	if err := cmd.Run(testContext(t), flags); err == nil {
		t.Fatalf("expected error for missing account")
	}
}

// -----------------------------------------------------------------------------
// DomainsCreateCmd Tests
// -----------------------------------------------------------------------------

func TestDomainsCreateCmd_JSON(t *testing.T) {
	var gotDomain string
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && strings.Contains(r.URL.Path, "/domains") {
			var payload map[string]any
			if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			gotDomain = payload["domainName"].(string)
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"domainName":   gotDomain,
				"isPrimary":    false,
				"verified":     false,
				"creationTime": "1704067200000",
			})
			return
		}
		http.NotFound(w, r)
	})
	stubAdminDirectory(t, h)

	flags := &RootFlags{Account: "admin@example.com"}
	cmd := &DomainsCreateCmd{Domain: "newdomain.com"}

	ctx := testContext(t)
	ctx = outfmt.WithMode(ctx, outfmt.Mode{JSON: true})

	out := captureStdout(t, func() {
		if err := cmd.Run(ctx, flags); err != nil {
			t.Fatalf("Run: %v", err)
		}
	})

	if gotDomain != "newdomain.com" {
		t.Fatalf("expected domain newdomain.com, got: %s", gotDomain)
	}
	if !strings.Contains(out, "newdomain.com") {
		t.Fatalf("expected JSON output with domain, got: %s", out)
	}
}

func TestDomainsCreateCmd_Plain(t *testing.T) {
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && strings.Contains(r.URL.Path, "/domains") {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"domainName":   "newdomain.com",
				"isPrimary":    false,
				"verified":     false,
				"creationTime": "1704067200000",
			})
			return
		}
		http.NotFound(w, r)
	})
	stubAdminDirectory(t, h)

	flags := &RootFlags{Account: "admin@example.com"}
	cmd := &DomainsCreateCmd{Domain: "newdomain.com"}

	out := captureStdout(t, func() {
		if err := cmd.Run(testContextWithStdout(t), flags); err != nil {
			t.Fatalf("Run: %v", err)
		}
	})

	if !strings.Contains(out, "Created domain:") || !strings.Contains(out, "newdomain.com") {
		t.Fatalf("unexpected output: %s", out)
	}
}

func TestDomainsCreateCmd_Error(t *testing.T) {
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"error": map[string]any{
				"code":    403,
				"message": "insufficient permissions",
			},
		})
	})
	stubAdminDirectory(t, h)

	flags := &RootFlags{Account: "admin@example.com"}
	cmd := &DomainsCreateCmd{Domain: "newdomain.com"}

	if err := cmd.Run(testContext(t), flags); err == nil {
		t.Fatalf("expected error for failed create")
	}
}

func TestDomainsCreateCmd_MissingAccount(t *testing.T) {
	flags := &RootFlags{}
	cmd := &DomainsCreateCmd{Domain: "newdomain.com"}

	if err := cmd.Run(testContext(t), flags); err == nil {
		t.Fatalf("expected error for missing account")
	}
}

// -----------------------------------------------------------------------------
// DomainsDeleteCmd Tests
// -----------------------------------------------------------------------------

func TestDomainsDeleteCmd_Success(t *testing.T) {
	var deletedDomain string
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodDelete && strings.Contains(r.URL.Path, "/domains/") {
			parts := strings.Split(r.URL.Path, "/domains/")
			if len(parts) > 1 {
				deletedDomain = parts[1]
			}
			w.WriteHeader(http.StatusNoContent)
			return
		}
		http.NotFound(w, r)
	})
	stubAdminDirectory(t, h)

	flags := &RootFlags{Account: "admin@example.com", Force: true}
	cmd := &DomainsDeleteCmd{Domain: "deleteme.com"}

	out := captureStdout(t, func() {
		if err := cmd.Run(testContextWithStdout(t), flags); err != nil {
			t.Fatalf("Run: %v", err)
		}
	})

	if deletedDomain != "deleteme.com" {
		t.Fatalf("expected domain deleteme.com to be deleted, got: %s", deletedDomain)
	}
	if !strings.Contains(out, "Deleted domain:") || !strings.Contains(out, "deleteme.com") {
		t.Fatalf("unexpected output: %s", out)
	}
}

func TestDomainsDeleteCmd_RequiresConfirmation(t *testing.T) {
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})
	stubAdminDirectory(t, h)

	flags := &RootFlags{Account: "admin@example.com", NoInput: true}
	cmd := &DomainsDeleteCmd{Domain: "deleteme.com"}

	err := cmd.Run(testContext(t), flags)
	if err == nil {
		t.Fatalf("expected error for missing confirmation")
	}
}

func TestDomainsDeleteCmd_Error(t *testing.T) {
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"error": map[string]any{
				"code":    404,
				"message": "domain not found",
			},
		})
	})
	stubAdminDirectory(t, h)

	flags := &RootFlags{Account: "admin@example.com", Force: true}
	cmd := &DomainsDeleteCmd{Domain: "nonexistent.com"}

	if err := cmd.Run(testContext(t), flags); err == nil {
		t.Fatalf("expected error for nonexistent domain")
	}
}

func TestDomainsDeleteCmd_MissingAccount(t *testing.T) {
	flags := &RootFlags{}
	cmd := &DomainsDeleteCmd{Domain: "deleteme.com"}

	if err := cmd.Run(testContext(t), flags); err == nil {
		t.Fatalf("expected error for missing account")
	}
}

// -----------------------------------------------------------------------------
// DomainsAliasesListCmd Tests
// -----------------------------------------------------------------------------

func TestDomainsAliasesListCmd_JSON(t *testing.T) {
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || !strings.Contains(r.URL.Path, "/domainaliases") {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"domainAliases": []map[string]any{
				{
					"domainAliasName":  "alias1.example.com",
					"parentDomainName": "example.com",
					"verified":         true,
					"creationTime":     "1704067200000",
				},
				{
					"domainAliasName":  "alias2.example.com",
					"parentDomainName": "example.com",
					"verified":         false,
					"creationTime":     "1704153600000",
				},
			},
		})
	})
	stubAdminDirectory(t, h)

	flags := &RootFlags{Account: "admin@example.com"}
	cmd := &DomainsAliasesListCmd{}

	ctx := testContext(t)
	ctx = outfmt.WithMode(ctx, outfmt.Mode{JSON: true})

	out := captureStdout(t, func() {
		if err := cmd.Run(ctx, flags); err != nil {
			t.Fatalf("Run: %v", err)
		}
	})

	if !strings.Contains(out, "domainAliases") {
		t.Fatalf("expected JSON output with domainAliases, got: %s", out)
	}
	if !strings.Contains(out, "alias1.example.com") {
		t.Fatalf("expected alias1.example.com in output, got: %s", out)
	}
}

func TestDomainsAliasesListCmd_Plain(t *testing.T) {
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || !strings.Contains(r.URL.Path, "/domainaliases") {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"domainAliases": []map[string]any{
				{
					"domainAliasName":  "alias1.example.com",
					"parentDomainName": "example.com",
					"verified":         true,
					"creationTime":     "1704067200000",
				},
				nil, // test nil handling
				{
					"domainAliasName":  "alias2.example.com",
					"parentDomainName": "example.com",
					"verified":         false,
					"creationTime":     "1704153600000",
				},
			},
		})
	})
	stubAdminDirectory(t, h)

	flags := &RootFlags{Account: "admin@example.com"}
	cmd := &DomainsAliasesListCmd{}

	out := captureStdout(t, func() {
		if err := cmd.Run(testContextWithStdout(t), flags); err != nil {
			t.Fatalf("Run: %v", err)
		}
	})

	if !strings.Contains(out, "ALIAS") || !strings.Contains(out, "PARENT DOMAIN") {
		t.Fatalf("expected table headers, got: %s", out)
	}
	if !strings.Contains(out, "alias1.example.com") {
		t.Fatalf("expected alias1.example.com in output, got: %s", out)
	}
	if !strings.Contains(out, "example.com") {
		t.Fatalf("expected parent domain in output, got: %s", out)
	}
}

func TestDomainsAliasesListCmd_Empty(t *testing.T) {
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || !strings.Contains(r.URL.Path, "/domainaliases") {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"domainAliases": []map[string]any{},
		})
	})
	stubAdminDirectory(t, h)

	flags := &RootFlags{Account: "admin@example.com"}
	cmd := &DomainsAliasesListCmd{}

	// Empty list should not error
	if err := cmd.Run(testContext(t), flags); err != nil {
		t.Fatalf("Run: %v", err)
	}
}

func TestDomainsAliasesListCmd_Error(t *testing.T) {
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.NotFound(w, r)
	})
	stubAdminDirectory(t, h)

	flags := &RootFlags{Account: "admin@example.com"}
	cmd := &DomainsAliasesListCmd{}

	if err := cmd.Run(testContext(t), flags); err == nil {
		t.Fatalf("expected error")
	}
}

func TestDomainsAliasesListCmd_MissingAccount(t *testing.T) {
	flags := &RootFlags{}
	cmd := &DomainsAliasesListCmd{}

	if err := cmd.Run(testContext(t), flags); err == nil {
		t.Fatalf("expected error for missing account")
	}
}

// -----------------------------------------------------------------------------
// DomainsAliasesCreateCmd Tests
// -----------------------------------------------------------------------------

func TestDomainsAliasesCreateCmd_JSON(t *testing.T) {
	var gotAlias, gotParent string
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && strings.Contains(r.URL.Path, "/domainaliases") {
			var payload map[string]any
			if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			gotAlias = payload["domainAliasName"].(string)
			gotParent = payload["parentDomainName"].(string)
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"domainAliasName":  gotAlias,
				"parentDomainName": gotParent,
				"verified":         false,
				"creationTime":     "1704067200000",
			})
			return
		}
		http.NotFound(w, r)
	})
	stubAdminDirectory(t, h)

	flags := &RootFlags{Account: "admin@example.com"}
	cmd := &DomainsAliasesCreateCmd{Alias: "newalias.example.com", Parent: "example.com"}

	ctx := testContext(t)
	ctx = outfmt.WithMode(ctx, outfmt.Mode{JSON: true})

	out := captureStdout(t, func() {
		if err := cmd.Run(ctx, flags); err != nil {
			t.Fatalf("Run: %v", err)
		}
	})

	if gotAlias != "newalias.example.com" {
		t.Fatalf("expected alias newalias.example.com, got: %s", gotAlias)
	}
	if gotParent != "example.com" {
		t.Fatalf("expected parent example.com, got: %s", gotParent)
	}
	if !strings.Contains(out, "newalias.example.com") {
		t.Fatalf("expected JSON output with alias, got: %s", out)
	}
}

func TestDomainsAliasesCreateCmd_Plain(t *testing.T) {
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && strings.Contains(r.URL.Path, "/domainaliases") {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"domainAliasName":  "newalias.example.com",
				"parentDomainName": "example.com",
				"verified":         false,
				"creationTime":     "1704067200000",
			})
			return
		}
		http.NotFound(w, r)
	})
	stubAdminDirectory(t, h)

	flags := &RootFlags{Account: "admin@example.com"}
	cmd := &DomainsAliasesCreateCmd{Alias: "newalias.example.com", Parent: "example.com"}

	out := captureStdout(t, func() {
		if err := cmd.Run(testContextWithStdout(t), flags); err != nil {
			t.Fatalf("Run: %v", err)
		}
	})

	if !strings.Contains(out, "Created domain alias:") {
		t.Fatalf("expected 'Created domain alias:' in output, got: %s", out)
	}
	if !strings.Contains(out, "newalias.example.com") {
		t.Fatalf("expected alias in output, got: %s", out)
	}
	if !strings.Contains(out, "example.com") {
		t.Fatalf("expected parent in output, got: %s", out)
	}
}

func TestDomainsAliasesCreateCmd_Error(t *testing.T) {
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"error": map[string]any{
				"code":    400,
				"message": "invalid domain alias",
			},
		})
	})
	stubAdminDirectory(t, h)

	flags := &RootFlags{Account: "admin@example.com"}
	cmd := &DomainsAliasesCreateCmd{Alias: "invalid", Parent: "example.com"}

	if err := cmd.Run(testContext(t), flags); err == nil {
		t.Fatalf("expected error for invalid alias")
	}
}

func TestDomainsAliasesCreateCmd_MissingAccount(t *testing.T) {
	flags := &RootFlags{}
	cmd := &DomainsAliasesCreateCmd{Alias: "newalias.example.com", Parent: "example.com"}

	if err := cmd.Run(testContext(t), flags); err == nil {
		t.Fatalf("expected error for missing account")
	}
}

// -----------------------------------------------------------------------------
// DomainsAliasesDeleteCmd Tests
// -----------------------------------------------------------------------------

func TestDomainsAliasesDeleteCmd_Success(t *testing.T) {
	var deletedAlias string
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodDelete && strings.Contains(r.URL.Path, "/domainaliases/") {
			parts := strings.Split(r.URL.Path, "/domainaliases/")
			if len(parts) > 1 {
				deletedAlias = parts[1]
			}
			w.WriteHeader(http.StatusNoContent)
			return
		}
		http.NotFound(w, r)
	})
	stubAdminDirectory(t, h)

	flags := &RootFlags{Account: "admin@example.com", Force: true}
	cmd := &DomainsAliasesDeleteCmd{Alias: "deleteme.example.com"}

	out := captureStdout(t, func() {
		if err := cmd.Run(testContextWithStdout(t), flags); err != nil {
			t.Fatalf("Run: %v", err)
		}
	})

	if deletedAlias != "deleteme.example.com" {
		t.Fatalf("expected alias deleteme.example.com to be deleted, got: %s", deletedAlias)
	}
	if !strings.Contains(out, "Deleted domain alias:") || !strings.Contains(out, "deleteme.example.com") {
		t.Fatalf("unexpected output: %s", out)
	}
}

func TestDomainsAliasesDeleteCmd_RequiresConfirmation(t *testing.T) {
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})
	stubAdminDirectory(t, h)

	flags := &RootFlags{Account: "admin@example.com", NoInput: true}
	cmd := &DomainsAliasesDeleteCmd{Alias: "deleteme.example.com"}

	err := cmd.Run(testContext(t), flags)
	if err == nil {
		t.Fatalf("expected error for missing confirmation")
	}
}

func TestDomainsAliasesDeleteCmd_Error(t *testing.T) {
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"error": map[string]any{
				"code":    404,
				"message": "domain alias not found",
			},
		})
	})
	stubAdminDirectory(t, h)

	flags := &RootFlags{Account: "admin@example.com", Force: true}
	cmd := &DomainsAliasesDeleteCmd{Alias: "nonexistent.example.com"}

	if err := cmd.Run(testContext(t), flags); err == nil {
		t.Fatalf("expected error for nonexistent alias")
	}
}

func TestDomainsAliasesDeleteCmd_MissingAccount(t *testing.T) {
	flags := &RootFlags{}
	cmd := &DomainsAliasesDeleteCmd{Alias: "deleteme.example.com"}

	if err := cmd.Run(testContext(t), flags); err == nil {
		t.Fatalf("expected error for missing account")
	}
}
