package cmd

import (
	"bytes"
	"context"
	"os"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/openclaw/gogcli/internal/app"
	"github.com/openclaw/gogcli/internal/authclient"
	"github.com/openclaw/gogcli/internal/config"
	"github.com/openclaw/gogcli/internal/googleapi"
	"github.com/openclaw/gogcli/internal/googleauth"
	"github.com/openclaw/gogcli/internal/secrets"
)

func TestCommandReauthorizationRetainsRuntimeAndRecordedGrant(t *testing.T) {
	setTestConfigHome(t)
	t.Setenv("GOG_KEYRING_BACKEND", "keychain")
	stdin, err := os.CreateTemp(t.TempDir(), "synthetic-confirmation")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := stdin.WriteString("y\n"); err != nil {
		t.Fatal(err)
	}
	if _, err := stdin.Seek(0, 0); err != nil {
		t.Fatal(err)
	}
	oldStdin := os.Stdin
	os.Stdin = stdin
	t.Cleanup(func() { os.Stdin = oldStdin; _ = stdin.Close() })
	runtime := newDefaultRuntime()
	var stderr bytes.Buffer
	runtime.IO = app.IO{In: strings.NewReader("y\n"), Out: &bytes.Buffer{}, Err: &stderr}
	scopes := []string{"https://www.googleapis.com/auth/gmail.readonly", "https://www.googleapis.com/auth/drive.readonly"}
	stored := &secrets.Token{Client: "original-client", Email: "reader@example.invalid", Services: []string{"gmail", "drive"}, Scopes: scopes}
	type marker struct{}
	requestCtx, cancel := context.WithCancel(context.WithValue(context.Background(), marker{}, "request-value"))
	defer cancel()
	ensured, authorized := 0, 0
	store := newMemSecretsStore()
	dependencies := googleapi.AuthDependencies{
		ReadCredentials: func(client string) (config.ClientCredentials, error) {
			if client != "original-client" {
				t.Fatalf("changed OAuth client: %s", client)
			}
			return config.ClientCredentials{ClientID: "synthetic-client", ClientSecret: "synthetic-secret"}, nil
		},
		OpenTokens:            func() (secrets.Store, error) { return store, nil },
		ResolveClient:         func(email, override string) (string, error) { return "original-client", nil },
		UpdateEmailReferences: func(string, string) error { return nil },
	}
	runtime.Auth.EnsureKeychainAccess = func(ctx context.Context) error {
		got, ok := app.FromContext(ctx)
		if !ok || got != runtime {
			t.Fatal("lost command runtime in keychain callback")
		}
		if ctx.Value(marker{}) != "request-value" || ctx.Done() != requestCtx.Done() {
			t.Fatal("request context replaced")
		}
		ensured++
		return nil
	}
	runtime.Auth.AuthorizeGoogle = func(ctx context.Context, opts googleauth.AuthorizeOptions) (string, error) {
		if opts.Client != "original-client" || !reflect.DeepEqual(opts.Scopes, scopes) {
			t.Fatalf("changed client or recorded scopes: %#v", opts)
		}
		got, ok := app.FromContext(ctx)
		if !ok || got != runtime {
			t.Fatal("lost runtime during authorization")
		}
		if _, err := authclient.ReadCredentials(ctx, opts.Client); err != nil {
			t.Fatalf("lost OAuth credentials reader: %v", err)
		}
		if _, err := authclient.OpenSecretsStore(ctx); err != nil {
			t.Fatal(err)
		}
		if client, err := authclient.ResolveClient(ctx, "reader@example.invalid"); err != nil || client != "original-client" {
			t.Fatalf("lost client resolver: %s %v", client, err)
		}
		opts.ListenAddr = "::1"
		if _, err := googleauth.Authorize(ctx, opts); err == nil || !strings.Contains(err.Error(), "invalid listen address") {
			t.Fatalf("native Authorize did not reach its offline pre-browser guard: %v", err)
		}
		authorized++
		return "synthetic-refreshed-token", nil
	}
	runtime.Auth.FetchAuthorizedIdentity = func(context.Context, string, string, []string, time.Duration) (googleauth.Identity, error) {
		return googleauth.Identity{Email: "reader@example.invalid", Subject: "synthetic-subject"}, nil
	}
	token, err := commandReauthorizer(runtime, dependencies)(requestCtx, "reader@example.invalid", "original-client", []string{"gmail"}, []string{scopes[0]}, stored)
	if err != nil {
		t.Fatalf("reauth failed before completing native flow: %v; stderr=%s", err, stderr.String())
	}
	if ensured != 1 || authorized != 1 || token.RefreshToken != "synthetic-refreshed-token" {
		t.Fatalf("native flow not completed: ensure=%d authorize=%d", ensured, authorized)
	}
	if !reflect.DeepEqual(token.Scopes, scopes) || !reflect.DeepEqual(token.Services, []string{"drive", "gmail"}) {
		t.Fatalf("recorded grant changed: %#v", token)
	}
}
