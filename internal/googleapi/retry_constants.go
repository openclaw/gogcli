package googleapi

import "time"

const (
	// MaxRateLimitRetries is the maximum number of retries on 429 responses.
	MaxRateLimitRetries = 3
	// RateLimitBaseDelay is the initial delay for rate limit exponential backoff.
	RateLimitBaseDelay = 1 * time.Second
	// Max5xxRetries is the maximum retries for server errors.
	Max5xxRetries = 1
	// ServerErrorRetryDelay is the delay before retrying on 5xx errors.
	ServerErrorRetryDelay = 1 * time.Second
	// MaxUnauthorizedRetries is the maximum retries for 401 responses.
	// A single retry allows the oauth2.Transport to re-fetch an access
	// token using the refresh token when the current one has been revoked
	// or expired server-side before its local expiry.
	MaxUnauthorizedRetries = 1
	// UnauthorizedRetryDelay is the delay before retrying on 401 errors.
	UnauthorizedRetryDelay = 500 * time.Millisecond
)
