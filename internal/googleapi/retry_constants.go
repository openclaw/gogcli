package googleapi

import "time"

const (
	// MaxRateLimitRetries is the maximum number of retries on rate-limit
	// responses (429, or 403 with a rate-limit reason). Six doublings from
	// RateLimitBaseDelay wait 63-94s in total, enough to outlast Google's
	// per-minute quotas such as Gmail's "Units per minute per user".
	MaxRateLimitRetries = 6
	// RateLimitBaseDelay is the initial delay for rate limit exponential backoff.
	RateLimitBaseDelay = 1 * time.Second
	// Max5xxRetries is the maximum retries for server errors.
	Max5xxRetries = 1
	// ServerErrorRetryDelay is the delay before retrying on 5xx errors.
	ServerErrorRetryDelay = 1 * time.Second
)
