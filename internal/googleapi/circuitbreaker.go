package googleapi

import (
	"log/slog"
	"sync"
	"time"
)

const (
	// CircuitBreakerThreshold is the number of consecutive failures to open the circuit
	CircuitBreakerThreshold = 5
	// CircuitBreakerResetTime is how long to wait before attempting to close the circuit
	CircuitBreakerResetTime = 30 * time.Second
	circuitStateOpen        = "open"
	circuitStateClosed      = "closed"
)

type CircuitBreaker struct {
	mu          sync.Mutex
	failures    int
	lastFailure time.Time
	open        bool
}

func NewCircuitBreaker() *CircuitBreaker {
	return &CircuitBreaker{}
}

func (cb *CircuitBreaker) RecordSuccess() {
	cb.mu.Lock()
	defer cb.mu.Unlock()
	wasOpen := cb.open
	cb.failures = 0
	cb.open = false

	if wasOpen {
		slog.Info("circuit breaker reset")
	}
}

func (cb *CircuitBreaker) RecordFailure() bool {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	cb.failures++
	cb.lastFailure = time.Now()

	if cb.failures >= CircuitBreakerThreshold {
		cb.open = true
		slog.Warn("circuit breaker opened", "failures", cb.failures)

		return true // circuit just opened
	}

	return false
}

func (cb *CircuitBreaker) IsOpen() bool {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	if !cb.open {
		return false
	}
	// Check if reset time has passed
	if time.Since(cb.lastFailure) > CircuitBreakerResetTime {
		cb.open = false
		cb.failures = 0

		slog.Info("circuit breaker attempting reset after timeout")

		return false
	}

	return true
}

func (cb *CircuitBreaker) State() string {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	if cb.open {
		return circuitStateOpen
	}

	return circuitStateClosed
}
