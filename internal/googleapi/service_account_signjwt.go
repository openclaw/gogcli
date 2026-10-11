package googleapi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
	iamcredentials "google.golang.org/api/iamcredentials/v1"
	"google.golang.org/api/option"
)

// SignJWTServiceAccountType marks a stored service account descriptor that
// carries no private key. Tokens are minted by asking the IAM Credentials API
// to sign the JWT assertion (signJwt) with Application Default Credentials,
// which keeps domain-wide delegation working without a downloadable key.
const SignJWTServiceAccountType = "signjwt_service_account"

const (
	defaultServiceAccountTokenURI = "https://oauth2.googleapis.com/token" //nolint:gosec // public endpoint, not a credential
	cloudPlatformScope            = "https://www.googleapis.com/auth/cloud-platform"
	signJWTAssertionLifetime      = time.Hour
	jwtBearerGrantType            = "urn:ietf:params:oauth:grant-type:jwt-bearer" //nolint:gosec // OAuth grant type name, not a credential
)

var (
	errSignJWTClientEmailRequired = errors.New("signjwt service account: client_email is required")
	errSignJWTTokenExchange       = errors.New("signjwt token exchange failed")
)

// SignJWTServiceAccount is the stored descriptor for keyless service accounts.
type SignJWTServiceAccount struct {
	Type        string `json:"type"`
	ClientEmail string `json:"client_email"`
	TokenURI    string `json:"token_uri,omitempty"`
}

// NewSignJWTServiceAccountJSON builds the descriptor stored by
// `gog auth service-account set --signjwt`.
func NewSignJWTServiceAccountJSON(clientEmail string) ([]byte, error) {
	clientEmail = strings.TrimSpace(clientEmail)
	if clientEmail == "" {
		return nil, errSignJWTClientEmailRequired
	}

	data, err := json.MarshalIndent(SignJWTServiceAccount{
		Type:        SignJWTServiceAccountType,
		ClientEmail: clientEmail,
		TokenURI:    defaultServiceAccountTokenURI,
	}, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("encode signjwt service account: %w", err)
	}

	return data, nil
}

// ParseSignJWTServiceAccount reports whether data is a keyless descriptor.
func ParseSignJWTServiceAccount(data []byte) (SignJWTServiceAccount, bool, error) {
	var sa SignJWTServiceAccount
	if err := json.Unmarshal(data, &sa); err != nil {
		return SignJWTServiceAccount{}, false, nil //nolint:nilerr // not a descriptor; key parsing reports the error
	}

	if sa.Type != SignJWTServiceAccountType {
		return SignJWTServiceAccount{}, false, nil
	}

	sa.ClientEmail = strings.TrimSpace(sa.ClientEmail)
	if sa.ClientEmail == "" {
		return SignJWTServiceAccount{}, true, errSignJWTClientEmailRequired
	}

	if strings.TrimSpace(sa.TokenURI) == "" {
		sa.TokenURI = defaultServiceAccountTokenURI
	}

	return sa, true, nil
}

// signJWTSigner signs a JWT claim set as the given service account.
type signJWTSigner func(ctx context.Context, serviceAccount string, payload string) (string, error)

type signJWTTokenSource struct {
	ctx     context.Context //nolint:containedctx // oauth2.TokenSource has no context parameter
	sa      SignJWTServiceAccount
	subject string
	scopes  []string
	sign    signJWTSigner
	client  *http.Client
	now     func() time.Time
}

func newSignJWTTokenSource(ctx context.Context, sa SignJWTServiceAccount, subject string, scopes []string, sign signJWTSigner, client *http.Client) oauth2.TokenSource {
	return oauth2.ReuseTokenSource(nil, &signJWTTokenSource{
		ctx:     ctx,
		sa:      sa,
		subject: serviceAccountSubject(subject, sa.ClientEmail),
		scopes:  scopes,
		sign:    sign,
		client:  client,
		now:     time.Now,
	})
}

// adcSignJWTSigner signs with the IAM Credentials API using Application
// Default Credentials. The caller needs iam.serviceAccounts.signJwt
// (roles/iam.serviceAccountTokenCreator) on the service account.
func adcSignJWTSigner(ctx context.Context, opts ...option.ClientOption) (signJWTSigner, error) {
	creds, err := google.FindDefaultCredentials(ctx, cloudPlatformScope)
	if err != nil {
		return nil, fmt.Errorf("signjwt service account: application default credentials: %w", err)
	}

	service, err := iamcredentials.NewService(ctx, append([]option.ClientOption{option.WithTokenSource(creds.TokenSource)}, opts...)...)
	if err != nil {
		return nil, fmt.Errorf("signjwt service account: iam credentials client: %w", err)
	}

	return iamSignJWTSigner(service), nil
}

func iamSignJWTSigner(service *iamcredentials.Service) signJWTSigner {
	return func(ctx context.Context, serviceAccount string, payload string) (string, error) {
		name := "projects/-/serviceAccounts/" + serviceAccount

		resp, err := service.Projects.ServiceAccounts.SignJwt(name, &iamcredentials.SignJwtRequest{Payload: payload}).Context(ctx).Do()
		if err != nil {
			return "", fmt.Errorf("signjwt %s: %w", serviceAccount, err)
		}

		return resp.SignedJwt, nil
	}
}

func (s *signJWTTokenSource) Token() (*oauth2.Token, error) {
	now := s.now()

	claims := map[string]any{
		"iss":   s.sa.ClientEmail,
		"scope": strings.Join(s.scopes, " "),
		"aud":   s.sa.TokenURI,
		"iat":   now.Unix(),
		"exp":   now.Add(signJWTAssertionLifetime).Unix(),
	}
	if s.subject != "" {
		claims["sub"] = s.subject
	}

	payload, err := json.Marshal(claims)
	if err != nil {
		return nil, fmt.Errorf("signjwt claims: %w", err)
	}

	assertion, err := s.sign(s.ctx, s.sa.ClientEmail, string(payload))
	if err != nil {
		return nil, err
	}

	return s.exchange(assertion)
}

func (s *signJWTTokenSource) exchange(assertion string) (*oauth2.Token, error) {
	form := url.Values{"grant_type": {jwtBearerGrantType}, "assertion": {assertion}}

	req, err := http.NewRequestWithContext(s.ctx, http.MethodPost, s.sa.TokenURI, strings.NewReader(form.Encode()))
	if err != nil {
		return nil, fmt.Errorf("signjwt token request: %w", err)
	}

	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := s.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("signjwt token exchange: %w", err)
	}
	defer resp.Body.Close()

	var body struct {
		AccessToken      string `json:"access_token"`
		TokenType        string `json:"token_type"`
		ExpiresIn        int64  `json:"expires_in"`
		Error            string `json:"error"`
		ErrorDescription string `json:"error_description"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return nil, fmt.Errorf("signjwt token exchange: decode response (HTTP %d): %w", resp.StatusCode, err)
	}

	if resp.StatusCode != http.StatusOK || body.AccessToken == "" {
		return nil, fmt.Errorf("%w: HTTP %d: %s %s", errSignJWTTokenExchange, resp.StatusCode, body.Error, body.ErrorDescription)
	}

	token := &oauth2.Token{AccessToken: body.AccessToken, TokenType: body.TokenType}
	if body.ExpiresIn > 0 {
		token.Expiry = s.now().Add(time.Duration(body.ExpiresIn) * time.Second)
	}

	return token, nil
}
