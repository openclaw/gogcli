package googleauth

import (
	"context"
	"net/url"
	"testing"

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

	useTempManualStatePath(t)

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
		Scopes: []string{"s1"},
		Manual: true,
	})
	if err != nil {
		t.Fatalf("ManualAuthURL: %v", err)
	}

	res2, err := ManualAuthURL(context.Background(), AuthorizeOptions{
		Scopes: []string{"s1"},
		Manual: true,
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

	useTempManualStatePath(t)

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
		Scopes:      []string{"s1"},
		Manual:      true,
		RedirectURI: "https://host.example/oauth2/callback",
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

	useTempManualStatePath(t)

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
		Scopes:      []string{"s1"},
		Manual:      true,
		RedirectURI: "https://host.example/oauth2/callback",
	})
	if err != nil {
		t.Fatalf("ManualAuthURL first: %v", err)
	}

	res2, err := ManualAuthURL(context.Background(), AuthorizeOptions{
		Scopes:      []string{"s1"},
		Manual:      true,
		RedirectURI: "https://other.example/oauth2/callback",
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
