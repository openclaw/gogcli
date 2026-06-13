package googleauth

import (
	"context"
	"errors"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/steipete/gogcli/internal/config"
)

func TestManualAuthURL_ReusesState(t *testing.T) {
	origRead := readClientCredentials
	origEndpoint := oauthEndpoint
	origState := randomStateFn

	t.Cleanup(func() {
		readClientCredentials = origRead
		oauthEndpoint = origEndpoint
		randomStateFn = origState
	})

	stateStore := newTestManualStateStore(t)

	readClientCredentials = func(string) (config.ClientCredentials, error) {
		return config.ClientCredentials{ClientID: "id", ClientSecret: "secret"}, nil
	}
	oauthEndpoint = oauth2EndpointForTest("http://example.com")
	stateCalls := 0
	randomStateFn = func() (string, error) {
		stateCalls++
		if stateCalls == 1 {
			return "state1", nil
		}

		return "state2", nil
	}

	res1, err := ManualAuthURL(context.Background(), AuthorizeOptions{
		Scopes:           []string{"s1"},
		Manual:           true,
		ManualStateStore: stateStore,
	})
	if err != nil {
		t.Fatalf("ManualAuthURL: %v", err)
	}

	res2, err := ManualAuthURL(context.Background(), AuthorizeOptions{
		Scopes:           []string{"s1"},
		Manual:           true,
		ManualStateStore: stateStore,
	})
	if err != nil {
		t.Fatalf("ManualAuthURL second: %v", err)
	}

	state1 := authURLState(t, res1.URL)

	state2 := authURLState(t, res2.URL)
	if state1 != "state1" || state2 != "state1" {
		t.Fatalf("expected reused state, got state1=%q state2=%q", state1, state2)
	}

	if !res2.StateReused {
		t.Fatalf("expected state_reused true on second call")
	}

	if stateCalls != 1 {
		t.Fatalf("expected randomStateFn called once, got %d", stateCalls)
	}
}

func TestManualAuthURL_UsesPKCEAndPersistsVerifier(t *testing.T) {
	origRead := readClientCredentials
	origEndpoint := oauthEndpoint
	origState := randomStateFn
	origVerifier := generateVerifierFn

	t.Cleanup(func() {
		readClientCredentials = origRead
		oauthEndpoint = origEndpoint
		randomStateFn = origState
		generateVerifierFn = origVerifier
	})

	stateStore := newTestManualStateStore(t)

	readClientCredentials = func(string) (config.ClientCredentials, error) {
		return config.ClientCredentials{ClientID: "id", ClientSecret: "secret"}, nil
	}
	oauthEndpoint = oauth2EndpointForTest("http://example.com")
	randomStateFn = func() (string, error) { return "state1", nil }
	generateVerifierFn = func() string { return testCodeVerifier }

	res, err := ManualAuthURL(context.Background(), AuthorizeOptions{
		Scopes:           []string{"s1"},
		Manual:           true,
		ManualStateStore: stateStore,
	})
	if err != nil {
		t.Fatalf("ManualAuthURL: %v", err)
	}

	parsed, err := url.Parse(res.URL)
	if err != nil {
		t.Fatalf("parse auth URL: %v", err)
	}

	if got := parsed.Query().Get("code_challenge_method"); got != "S256" {
		t.Fatalf("expected S256 challenge method, got %q", got)
	}

	if got, want := parsed.Query().Get("code_challenge"), pkceChallengeForTest(); got != want {
		t.Fatalf("unexpected code challenge: got %q want %q", got, want)
	}

	if got := parsed.Query().Get("code_verifier"); got != "" {
		t.Fatalf("code_verifier must not be exposed in auth URL, got %q", got)
	}

	st, ok, err := stateStore.load("", []string{"s1"}, false)
	if err != nil {
		t.Fatalf("load manual state: %v", err)
	}

	if !ok {
		t.Fatalf("expected manual state")
	}

	if st.CodeVerifier != testCodeVerifier {
		t.Fatalf("expected persisted verifier")
	}
}

func TestManualAuthURL_UsesRedirectURIOverride(t *testing.T) {
	origRead := readClientCredentials
	origEndpoint := oauthEndpoint
	origState := randomStateFn
	origManualRedirect := manualRedirectURIFn

	t.Cleanup(func() {
		readClientCredentials = origRead
		oauthEndpoint = origEndpoint
		randomStateFn = origState
		manualRedirectURIFn = origManualRedirect
	})

	stateStore := newTestManualStateStore(t)

	readClientCredentials = func(string) (config.ClientCredentials, error) {
		return config.ClientCredentials{ClientID: "id", ClientSecret: "secret"}, nil
	}
	oauthEndpoint = oauth2EndpointForTest("http://example.com")
	randomStateFn = func() (string, error) { return "state1", nil }
	manualRedirectURIFn = func(context.Context) (string, error) {
		t.Fatal("manualRedirectURIFn should not be called when redirect-uri is provided")
		return "", nil
	}

	res, err := ManualAuthURL(context.Background(), AuthorizeOptions{
		Scopes:           []string{"s1"},
		Manual:           true,
		RedirectURI:      "https://host.example/oauth2/callback",
		ManualStateStore: stateStore,
	})
	if err != nil {
		t.Fatalf("ManualAuthURL: %v", err)
	}

	if got := authURLRedirectURI(t, res.URL); got != "https://host.example/oauth2/callback" {
		t.Fatalf("unexpected redirect uri: %q", got)
	}
}

func TestManualAuthURL_ChangesStateWhenRedirectURIOverrideChanges(t *testing.T) {
	origRead := readClientCredentials
	origEndpoint := oauthEndpoint
	origState := randomStateFn
	origManualRedirect := manualRedirectURIFn

	t.Cleanup(func() {
		readClientCredentials = origRead
		oauthEndpoint = origEndpoint
		randomStateFn = origState
		manualRedirectURIFn = origManualRedirect
	})

	stateStore := newTestManualStateStore(t)

	readClientCredentials = func(string) (config.ClientCredentials, error) {
		return config.ClientCredentials{ClientID: "id", ClientSecret: "secret"}, nil
	}
	oauthEndpoint = oauth2EndpointForTest("http://example.com")
	stateCalls := 0
	randomStateFn = func() (string, error) {
		stateCalls++
		if stateCalls == 1 {
			return "state1", nil
		}

		return "state2", nil
	}
	manualRedirectURIFn = func(context.Context) (string, error) {
		t.Fatal("manualRedirectURIFn should not be called when redirect-uri is provided")
		return "", nil
	}

	res1, err := ManualAuthURL(context.Background(), AuthorizeOptions{
		Scopes:           []string{"s1"},
		Manual:           true,
		RedirectURI:      "https://host.example/oauth2/callback",
		ManualStateStore: stateStore,
	})
	if err != nil {
		t.Fatalf("ManualAuthURL first: %v", err)
	}

	res2, err := ManualAuthURL(context.Background(), AuthorizeOptions{
		Scopes:           []string{"s1"},
		Manual:           true,
		RedirectURI:      "https://other.example/oauth2/callback",
		ManualStateStore: stateStore,
	})
	if err != nil {
		t.Fatalf("ManualAuthURL second: %v", err)
	}

	if authURLState(t, res1.URL) == authURLState(t, res2.URL) {
		t.Fatalf("expected a new state when redirect uri changes")
	}

	if res2.StateReused {
		t.Fatalf("expected state_reused false when redirect uri changes")
	}

	if stateCalls != 2 {
		t.Fatalf("expected randomStateFn called twice, got %d", stateCalls)
	}
}

func TestNewManualStateStoreRejectsRelativeDir(t *testing.T) {
	t.Parallel()

	_, err := NewManualStateStore(config.Layout{ConfigDir: "relative"})
	if !errors.Is(err, errManualStateDirAbsolute) {
		t.Fatalf("expected absolute-dir error, got %v", err)
	}
}

func TestManualStateStoreLoadDoesNotCreateDir(t *testing.T) {
	t.Parallel()

	dir := filepath.Join(t.TempDir(), "missing")

	store, err := NewManualStateStore(config.Layout{ConfigDir: dir})
	if err != nil {
		t.Fatalf("NewManualStateStore: %v", err)
	}

	if _, ok, err := store.load("", []string{"scope"}, false); err != nil || ok {
		t.Fatalf("load = ok %v, err %v; want cache miss", ok, err)
	}

	if _, err := os.Stat(dir); !os.IsNotExist(err) {
		t.Fatalf("read created state directory: %v", err)
	}
}

func TestManualStateStoreSaveUsesPrivateModes(t *testing.T) {
	t.Parallel()

	dir := filepath.Join(t.TempDir(), "state")

	store, err := NewManualStateStore(config.Layout{ConfigDir: dir})
	if err != nil {
		t.Fatalf("NewManualStateStore: %v", err)
	}

	if saveErr := store.save("default", []string{"scope"}, false, "state123", testRedirectURI, testCodeVerifier); saveErr != nil {
		t.Fatalf("save: %v", saveErr)
	}

	dirInfo, err := os.Stat(dir)
	if err != nil {
		t.Fatalf("stat state dir: %v", err)
	}

	if got := dirInfo.Mode().Perm(); runtime.GOOS != "windows" && got != 0o700 {
		t.Fatalf("state dir mode = %o, want 700", got)
	}

	path, err := store.pathFor("state123")
	if err != nil {
		t.Fatalf("pathFor: %v", err)
	}

	fileInfo, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat state file: %v", err)
	}

	if got := fileInfo.Mode().Perm(); runtime.GOOS != "windows" && got != 0o600 {
		t.Fatalf("state file mode = %o, want 600", got)
	}
}

func TestManualStateStoreRejectsUnsafeState(t *testing.T) {
	t.Parallel()

	root := t.TempDir()

	store, err := NewManualStateStore(config.Layout{ConfigDir: filepath.Join(root, "config")})
	if err != nil {
		t.Fatalf("NewManualStateStore: %v", err)
	}

	marker := filepath.Join(root, "marker.json")
	if writeErr := os.WriteFile(marker, []byte("keep"), 0o600); writeErr != nil {
		t.Fatalf("write marker: %v", writeErr)
	}

	if clearErr := store.clear("../../marker"); !errors.Is(clearErr, errInvalidManualAuthState) {
		t.Fatalf("expected invalid-state error, got %v", clearErr)
	}

	data, err := os.ReadFile(marker)
	if err != nil {
		t.Fatalf("read marker: %v", err)
	}

	if string(data) != "keep" {
		t.Fatalf("marker changed: %q", data)
	}
}

func TestValidateManualStateRejectsUnsafeStateAsMismatch(t *testing.T) {
	t.Parallel()

	store := newTestManualStateStore(t)
	for _, tt := range []struct {
		name         string
		requireState bool
		want         error
	}{
		{name: "optional", want: errStateMismatch},
		{name: "required", requireState: true, want: errManualStateMismatch},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			_, err := validateManualState(AuthorizeOptions{
				ManualStateStore: store,
				RequireState:     tt.requireState,
			}, "../../marker", "")
			if !errors.Is(err, tt.want) {
				t.Fatalf("validateManualState error = %v, want %v", err, tt.want)
			}
		})
	}
}

func TestManualStateStoreExpiresState(t *testing.T) {
	t.Parallel()

	store := newTestManualStateStore(t)
	now := time.Date(2026, time.June, 13, 12, 0, 0, 0, time.UTC)

	store.now = func() time.Time { return now }
	if err := store.save("default", []string{"scope"}, false, "state123", testRedirectURI, testCodeVerifier); err != nil {
		t.Fatalf("save: %v", err)
	}

	store.now = func() time.Time { return now.Add(manualStateTTL + time.Second) }
	if _, ok, err := store.loadState("state123"); err != nil || ok {
		t.Fatalf("loadState = ok %v, err %v; want expired miss", ok, err)
	}

	path, err := store.pathFor("state123")
	if err != nil {
		t.Fatalf("pathFor: %v", err)
	}

	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("expired state file remains: %v", err)
	}
}

func authURLState(t *testing.T, rawURL string) string {
	t.Helper()

	parsed, err := url.Parse(rawURL)
	if err != nil {
		t.Fatalf("parse auth URL: %v", err)
	}

	return parsed.Query().Get("state")
}

func authURLRedirectURI(t *testing.T, rawURL string) string {
	t.Helper()

	parsed, err := url.Parse(rawURL)
	if err != nil {
		t.Fatalf("parse auth URL: %v", err)
	}

	return parsed.Query().Get("redirect_uri")
}
