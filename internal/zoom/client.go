//nolint:wsl_v5
package zoom

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const (
	defaultAPIBaseURL = "https://api.zoom.us/v2"
	defaultTokenURL   = "https://zoom.us/oauth/token" //nolint:gosec // OAuth token endpoint URL, not a credential.
	tokenRefreshSkew  = 60 * time.Second
)

var (
	ErrCredentialsNotFound       = errors.New("Zoom credentials not found. Run `gog zoom auth setup` or set GOG_ZOOM_ACCOUNT_ID, GOG_ZOOM_CLIENT_ID, GOG_ZOOM_CLIENT_SECRET.") //nolint:staticcheck // Exact user-facing string required by issue #589.
	ErrMeetingNotFound           = errors.New("zoom meeting not found")
	ErrZoomRequestFailed         = errors.New("zoom request failed")
	ErrZoomTokenRequestFailed    = errors.New("zoom token request failed")
	ErrZoomTokenMissingAccessKey = errors.New("zoom token response missing access_token")
)

type Credentials struct {
	AccountID    string
	ClientID     string
	ClientSecret string
}

type Client struct {
	credentials Credentials
	alias       string
	httpClient  *http.Client
	now         func() time.Time
}

type Option func(*Client)

func WithRoundTripper(rt http.RoundTripper) Option {
	return func(c *Client) {
		c.httpClient = &http.Client{Transport: rt}
	}
}

func WithHTTPClient(httpClient *http.Client) Option {
	return func(c *Client) {
		if httpClient != nil {
			c.httpClient = httpClient
		}
	}
}

func WithNow(now func() time.Time) Option {
	return func(c *Client) {
		if now != nil {
			c.now = now
		}
	}
}

func NewClient(alias string, credentials Credentials, opts ...Option) (*Client, error) {
	if strings.TrimSpace(credentials.AccountID) == "" ||
		strings.TrimSpace(credentials.ClientID) == "" ||
		strings.TrimSpace(credentials.ClientSecret) == "" {
		return nil, ErrCredentialsNotFound
	}
	c := &Client{
		credentials: Credentials{
			AccountID:    strings.TrimSpace(credentials.AccountID),
			ClientID:     strings.TrimSpace(credentials.ClientID),
			ClientSecret: strings.TrimSpace(credentials.ClientSecret),
		},
		alias:      NormalizeAlias(alias),
		httpClient: http.DefaultClient,
		now:        time.Now,
	}
	for _, opt := range opts {
		opt(c)
	}
	return c, nil
}

type Meeting struct {
	ID        int64  `json:"id,omitempty"`
	UUID      string `json:"uuid,omitempty"`
	Topic     string `json:"topic,omitempty"`
	JoinURL   string `json:"join_url,omitempty"`
	Password  string `json:"password,omitempty"`
	StartURL  string `json:"start_url,omitempty"`
	HostEmail string `json:"host_email,omitempty"`
	IconURI   string `json:"icon_uri,omitempty"`
}

type CreateMeetingRequest struct {
	Topic     string    `json:"topic,omitempty"`
	Type      int       `json:"type,omitempty"`
	StartTime time.Time `json:"-"`
	Duration  int       `json:"duration,omitempty"`
	Timezone  string    `json:"timezone,omitempty"`
	Agenda    string    `json:"agenda,omitempty"`
}

type createMeetingJSON struct {
	Topic     string `json:"topic,omitempty"`
	Type      int    `json:"type,omitempty"`
	StartTime string `json:"start_time,omitempty"`
	Duration  int    `json:"duration,omitempty"`
	Timezone  string `json:"timezone,omitempty"`
	Agenda    string `json:"agenda,omitempty"`
}

func (c *Client) CreateMeeting(ctx context.Context, userID string, req CreateMeetingRequest) (*Meeting, error) {
	userID = strings.TrimSpace(userID)
	if userID == "" {
		userID = "me"
	}
	if req.Type == 0 {
		req.Type = 2
	}
	payload := createMeetingJSON{
		Topic:    strings.TrimSpace(req.Topic),
		Type:     req.Type,
		Duration: req.Duration,
		Timezone: strings.TrimSpace(req.Timezone),
		Agenda:   strings.TrimSpace(req.Agenda),
	}
	if !req.StartTime.IsZero() {
		payload.StartTime = req.StartTime.UTC().Format("2006-01-02T15:04:05Z")
	}
	var meeting Meeting
	if err := c.doJSON(ctx, http.MethodPost, defaultAPIBaseURL+"/users/"+url.PathEscape(userID)+"/meetings", payload, &meeting); err != nil {
		return nil, err
	}
	return &meeting, nil
}

func (c *Client) DeleteMeeting(ctx context.Context, meetingID string) error {
	meetingID = strings.TrimSpace(meetingID)
	if meetingID == "" {
		return ErrMeetingNotFound
	}
	err := c.doJSON(ctx, http.MethodDelete, defaultAPIBaseURL+"/meetings/"+url.PathEscape(meetingID), nil, nil)
	if errors.Is(err, ErrMeetingNotFound) {
		return nil
	}
	return err
}

func (c *Client) Validate(ctx context.Context) error {
	return c.doJSON(ctx, http.MethodGet, defaultAPIBaseURL+"/users/me", nil, nil)
}

func (c *Client) doJSON(ctx context.Context, method, endpoint string, payload any, out any) error {
	token, err := c.accessToken(ctx)
	if err != nil {
		return err
	}
	var body io.Reader
	if payload != nil {
		b, marshalErr := json.Marshal(payload)
		if marshalErr != nil {
			return fmt.Errorf("encode zoom request: %w", marshalErr)
		}
		body = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, endpoint, body)
	if err != nil {
		return fmt.Errorf("build zoom request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+token)
	if payload != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("zoom request failed: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound || resp.StatusCode == http.StatusGone {
		return ErrMeetingNotFound
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("%w: %s: %s", ErrZoomRequestFailed, resp.Status, readSmallBody(resp.Body))
	}
	if out == nil || resp.StatusCode == http.StatusNoContent {
		return nil
	}
	if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
		return fmt.Errorf("decode zoom response: %w", err)
	}
	return nil
}

type tokenResponse struct {
	AccessToken string `json:"access_token"`
	ExpiresIn   int64  `json:"expires_in"`
}

func (c *Client) accessToken(ctx context.Context) (string, error) {
	if tok, ok := c.cachedAccessToken(); ok {
		return tok, nil
	}
	values := url.Values{}
	values.Set("grant_type", "account_credentials")
	values.Set("account_id", c.credentials.AccountID)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, defaultTokenURL+"?"+values.Encode(), nil)
	if err != nil {
		return "", fmt.Errorf("build zoom token request: %w", err)
	}
	req.Header.Set("Authorization", "Basic "+basicAuth(c.credentials.ClientID, c.credentials.ClientSecret))
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("zoom token request failed: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("%w: %s: %s", ErrZoomTokenRequestFailed, resp.Status, readSmallBody(resp.Body))
	}
	var decoded tokenResponse
	if err := json.NewDecoder(resp.Body).Decode(&decoded); err != nil {
		return "", fmt.Errorf("decode zoom token response: %w", err)
	}
	if strings.TrimSpace(decoded.AccessToken) == "" {
		return "", ErrZoomTokenMissingAccessKey
	}
	expiresIn := decoded.ExpiresIn
	if expiresIn <= 0 {
		expiresIn = 3600
	}
	_ = StoreCachedToken(c.alias, CachedToken{
		AccessToken: decoded.AccessToken,
		ExpiresAt:   c.now().UTC().Add(time.Duration(expiresIn) * time.Second),
	})
	return decoded.AccessToken, nil
}

func (c *Client) cachedAccessToken() (string, bool) {
	tok, err := LoadCachedToken(c.alias)
	if err != nil {
		return "", false
	}
	if strings.TrimSpace(tok.AccessToken) == "" {
		return "", false
	}
	if !tok.ExpiresAt.After(c.now().UTC().Add(tokenRefreshSkew)) {
		return "", false
	}
	return tok.AccessToken, true
}

func basicAuth(clientID, clientSecret string) string {
	return base64.StdEncoding.EncodeToString([]byte(clientID + ":" + clientSecret))
}

func readSmallBody(r io.Reader) string {
	b, _ := io.ReadAll(io.LimitReader(r, 4096))
	return strings.TrimSpace(string(b))
}
