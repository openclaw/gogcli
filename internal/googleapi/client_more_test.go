package googleapi

import (
	"context"
	"crypto/tls"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/99designs/keyring"
	"golang.org/x/oauth2"

	"github.com/steipete/gogcli/internal/authclient"
	"github.com/steipete/gogcli/internal/config"
	"github.com/steipete/gogcli/internal/googleauth"
	"github.com/steipete/gogcli/internal/secrets"
)

var (
	errBoom = errors.New("boom")
	errNope = errors.New("nope")
)

type stubStore struct {
	lastClient string
	lastEmail  string
	tok        secrets.Token
	err        error

	setClient string
	setEmail  string
	lastSet   secrets.Token
	setCalls  int
	setErr    error

	deleteClient string
	deleteEmail  string
	deleteCalls  int
	deleteErr    error

	defaultEmail     string
	setDefaultClient string
	setDefaultEmail  string
	setDefaultCalls  int
}

type rotatingTokenSource struct {
	mu   sync.Mutex
	next int
}

func (s *rotatingTokenSource) Token() (*oauth2.Token, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.next++

	return &oauth2.Token{
		AccessToken:  fmt.Sprintf("access-%d", s.next),
		RefreshToken: fmt.Sprintf("refresh-%d", s.next),
	}, nil
}

func (s *stubStore) Keys() ([]string, error) { return nil, nil }
func (s *stubStore) SetToken(client string, email string, tok secrets.Token) error {
	s.setClient = client
	s.setEmail = email
	s.lastSet = tok
	s.setCalls++

	if s.setErr != nil {
		return s.setErr
	}

	s.tok = tok

	return nil
}

func (s *stubStore) DeleteToken(client string, email string) error {
	s.deleteClient = client
	s.deleteEmail = email
	s.deleteCalls++

	if s.deleteErr != nil {
		return s.deleteErr
	}

	return nil
}

func (s *stubStore) DeleteTokenAlias(client string, email string) error {
	return s.DeleteToken(client, email)
}

func (s *stubStore) ListTokens() ([]secrets.Token, error) { return nil, nil }
func (s *stubStore) GetDefaultAccount(string) (string, error) {
	return s.defaultEmail, nil
}

func (s *stubStore) SetDefaultAccount(client string, email string) error {
	s.setDefaultClient = client
	s.setDefaultEmail = email
	s.setDefaultCalls++
	s.defaultEmail = email

	return nil
}

func (s *stubStore) GetToken(client string, email string) (secrets.Token, error) {
	s.lastClient = client
	s.lastEmail = email

	if s.err != nil {
		return secrets.Token{}, s.err
	}

	return s.tok, nil
}

func tokenTestDependencies(openTokens authclient.SecretsStoreOpener) AuthDependencies {
	return AuthDependencies{
		OpenTokens: openTokens,
		UpdateEmailReferences: func(string, string) error {
			return nil
		},
	}
}

func TestTokenSourceForAccountScopes_StoreErrors(t *testing.T) {
	dependencies := tokenTestDependencies(func() (secrets.Store, error) {
		return nil, errBoom
	})

	_, err := tokenSourceForAccountScopesWithStoredScopeCheck(context.Background(), dependencies, "svc", "a@b.com", "default", "id", "secret", []string{"s1"}, false)
	if err == nil || !errors.Is(err, errBoom) {
		t.Fatalf("expected boom, got: %v", err)
	}
}

func TestTokenSourceForAccountScopes_KeyNotFound(t *testing.T) {
	dependencies := tokenTestDependencies(func() (secrets.Store, error) {
		return &stubStore{err: keyring.ErrKeyNotFound}, nil
	})

	_, err := tokenSourceForAccountScopesWithStoredScopeCheck(context.Background(), dependencies, "gmail", "a@b.com", "default", "id", "secret", []string{"s1"}, false)
	if err == nil {
		t.Fatalf("expected error")
	}
	var are *AuthRequiredError

	if !errors.As(err, &are) {
		t.Fatalf("expected AuthRequiredError, got: %T %v", err, err)
	}

	if are.Service != "gmail" || are.Email != "a@b.com" {
		t.Fatalf("unexpected: %#v", are)
	}
}

func TestTokenSourceForAccountScopes_CorruptStoredToken(t *testing.T) {
	dependencies := tokenTestDependencies(func() (secrets.Store, error) {
		return &stubStore{err: fmt.Errorf("read token: %w", secrets.ErrCorruptStoredToken)}, nil
	})

	_, err := tokenSourceForAccountScopesWithStoredScopeCheck(context.Background(), dependencies, "gmail", "a@b.com", "default", "id", "secret", []string{"s1"}, false)
	if err == nil {
		t.Fatalf("expected error")
	}
	var are *AuthRequiredError

	if !errors.As(err, &are) {
		t.Fatalf("expected AuthRequiredError, got: %T %v", err, err)
	}

	if are.Service != "gmail" || are.Email != "a@b.com" || are.Client != "default" {
		t.Fatalf("unexpected: %#v", are)
	}

	if !errors.Is(are.Cause, secrets.ErrCorruptStoredToken) {
		t.Fatalf("expected corrupt stored token cause, got: %v", are.Cause)
	}
}

func TestTokenSourceForAccountScopes_OtherGetError(t *testing.T) {
	dependencies := tokenTestDependencies(func() (secrets.Store, error) {
		return &stubStore{err: errNope}, nil
	})

	_, err := tokenSourceForAccountScopesWithStoredScopeCheck(context.Background(), dependencies, "svc", "a@b.com", "default", "id", "secret", []string{"s1"}, false)
	if err == nil || !errors.Is(err, errNope) {
		t.Fatalf("expected nope, got: %v", err)
	}
}

func TestTokenSourceForAccountScopes_HappyPath(t *testing.T) {
	s := &stubStore{tok: secrets.Token{Email: "a@b.com", RefreshToken: "rt"}}
	dependencies := tokenTestDependencies(func() (secrets.Store, error) { return s, nil })

	ts, err := tokenSourceForAccountScopesWithStoredScopeCheck(context.Background(), dependencies, "svc", "A@B.COM", "default", "id", "secret", []string{"s1"}, false)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}

	if ts == nil {
		t.Fatalf("expected token source")
	}
	// Ensure we pass through the email (store normalizes in production).
	if s.lastEmail != "A@B.COM" {
		t.Fatalf("expected email passed through, got: %q", s.lastEmail)
	}
}

func TestTokenSourceForAccountScopes_RequiredStoredGrantMissing(t *testing.T) {
	dependencies := tokenTestDependencies(func() (secrets.Store, error) {
		return &stubStore{tok: secrets.Token{
			Email:        "a@b.com",
			RefreshToken: "rt",
			Services:     []string{"gmail", "drive"},
			Scopes:       []string{"https://www.googleapis.com/auth/gmail.modify"},
		}}, nil
	})

	_, err := tokenSourceForAccountScopesWithStoredScopeCheck(
		context.Background(),
		dependencies,
		"gmail",
		"a@b.com",
		"default",
		"id",
		"secret",
		[]string{scopeGmailFullAccess},
		true,
	)
	if err == nil {
		t.Fatal("expected insufficient scope error")
	}

	var scopeErr *InsufficientScopeError
	if !errors.As(err, &scopeErr) {
		t.Fatalf("expected InsufficientScopeError, got %T: %v", err, err)
	}

	if got := scopeErr.ReauthorizeCommand; got != "gog auth add a@b.com --services drive,gmail --extra-scopes https://mail.google.com/ --force-consent" {
		t.Fatalf("reauthorize command = %q", got)
	}
}

func TestTokenSourceForAccountScopes_RequiredStoredGrantPresent(t *testing.T) {
	dependencies := tokenTestDependencies(func() (secrets.Store, error) {
		return &stubStore{tok: secrets.Token{
			Email:        "a@b.com",
			RefreshToken: "rt",
			Scopes:       []string{scopeGmailFullAccess},
		}}, nil
	})

	ts, err := tokenSourceForAccountScopesWithStoredScopeCheck(
		context.Background(),
		dependencies,
		"gmail",
		"a@b.com",
		"default",
		"id",
		"secret",
		[]string{scopeGmailFullAccess},
		true,
	)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}

	if ts == nil {
		t.Fatal("expected token source")
	}
}

func TestTokenSourceForAccountScopes_RequiredStoredGrantAllowsLegacyMetadata(t *testing.T) {
	dependencies := tokenTestDependencies(func() (secrets.Store, error) {
		return &stubStore{tok: secrets.Token{Email: "a@b.com", RefreshToken: "rt"}}, nil
	})

	ts, err := tokenSourceForAccountScopesWithStoredScopeCheck(
		context.Background(),
		dependencies,
		"gmail",
		"a@b.com",
		"default",
		"id",
		"secret",
		[]string{scopeGmailFullAccess},
		true,
	)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}

	if ts == nil {
		t.Fatal("expected token source")
	}
}

func TestPersistingTokenSource_PersistsRotatedRefreshToken(t *testing.T) {
	stored := secrets.Token{
		Client:       config.DefaultClientName,
		Email:        "a@b.com",
		RefreshToken: "old-refresh-token",
		Services:     []string{"gmail"},
		Scopes:       []string{"s1"},
		CreatedAt:    time.Unix(1735689600, 0).UTC(),
	}

	store := &stubStore{tok: stored}
	base := oauth2.StaticTokenSource(&oauth2.Token{AccessToken: "access", RefreshToken: "new-refresh-token"})
	ts := newPersistingTokenSource(base, store, config.DefaultClientName, "A@B.COM", stored, "", nil)

	if _, err := ts.Token(); err != nil {
		t.Fatalf("Token: %v", err)
	}

	if store.setCalls != 1 {
		t.Fatalf("expected 1 SetToken call, got %d", store.setCalls)
	}

	if store.setClient != config.DefaultClientName {
		t.Fatalf("unexpected client: %q", store.setClient)
	}

	if store.setEmail != "A@B.COM" {
		t.Fatalf("unexpected email: %q", store.setEmail)
	}

	if store.lastSet.RefreshToken != "new-refresh-token" {
		t.Fatalf("expected rotated refresh token to persist, got %q", store.lastSet.RefreshToken)
	}

	if !reflect.DeepEqual(store.lastSet.Services, stored.Services) {
		t.Fatalf("services changed unexpectedly: %#v", store.lastSet.Services)
	}

	if !reflect.DeepEqual(store.lastSet.Scopes, stored.Scopes) {
		t.Fatalf("scopes changed unexpectedly: %#v", store.lastSet.Scopes)
	}

	if !store.lastSet.CreatedAt.Equal(stored.CreatedAt) {
		t.Fatalf("createdAt changed unexpectedly: %v", store.lastSet.CreatedAt)
	}
}

func TestPersistingTokenSource_ConcurrentTokenCalls(t *testing.T) {
	t.Parallel()

	stored := secrets.Token{
		Client:       config.DefaultClientName,
		Email:        "a@b.com",
		RefreshToken: "initial",
	}
	store := &stubStore{tok: stored}
	source := newPersistingTokenSource(
		&rotatingTokenSource{},
		store,
		config.DefaultClientName,
		"a@b.com",
		stored,
		"",
		nil,
	)

	const callers = 32
	var wait sync.WaitGroup
	errs := make(chan error, callers)
	wait.Add(callers)

	for range callers {
		go func() {
			defer wait.Done()
			_, err := source.Token()

			errs <- err
		}()
	}

	wait.Wait()

	close(errs)

	for err := range errs {
		if err != nil {
			t.Fatalf("Token: %v", err)
		}
	}

	if store.setCalls != callers {
		t.Fatalf("SetToken calls = %d, want %d", store.setCalls, callers)
	}
}

func TestPersistingTokenSource_PersistsAccessToken(t *testing.T) {
	stored := secrets.Token{
		Client:       config.DefaultClientName,
		Email:        "a@b.com",
		RefreshToken: "refresh-token",
	}
	expires := time.Date(2026, 5, 22, 13, 0, 0, 0, time.UTC)

	store := &stubStore{tok: stored}
	base := oauth2.StaticTokenSource(&oauth2.Token{
		AccessToken:  "access-token",
		RefreshToken: "refresh-token",
		Expiry:       expires,
	})
	ts := newPersistingTokenSource(base, store, config.DefaultClientName, "a@b.com", stored, "", nil)

	if _, err := ts.Token(); err != nil {
		t.Fatalf("Token: %v", err)
	}

	if store.setCalls != 1 {
		t.Fatalf("expected 1 SetToken call, got %d", store.setCalls)
	}

	if store.lastSet.AccessToken != "access-token" {
		t.Fatalf("expected access token to persist, got %q", store.lastSet.AccessToken)
	}

	if !store.lastSet.AccessTokenExpiresAt.Equal(expires) {
		t.Fatalf("expected expiry to persist, got %s", store.lastSet.AccessTokenExpiresAt)
	}

	if store.lastSet.RefreshToken != "refresh-token" {
		t.Fatalf("refresh token changed unexpectedly: %q", store.lastSet.RefreshToken)
	}
}

func TestPersistingTokenSource_NoRotationDoesNotPersist(t *testing.T) {
	expires := time.Date(2026, 5, 22, 13, 0, 0, 0, time.UTC)
	stored := secrets.Token{
		Email:                "a@b.com",
		RefreshToken:         "same-token",
		AccessToken:          "access",
		AccessTokenExpiresAt: expires,
	}
	store := &stubStore{tok: stored}
	base := oauth2.StaticTokenSource(&oauth2.Token{AccessToken: "access", RefreshToken: "same-token", Expiry: expires})
	ts := newPersistingTokenSource(base, store, config.DefaultClientName, "a@b.com", stored, "", nil)

	if _, err := ts.Token(); err != nil {
		t.Fatalf("Token: %v", err)
	}

	if store.setCalls != 0 {
		t.Fatalf("expected no SetToken calls, got %d", store.setCalls)
	}
}

func TestPersistingTokenSource_PersistsObservedGrantedScopeUpgrade(t *testing.T) {
	gmailScopes, err := googleauth.Scopes(googleauth.ServiceGmail)
	if err != nil {
		t.Fatalf("gmail scopes: %v", err)
	}
	grantedScopes := normalizeScopeList(append([]string{
		"https://www.googleapis.com/auth/calendar",
		"openid",
	}, gmailScopes...))
	stored := secrets.Token{
		Email:        "a@b.com",
		RefreshToken: "same-token",
		Services:     []string{"calendar"},
		Scopes: []string{
			"https://www.googleapis.com/auth/calendar",
			"openid",
		},
	}
	store := &stubStore{tok: stored}
	base := oauth2.StaticTokenSource((&oauth2.Token{
		AccessToken:  "access",
		RefreshToken: "same-token",
	}).WithExtra(map[string]any{
		"scope": strings.Join(grantedScopes, " "),
	}))
	ts := newPersistingTokenSource(base, store, config.DefaultClientName, "a@b.com", stored, "gmail", nil)

	if _, err := ts.Token(); err != nil {
		t.Fatalf("Token: %v", err)
	}

	if store.setCalls != 1 {
		t.Fatalf("expected 1 SetToken call, got %d", store.setCalls)
	}

	wantServices := []string{"calendar", "gmail"}
	if !reflect.DeepEqual(store.lastSet.Services, wantServices) {
		t.Fatalf("services=%#v want %#v", store.lastSet.Services, wantServices)
	}

	wantScopes := grantedScopes
	if !reflect.DeepEqual(store.lastSet.Scopes, wantScopes) {
		t.Fatalf("scopes=%#v want %#v", store.lastSet.Scopes, wantScopes)
	}
}

func TestPersistingTokenSource_DoesNotAddServiceForPartialObservedGrant(t *testing.T) {
	stored := secrets.Token{
		Email:        "a@b.com",
		RefreshToken: "same-token",
		AccessToken:  "access",
		Services:     []string{"calendar"},
		Scopes:       []string{"https://www.googleapis.com/auth/calendar"},
	}
	store := &stubStore{tok: stored}
	base := oauth2.StaticTokenSource((&oauth2.Token{
		AccessToken:  "access",
		RefreshToken: "same-token",
	}).WithExtra(map[string]any{
		"scope": "https://www.googleapis.com/auth/calendar https://www.googleapis.com/auth/directory.readonly",
	}))
	ts := newPersistingTokenSource(base, store, config.DefaultClientName, "a@b.com", stored, "contacts", nil)

	if _, err := ts.Token(); err != nil {
		t.Fatalf("Token: %v", err)
	}

	if store.setCalls != 1 {
		t.Fatalf("expected 1 SetToken call, got %d", store.setCalls)
	}

	wantServices := []string{"calendar"}
	if !reflect.DeepEqual(store.lastSet.Services, wantServices) {
		t.Fatalf("services=%#v want %#v", store.lastSet.Services, wantServices)
	}

	wantScopes := []string{
		"https://www.googleapis.com/auth/calendar",
		"https://www.googleapis.com/auth/directory.readonly",
	}
	if !reflect.DeepEqual(store.lastSet.Scopes, wantScopes) {
		t.Fatalf("scopes=%#v want %#v", store.lastSet.Scopes, wantScopes)
	}
}

func TestPersistingTokenSource_DoesNotPersistRequestedScopeWithoutObservedGrant(t *testing.T) {
	stored := secrets.Token{
		Email:        "a@b.com",
		RefreshToken: "same-token",
		AccessToken:  "access",
		Services:     []string{"calendar"},
		Scopes:       []string{"https://www.googleapis.com/auth/calendar"},
	}
	store := &stubStore{tok: stored}
	base := oauth2.StaticTokenSource((&oauth2.Token{
		AccessToken:  "access",
		RefreshToken: "same-token",
	}).WithExtra(map[string]any{
		"scope": "https://www.googleapis.com/auth/calendar",
	}))
	ts := newPersistingTokenSource(base, store, config.DefaultClientName, "a@b.com", stored, "gmail", nil)

	if _, err := ts.Token(); err != nil {
		t.Fatalf("Token: %v", err)
	}

	if store.setCalls != 0 {
		t.Fatalf("expected no SetToken calls, got %d", store.setCalls)
	}
}

func TestPersistingTokenSource_BackfillsSubjectFromIDToken(t *testing.T) {
	stored := secrets.Token{Email: "a@b.com", RefreshToken: "same-token"}
	store := &stubStore{tok: stored}
	base := oauth2.StaticTokenSource((&oauth2.Token{
		AccessToken:  "access",
		RefreshToken: "same-token",
	}).WithExtra(map[string]any{
		"id_token": unsignedIDTokenForTest(t, "sub-123", "a@b.com"),
	}))
	ts := newPersistingTokenSource(base, store, config.DefaultClientName, "a@b.com", stored, "", nil)

	if _, err := ts.Token(); err != nil {
		t.Fatalf("Token: %v", err)
	}

	if store.setCalls != 1 {
		t.Fatalf("expected 1 SetToken call, got %d", store.setCalls)
	}

	if store.setEmail != "a@b.com" {
		t.Fatalf("unexpected email: %q", store.setEmail)
	}

	if store.lastSet.Subject != "sub-123" {
		t.Fatalf("expected subject to persist, got %q", store.lastSet.Subject)
	}

	if store.lastSet.RefreshToken != "same-token" {
		t.Fatalf("refresh token changed unexpectedly: %q", store.lastSet.RefreshToken)
	}
}

func TestPersistingTokenSource_MigratesRenamedEmailFromIDToken(t *testing.T) {
	ambientStore := config.NewConfigStore(config.Layout{ConfigDir: t.TempDir()})
	if err := ambientStore.Write(config.File{
		AccountAliases: map[string]string{"work": "old@example.com"},
		AccountClients: map[string]string{"old@example.com": "ambient-client"},
	}); err != nil {
		t.Fatalf("write ambient config: %v", err)
	}

	runtimeStore := config.NewConfigStore(config.Layout{ConfigDir: t.TempDir()})
	if err := runtimeStore.Write(config.File{
		AccountAliases: map[string]string{"work": "old@example.com"},
		AccountClients: map[string]string{"old@example.com": "work-client"},
	}); err != nil {
		t.Fatalf("write runtime config: %v", err)
	}

	stored := secrets.Token{Email: "old@example.com", RefreshToken: "same-token", Subject: "sub-123"}
	store := &stubStore{tok: stored, defaultEmail: "old@example.com"}
	base := oauth2.StaticTokenSource((&oauth2.Token{
		AccessToken:  "access",
		RefreshToken: "same-token",
	}).WithExtra(map[string]any{
		"id_token": unsignedIDTokenForTest(t, "sub-123", "new@example.com"),
	}))
	ts := newPersistingTokenSource(base, store, config.DefaultClientName, "old@example.com", stored, "", runtimeStore.MigrateAccountEmailReferences)

	if _, err := ts.Token(); err != nil {
		t.Fatalf("Token: %v", err)
	}

	if store.setCalls != 1 {
		t.Fatalf("expected 1 SetToken call, got %d", store.setCalls)
	}

	if store.setEmail != "new@example.com" {
		t.Fatalf("expected new email to persist, got %q", store.setEmail)
	}

	if store.lastSet.Email != "new@example.com" || store.lastSet.Subject != "sub-123" {
		t.Fatalf("unexpected stored token: %#v", store.lastSet)
	}

	if store.deleteCalls != 1 || store.deleteClient != config.DefaultClientName || store.deleteEmail != "old@example.com" {
		t.Fatalf("expected old alias delete, got calls=%d client=%q email=%q", store.deleteCalls, store.deleteClient, store.deleteEmail)
	}

	if store.setDefaultCalls != 1 || store.setDefaultClient != config.DefaultClientName || store.setDefaultEmail != "new@example.com" {
		t.Fatalf("expected default migration, got calls=%d client=%q email=%q", store.setDefaultCalls, store.setDefaultClient, store.setDefaultEmail)
	}

	updated, err := runtimeStore.Read()
	if err != nil {
		t.Fatalf("read runtime config: %v", err)
	}

	if updated.AccountAliases["work"] != "new@example.com" {
		t.Fatalf("expected alias migrated, got %#v", updated.AccountAliases)
	}

	if updated.AccountClients["new@example.com"] != "work-client" {
		t.Fatalf("expected account client migrated, got %#v", updated.AccountClients)
	}

	if _, ok := updated.AccountClients["old@example.com"]; ok {
		t.Fatalf("expected old account client removed, got %#v", updated.AccountClients)
	}

	ambient, err := ambientStore.Read()
	if err != nil {
		t.Fatalf("read ambient config: %v", err)
	}

	if ambient.AccountAliases["work"] != "old@example.com" || ambient.AccountClients["old@example.com"] != "ambient-client" {
		t.Fatalf("ambient config changed: %#v", ambient)
	}

	pts, ok := ts.(*persistingTokenSource)
	if !ok {
		t.Fatalf("expected persistingTokenSource")
	}

	if pts.email != "new@example.com" {
		t.Fatalf("expected source email updated, got %q", pts.email)
	}
}

func TestPersistingTokenSource_PersistFailureIsNonFatal(t *testing.T) {
	stored := secrets.Token{Email: "a@b.com", RefreshToken: "old-token"}
	store := &stubStore{tok: stored, setErr: errBoom}
	base := oauth2.StaticTokenSource(&oauth2.Token{AccessToken: "access", RefreshToken: "new-token"})
	ts := newPersistingTokenSource(base, store, config.DefaultClientName, "a@b.com", stored, "", nil)

	tok, err := ts.Token()
	if err != nil {
		t.Fatalf("Token: %v", err)
	}

	if tok.AccessToken != "access" {
		t.Fatalf("unexpected access token: %q", tok.AccessToken)
	}

	if store.setCalls != 1 {
		t.Fatalf("expected 1 SetToken attempt, got %d", store.setCalls)
	}

	if store.tok.RefreshToken != "old-token" {
		t.Fatalf("store should keep old token on persist error, got %q", store.tok.RefreshToken)
	}
}

func unsignedIDTokenForTest(t *testing.T, subject string, email string) string {
	t.Helper()

	header, err := json.Marshal(map[string]string{"alg": "none"})
	if err != nil {
		t.Fatalf("marshal header: %v", err)
	}

	payload, err := json.Marshal(map[string]string{"sub": subject, "email": email})
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}

	return base64.RawURLEncoding.EncodeToString(header) + "." + base64.RawURLEncoding.EncodeToString(payload) + "."
}

func testClientResolverContext(t *testing.T) context.Context {
	t.Helper()

	ctx, _ := testClientResolverContextWithServiceAccounts(t)

	return ctx
}

func testClientResolverContextWithServiceAccounts(t *testing.T) (context.Context, *config.ServiceAccountStore) {
	t.Helper()

	return testServiceAccountContext(t, context.Background())
}

func testServiceAccountContext(t *testing.T, ctx context.Context) (context.Context, *config.ServiceAccountStore) {
	t.Helper()

	root := t.TempDir()
	store := config.NewServiceAccountStore(config.Layout{
		ConfigDir:      filepath.Join(root, "config"),
		DataDir:        filepath.Join(root, "data"),
		ExplicitConfig: true,
		ExplicitData:   true,
	})
	dependencies, ok := authDependenciesFromContext(ctx)

	if !ok {
		dependencies = AuthDependencies{
			ResolveClient: func(_ string, override string) (string, error) {
				return config.NormalizeClientNameOrDefault(override)
			},
			ReadCredentials: func(string) (config.ClientCredentials, error) {
				return config.ClientCredentials{ClientID: "id", ClientSecret: "secret"}, nil
			},
			OpenTokens: func() (secrets.Store, error) {
				return &stubStore{tok: secrets.Token{Email: "a@b.com", RefreshToken: "rt"}}, nil
			},
			UpdateEmailReferences: func(string, string) error {
				return nil
			},
			Mode:                      AuthModeStored,
			ADCTokenSource:            DefaultADCTokenSource,
			ServiceAccountTokenSource: DefaultServiceAccountTokenSource,
		}
	}
	dependencies.ServiceAccounts = func() (*config.ServiceAccountStore, error) {
		return store, nil
	}
	ctx = WithAuthDependencies(ctx, dependencies)

	return ctx, store
}

func TestClientCredentialsForAccountUsesDependencies(t *testing.T) {
	dependencies := AuthDependencies{
		ResolveClient: func(_ string, _ string) (string, error) {
			return "runtime", nil
		},
		ReadCredentials: func(client string) (config.ClientCredentials, error) {
			if client != "runtime" {
				t.Fatalf("client = %q", client)
			}

			return config.ClientCredentials{ClientID: "runtime-id", ClientSecret: "runtime-secret"}, nil
		},
	}

	client, credentials, err := clientCredentialsForAccount(context.Background(), dependencies, "a@b.com")
	if err != nil {
		t.Fatalf("clientCredentialsForAccount: %v", err)
	}

	if client != "runtime" || credentials.ClientID != "runtime-id" || credentials.ClientSecret != "runtime-secret" {
		t.Fatalf("client=%q credentials=%#v", client, credentials)
	}
}

func TestTokenSourceUsesDependencyTokenStore(t *testing.T) {
	store := &stubStore{tok: secrets.Token{Email: "a@b.com", RefreshToken: "rt"}}
	dependencies := tokenTestDependencies(func() (secrets.Store, error) {
		return store, nil
	})

	if _, err := tokenSourceForAccountScopesWithStoredScopeCheck(context.Background(), dependencies, "gmail", "a@b.com", "runtime", "id", "secret", []string{"scope"}, false); err != nil {
		t.Fatalf("tokenSourceForAccountScopes: %v", err)
	}
}

func TestOptionsForAccountScopes_HappyPath(t *testing.T) {
	opts, err := optionsForAccountScopes(testClientResolverContext(t), "svc", "a@b.com", []string{"s1"})
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}

	if len(opts) == 0 {
		t.Fatalf("expected client options")
	}
}

func TestOptionsForAccount_HappyPath(t *testing.T) {
	opts, err := optionsForAccount(testClientResolverContext(t), googleauth.ServiceDrive, "a@b.com")
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}

	if len(opts) == 0 {
		t.Fatalf("expected client options")
	}
}

func TestOptionsForAccountScopes_AccessTokenBypassesStoredAuth(t *testing.T) {
	ctx := authclient.WithAccessToken(context.Background(), "ya29.test-access-token")

	opts, err := optionsForAccountScopes(ctx, "svc", "a@b.com", []string{"s1"})
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}

	if len(opts) == 0 {
		t.Fatalf("expected client options")
	}
}

func TestOptionsForAccountScopes_RequiredStoredGrantAllowsDirectAccessToken(t *testing.T) {
	ctx := authclient.WithAccessToken(context.Background(), "ya29.test-access-token")

	opts, err := optionsForAccountScopesRequiringStoredGrant(ctx, "gmail", "a@b.com", []string{scopeGmailFullAccess})
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}

	if len(opts) == 0 {
		t.Fatal("expected client options")
	}
}

func TestOptionsForAccountScopes_ServiceAccountPreferred(t *testing.T) {
	ctx, serviceAccounts := testClientResolverContextWithServiceAccounts(t)
	if _, err := serviceAccounts.Write("a@b.com", []byte(`{"type":"service_account"}`)); err != nil {
		t.Fatalf("write service account: %v", err)
	}

	dependencies, _ := authDependenciesFromContext(ctx)
	dependencies.ReadCredentials = func(string) (config.ClientCredentials, error) {
		t.Fatalf("readClientCredentials should not be called")
		return config.ClientCredentials{}, nil
	}
	dependencies.OpenTokens = func() (secrets.Store, error) {
		t.Fatalf("openSecretsStore should not be called")
		return nil, errBoom
	}

	called := false
	dependencies.ServiceAccountTokenSource = func(_ context.Context, keyJSON []byte, subject string, scopes []string) (oauth2.TokenSource, error) {
		called = true

		if subject != "a@b.com" {
			t.Fatalf("unexpected subject: %q", subject)
		}

		if len(scopes) != 1 || scopes[0] != "s1" {
			t.Fatalf("unexpected scopes: %#v", scopes)
		}

		if string(keyJSON) == "" {
			t.Fatalf("expected keyJSON")
		}

		return oauth2.StaticTokenSource(&oauth2.Token{AccessToken: "t"}), nil
	}
	ctx = WithAuthDependencies(ctx, dependencies)

	opts, err := optionsForAccountScopes(ctx, "svc", "a@b.com", []string{"s1"})
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}

	if !called {
		t.Fatalf("expected service account token source used")
	}

	if len(opts) == 0 {
		t.Fatalf("expected client options")
	}
}

func TestParseAuthMode(t *testing.T) {
	tests := []struct {
		value string
		want  AuthMode
	}{
		{value: "", want: AuthModeStored},
		{value: "adc", want: AuthModeADC},
		{value: "oauth", want: AuthModeStored},
		{value: "ADC", want: AuthModeStored},
		{value: " adc ", want: AuthModeStored},
	}
	for _, test := range tests {
		if got := ParseAuthMode(test.value); got != test.want {
			t.Fatalf("ParseAuthMode(%q) = %q, want %q", test.value, got, test.want)
		}
	}
}

func TestOptionsForAccountScopes_ADCMode(t *testing.T) {
	called := false
	ctx := WithAuthDependencies(context.Background(), AuthDependencies{
		Mode: AuthModeADC,
		ADCTokenSource: func(_ context.Context, scopes ...string) (oauth2.TokenSource, error) {
			called = true

			if len(scopes) != 1 || scopes[0] != "https://www.googleapis.com/auth/spreadsheets.readonly" {
				t.Fatalf("unexpected scopes: %v", scopes)
			}

			return oauth2.StaticTokenSource(&oauth2.Token{AccessToken: "adc-token"}), nil
		},
		ReadCredentials: func(string) (config.ClientCredentials, error) {
			t.Fatalf("ReadCredentials should not be called in ADC mode")
			return config.ClientCredentials{}, nil
		},
		OpenTokens: func() (secrets.Store, error) {
			t.Fatalf("OpenTokens should not be called in ADC mode")
			return nil, errBoom
		},
	})

	opts, err := optionsForAccountScopes(ctx, "sheets", "adc", []string{
		"https://www.googleapis.com/auth/spreadsheets.readonly",
	})
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}

	if !called {
		t.Fatalf("expected ADC token source to be called")
	}

	if len(opts) == 0 {
		t.Fatalf("expected client options")
	}
}

func TestNewBaseTransport_RespectsProxyAndTLSMinimum(t *testing.T) {
	t.Setenv("HTTPS_PROXY", "http://127.0.0.1:8888")

	transport := newBaseTransport()
	if transport == nil {
		t.Fatalf("expected transport")
		return
	}

	if transport.Proxy == nil {
		t.Fatalf("expected proxy func")
	}

	if transport.TLSClientConfig == nil {
		t.Fatalf("expected TLS config")
	}

	if transport.TLSClientConfig.MinVersion < tls.VersionTLS12 {
		t.Fatalf("expected TLS min version >= 1.2, got %d", transport.TLSClientConfig.MinVersion)
	}

	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, "https://www.googleapis.com", nil)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}

	proxyURL, err := transport.Proxy(req)
	if err != nil {
		t.Fatalf("proxy lookup: %v", err)
	}

	if proxyURL == nil || !strings.Contains(proxyURL.String(), "127.0.0.1:8888") {
		t.Fatalf("expected HTTPS proxy to be honored, got: %v", proxyURL)
	}
}

func TestNewBaseTransport_SetsResponseHeaderTimeout(t *testing.T) {
	transport := newBaseTransport()
	if transport.ResponseHeaderTimeout != responseHeaderTimeout {
		t.Fatalf("expected ResponseHeaderTimeout=%v, got %v", responseHeaderTimeout, transport.ResponseHeaderTimeout)
	}
}

func TestOptionsForAccountScopes_NoClientTimeout(t *testing.T) {
	opts, err := optionsForAccountScopes(testClientResolverContext(t), "svc", "a@b.com", []string{"s1"})
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}

	if len(opts) == 0 {
		t.Fatalf("expected client options")
	}

	// The http.Client returned by optionsForAccountScopes must not set a
	// hard Timeout so that large file downloads (Drive videos, etc.) are
	// not interrupted. Server responsiveness is instead guarded by the
	// transport-level ResponseHeaderTimeout.
	//
	// We cannot easily extract the http.Client from option.ClientOption,
	// so we verify the transport layer instead.
	transport := newBaseTransport()
	if transport.ResponseHeaderTimeout == 0 {
		t.Fatalf("expected ResponseHeaderTimeout to be set on transport")
	}
}

func TestNewHTTPClient_NoRedirectPolicy(t *testing.T) {
	client, err := NewHTTPClient(testClientResolverContext(t), googleauth.ServiceDocs, "a@b.com")
	if err != nil {
		t.Fatalf("NewHTTPClient: %v", err)
	}

	if client.Transport == nil {
		t.Fatal("expected Transport to be set on client")
	}

	if client.CheckRedirect != nil {
		t.Fatal("expected no CheckRedirect on bare client")
	}
}
