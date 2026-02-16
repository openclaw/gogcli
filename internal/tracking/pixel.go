package tracking

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strconv"
	"time"
)

// GeneratePixelURL creates a tracking pixel URL for an email
func GeneratePixelURL(cfg *Config, recipient, subject string) (string, string, error) {
	if !cfg.IsConfigured() {
		return "", "", errTrackingNotConfigured
	}

	// Hash subject (first 6 chars)
	subjectHash := hashSubject(subject)

	payload := &PixelPayload{
		Recipient:   recipient,
		SubjectHash: subjectHash,
		SentAt:      time.Now().Unix(),
	}

	trackingKeyVersion := cfg.TrackingCurrentKeyVersion
	if trackingKeyVersion == 0 {
		trackingKeyVersion = 1
	}

	blob, err := EncryptWithVersion(payload, cfg.TrackingKey, strconv.Itoa(trackingKeyVersion))
	if err != nil {
		return "", "", fmt.Errorf("encrypt payload: %w", err)
	}

	pixelURL := fmt.Sprintf("%s/p/%s.gif", cfg.WorkerURL, blob)

	return pixelURL, blob, nil
}

// GeneratePixelHTML returns HTML img tag for the tracking pixel
func GeneratePixelHTML(pixelURL string) string {
	return fmt.Sprintf(
		`<img src="%s" width="1" height="1" style="display:none;width:1px;height:1px;border:0;" alt="" />`,
		pixelURL,
	)
}

func hashSubject(subject string) string {
	h := sha256.Sum256([]byte(subject))
	return hex.EncodeToString(h[:])[:6]
}

var errTrackingNotConfigured = errors.New("tracking not configured")
