package googleapi

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestParseSignJWTServiceAccount(t *testing.T) {
	data, err := NewSignJWTServiceAccountJSON("svc@proj.iam.gserviceaccount.com")
	if err != nil {
		t.Fatalf("NewSignJWTServiceAccountJSON: %v", err)
	}

	sa, ok, err := ParseSignJWTServiceAccount(data)
	if !ok || err != nil {
		t.Fatalf("expected descriptor, ok=%v err=%v", ok, err)
	}

	if sa.ClientEmail != "svc@proj.iam.gserviceaccount.com" || sa.TokenURI != defaultServiceAccountTokenURI {
		t.Fatalf("unexpected descriptor: %+v", sa)
	}

	if _, ok, _ := ParseSignJWTServiceAccount([]byte(`{"type":"service_account"}`)); ok {
		t.Fatal("key JSON must not parse as a keyless descriptor")
	}

	if _, ok, err := ParseSignJWTServiceAccount([]byte(`{"type":"signjwt_service_account"}`)); !ok || err == nil {
		t.Fatalf("descriptor without client_email must fail, ok=%v err=%v", ok, err)
	}

	if _, err := NewSignJWTServiceAccountJSON("  "); err == nil {
		t.Fatal("empty client email must fail")
	}
}

func TestSignJWTTokenSource_SignsClaimsAndExchanges(t *testing.T) {
	var gotAssertion, gotGrant string

	tokenServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			t.Errorf("parse form: %v", err)
		}
		gotGrant = r.PostForm.Get("grant_type")
		gotAssertion = r.PostForm.Get("assertion")

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"access_token":"access-123","token_type":"Bearer","expires_in":3600}`))
	}))
	defer tokenServer.Close()

	var claims map[string]any
	signs := 0
	sign := func(_ context.Context, serviceAccount string, payload string) (string, error) {
		signs++

		if serviceAccount != "svc@proj.iam.gserviceaccount.com" {
			t.Errorf("signed as %q", serviceAccount)
		}

		if err := json.Unmarshal([]byte(payload), &claims); err != nil {
			t.Errorf("payload is not JSON: %v", err)
		}

		return "signed.jwt.value", nil
	}

	sa := SignJWTServiceAccount{Type: SignJWTServiceAccountType, ClientEmail: "svc@proj.iam.gserviceaccount.com", TokenURI: tokenServer.URL}
	ts := newSignJWTTokenSource(context.Background(), sa, "admin@example.com", []string{"scope-a", "scope-b"}, sign, tokenServer.Client())

	tok, err := ts.Token()
	if err != nil {
		t.Fatalf("Token: %v", err)
	}

	if tok.AccessToken != "access-123" || time.Until(tok.Expiry) < 59*time.Minute {
		t.Fatalf("unexpected token: %+v", tok)
	}

	if gotGrant != jwtBearerGrantType || gotAssertion != "signed.jwt.value" {
		t.Fatalf("unexpected exchange: grant=%q assertion=%q", gotGrant, gotAssertion)
	}

	if claims["iss"] != sa.ClientEmail || claims["sub"] != "admin@example.com" || claims["scope"] != "scope-a scope-b" || claims["aud"] != tokenServer.URL {
		t.Fatalf("unexpected claims: %v", claims)
	}

	if _, err := ts.Token(); err != nil {
		t.Fatalf("second Token: %v", err)
	}

	if signs != 1 {
		t.Fatalf("valid token must be reused, signed %d times", signs)
	}
}

func TestSignJWTTokenSource_PureServiceAccountOmitsSubject(t *testing.T) {
	tokenServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"access_token":"a","expires_in":3600}`))
	}))
	defer tokenServer.Close()

	var claims map[string]any
	sign := func(_ context.Context, _ string, payload string) (string, error) {
		_ = json.Unmarshal([]byte(payload), &claims)
		return "jwt", nil
	}

	sa := SignJWTServiceAccount{Type: SignJWTServiceAccountType, ClientEmail: "svc@proj.iam.gserviceaccount.com", TokenURI: tokenServer.URL}
	if _, err := newSignJWTTokenSource(context.Background(), sa, sa.ClientEmail, []string{"s"}, sign, tokenServer.Client()).Token(); err != nil {
		t.Fatalf("Token: %v", err)
	}

	if _, has := claims["sub"]; has {
		t.Fatalf("subject equal to the service account must be omitted: %v", claims)
	}
}

func TestSignJWTTokenSource_ReportsExchangeError(t *testing.T) {
	tokenServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error":"unauthorized_client","error_description":"Client is unauthorized to retrieve access tokens"}`))
	}))
	defer tokenServer.Close()

	sign := func(context.Context, string, string) (string, error) { return "jwt", nil }
	sa := SignJWTServiceAccount{Type: SignJWTServiceAccountType, ClientEmail: "svc@proj.iam.gserviceaccount.com", TokenURI: tokenServer.URL}

	_, err := newSignJWTTokenSource(context.Background(), sa, "admin@example.com", []string{"s"}, sign, tokenServer.Client()).Token()
	if err == nil {
		t.Fatal("expected exchange error")
	}
}
