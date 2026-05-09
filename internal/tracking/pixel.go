package tracking

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
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

	blob, err := encryptTrackingPayload(payload, cfg)
	if err != nil {
		return "", "", fmt.Errorf("encrypt payload: %w", err)
	}

	pixelURL := fmt.Sprintf("%s/p/%s.gif", cfg.WorkerURL, blob)

	return pixelURL, blob, nil
}

func encryptTrackingPayload(payload *PixelPayload, cfg *Config) (string, error) {
	if cfg.TrackingCurrentKeyVersion > 0 {
		return EncryptWithVersion(payload, cfg.TrackingKey, cfg.TrackingCurrentKeyVersion)
	}

	return Encrypt(payload, cfg.TrackingKey)
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
