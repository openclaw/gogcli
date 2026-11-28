package googleapi

import "time"

const (
	// MaxRateLimitRetries caps rate-limit responses per request, including the
	// first one. The shared backoff may exhaust MaxRateLimitWait sooner.
	MaxRateLimitRetries = 12
	// MaxRateLimitWait bounds retry waits from a request's first rate-limit response.
	MaxRateLimitWait = 120 * time.Second
	// RateLimitBaseDelay is the initial delay for rate limit exponential backoff.
	RateLimitBaseDelay = 1 * time.Second
	// Max5xxRetries is the maximum retries for server errors.
	Max5xxRetries = 1
	// ServerErrorRetryDelay is the delay before retrying on 5xx errors.
	ServerErrorRetryDelay = 1 * time.Second
)
