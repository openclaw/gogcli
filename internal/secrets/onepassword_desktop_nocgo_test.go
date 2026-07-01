//go:build !cgo && (darwin || linux)

package secrets

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestOnePasswordDesktopAuthRejectedWithoutCGO(t *testing.T) {
	t.Parallel()

	if OnePasswordDesktopAuthSupported() {
		t.Fatal("desktop app auth unexpectedly supported")
	}

	_, err := newOnePasswordItemsClient(context.Background(), onePasswordConfig{
		authMode:    onePasswordAuthDesktop,
		accountName: "example",
		vaultID:     "vault",
		timeout:     time.Second,
	})
	if !errors.Is(err, errOnePasswordDesktopAuth) {
		t.Fatalf("error = %v, want %v", err, errOnePasswordDesktopAuth)
	}
}
