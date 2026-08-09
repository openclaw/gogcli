package cmd

import (
	"bytes"
	"context"
	"io"
	goruntime "runtime"
	"strings"
	"testing"

	"google.golang.org/api/people/v1"

	"github.com/openclaw/gogcli/internal/app"
	"github.com/openclaw/gogcli/internal/authclient"
	"github.com/openclaw/gogcli/internal/config"
	"github.com/openclaw/gogcli/internal/outfmt"
	"github.com/openclaw/gogcli/internal/secrets"
	"github.com/openclaw/gogcli/internal/ui"
)

func newCmdOutputContext(t *testing.T, stdout, stderr io.Writer) context.Context {
	t.Helper()

	u, err := ui.New(ui.Options{Stdout: stdout, Stderr: stderr, Color: "never"})
	if err != nil {
		t.Fatalf("ui.New: %v", err)
	}
	return withTestRuntime(ui.WithUI(context.Background(), u), func(*app.Runtime) {})
}

func newCmdRuntimeOutputContext(t *testing.T, stdout, stderr io.Writer) context.Context {
	t.Helper()
	return newCmdRuntimeIOContext(t, strings.NewReader(""), stdout, stderr)
}

func newCmdRuntimeIOContext(t *testing.T, stdin io.Reader, stdout, stderr io.Writer) context.Context {
	t.Helper()
	return app.WithRuntime(newCmdOutputContext(t, stdout, stderr), &app.Runtime{IO: app.IO{
		In:  stdin,
		Out: stdout,
		Err: stderr,
	}, KeyringOptions: testKeyringOptions()})
}

func newCmdJSONContext(t *testing.T) context.Context {
	t.Helper()
	return newCmdJSONOutputContext(t, io.Discard, io.Discard)
}

func newCmdJSONOutputContext(t *testing.T, stdout, stderr io.Writer) context.Context {
	t.Helper()
	return outfmt.WithMode(newCmdOutputContext(t, stdout, stderr), outfmt.Mode{JSON: true})
}

func newCmdRuntimeJSONOutputContext(t *testing.T, stdout, stderr io.Writer) context.Context {
	t.Helper()
	return outfmt.WithMode(newCmdRuntimeOutputContext(t, stdout, stderr), outfmt.Mode{JSON: true})
}

func withTestRuntime(ctx context.Context, configure func(*app.Runtime)) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	ctx = withTestClientResolver(ctx)

	runtime := &app.Runtime{}
	if existing, ok := app.FromContext(ctx); ok {
		*runtime = *existing
	}
	if runtime.KeyringOptions == nil {
		runtime.KeyringOptions = testKeyringOptions()
	}
	configure(runtime)
	return app.WithRuntime(ctx, runtime)
}

func withDefaultTestRuntime(ctx context.Context) context.Context {
	return withTestRuntime(ctx, func(*app.Runtime) {})
}

func withTestClientResolver(ctx context.Context) context.Context {
	resolver := config.NewSystemResolver("")
	resolveStores := func() (*config.ConfigStore, *config.ClientCredentialsStore, error) {
		layout, err := resolver.Resolve(config.PathKindConfig, config.PathKindData)
		if err != nil {
			return nil, nil, err
		}
		return config.NewConfigStore(layout), config.NewClientCredentialsStore(layout), nil
	}
	ctx = authclient.WithClientResolver(ctx, func(email string, override string) (string, error) {
		store, files, err := resolveStores()
		if err != nil {
			return "", err
		}
		cfg, err := store.Read()
		if err != nil {
			return "", err
		}
		return config.ResolveClientForAccountWithCredentials(cfg, email, override, func(client string) (bool, error) {
			_, ok, existingErr := files.ExistingPath(client)
			return ok, existingErr
		})
	})
	return authclient.WithEmailReferenceUpdater(ctx, func(oldEmail, newEmail string) error {
		store, _, err := resolveStores()
		if err != nil {
			return err
		}
		return store.MigrateAccountEmailReferences(oldEmail, newEmail)
	})
}

func defaultConfigStoreForTest(t *testing.T) *config.ConfigStore {
	t.Helper()

	return config.NewConfigStore(defaultLayoutForTest(t, config.PathKindConfig))
}

func defaultLayoutForTest(t *testing.T, kinds ...config.PathKind) config.Layout {
	t.Helper()

	layout, err := config.NewSystemResolver("").Resolve(kinds...)
	if err != nil {
		t.Fatalf("resolve test layout: %v", err)
	}
	return layout
}

func defaultCredentialsStoreForTest(t *testing.T) *config.ClientCredentialsStore {
	t.Helper()
	return config.NewClientCredentialsStore(defaultLayoutForTest(t, config.PathKindConfig, config.PathKindData))
}

func withAuthStore(ctx context.Context, store secrets.Store) context.Context {
	return withTestRuntime(ctx, func(runtime *app.Runtime) {
		runtime.Auth.OpenSecretsStore = func() (secrets.Store, error) {
			return store, nil
		}
		if secretStore, ok := store.(secrets.SecretStore); ok {
			runtime.Auth.OpenSecretStore = func() (secrets.SecretStore, error) {
				return secretStore, nil
			}
		}
	})
}

func withAuthOperations(ctx context.Context, operations app.AuthOperations) context.Context {
	return withTestRuntime(ctx, func(runtime *app.Runtime) {
		runtime.Auth = operations
	})
}

func runtimeWithAuthStore(store secrets.Store) *app.Runtime {
	operations := app.AuthOperations{
		OpenSecretsStore: func() (secrets.Store, error) {
			return store, nil
		},
	}
	if secretStore, ok := store.(secrets.SecretStore); ok {
		operations.OpenSecretStore = func() (secrets.SecretStore, error) {
			return secretStore, nil
		}
	}
	return &app.Runtime{Auth: operations, KeyringOptions: testKeyringOptions()}
}

func testKeyringOptions() *secrets.OpenOptions {
	return &secrets.OpenOptions{
		Backend:     "file",
		Password:    "test-password",
		PasswordSet: true,
		GOOS:        goruntime.GOOS,
	}
}

func rootFlagsWithAuthStore(flags *RootFlags, store secrets.Store) *RootFlags {
	if flags == nil {
		flags = &RootFlags{}
	} else {
		clone := *flags
		flags = &clone
	}
	flags.authOperations = runtimeWithAuthStore(store).Auth
	return flags
}

func executeWithPeopleDirectoryTestService(t *testing.T, args []string, svc *people.Service) executeTestResult {
	t.Helper()
	return executeWithTestRuntime(t, args, &app.Runtime{Services: app.Services{
		PeopleDirectory: func(context.Context, string) (*people.Service, error) {
			return svc, nil
		},
	}})
}

type executeTestResult struct {
	stdout string
	stderr string
	err    error
}

func executeWithTestRuntime(t *testing.T, args []string, runtime *app.Runtime) executeTestResult {
	t.Helper()

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	if runtime == nil {
		runtime = &app.Runtime{}
	} else {
		runtimeCopy := *runtime
		runtime = &runtimeCopy
	}
	if runtime.KeyringOptions == nil {
		runtime.KeyringOptions = testKeyringOptions()
	}
	runtime.IO = app.IO{
		In:  strings.NewReader(""),
		Out: &stdout,
		Err: &stderr,
	}
	err := executeWithRuntime(args, runtime)
	return executeTestResult{
		stdout: stdout.String(),
		stderr: stderr.String(),
		err:    err,
	}
}
