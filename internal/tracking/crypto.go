package tracking

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
)

var (
	errCiphertextTooShort             = errors.New("ciphertext too short")
	errInvalidTrackingKeyVersion      = errors.New("invalid tracking key version")
	errMissingCurrentTrackingKeyValue = errors.New("missing current tracking key version")
	errNoTrackingKeys                 = errors.New("no tracking keys configured")
)

// PixelPayload is encrypted into the tracking pixel URL
// to be decrypted by the worker.
type PixelPayload struct {
	Recipient   string `json:"r"`
	SubjectHash string `json:"s"`
	SentAt      int64  `json:"t"`
}

// Encrypt encrypts a PixelPayload into a legacy URL-safe base64 blob using AES-GCM.
func Encrypt(payload *PixelPayload, keyBase64 string) (string, error) {
	return encryptPayload(payload, keyBase64, 0)
}

// EncryptWithVersion encrypts a PixelPayload and prefixes the ciphertext with
// a one-byte key version so future rotations can select the right key.
func EncryptWithVersion(payload *PixelPayload, keyBase64 string, version int) (string, error) {
	versionByte, err := trackingKeyVersionByte(version)
	if err != nil {
		return "", err
	}

	return encryptPayload(payload, keyBase64, versionByte)
}

func encryptPayload(payload *PixelPayload, keyBase64 string, version byte) (string, error) {
	key, err := base64.StdEncoding.DecodeString(keyBase64)
	if err != nil {
		return "", fmt.Errorf("decode key: %w", err)
	}

	plaintext, err := json.Marshal(payload)
	if err != nil {
		return "", fmt.Errorf("marshal payload: %w", err)
	}

	block, err := aes.NewCipher(key)
	if err != nil {
		return "", fmt.Errorf("new cipher: %w", err)
	}

	aead, err := cipher.NewGCM(block)
	if err != nil {
		return "", fmt.Errorf("new gcm: %w", err)
	}

	nonce := make([]byte, aead.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return "", fmt.Errorf("nonce: %w", err)
	}

	prefix := nonce
	if version > 0 {
		prefix = make([]byte, 0, 1+len(nonce)+len(plaintext)+aead.Overhead())
		prefix = append(prefix, version)
		prefix = append(prefix, nonce...)
	}

	ciphertext := aead.Seal(prefix, nonce, plaintext, nil)

	// URL-safe base64 encode
	return base64.RawURLEncoding.EncodeToString(ciphertext), nil
}

// Decrypt decrypts a URL-safe base64 blob using AES-GCM.
func Decrypt(blob string, keyBase64 string) (*PixelPayload, error) {
	return DecryptWithKeys(blob, map[int]string{1: keyBase64})
}

// DecryptWithKeys decrypts versioned and legacy tracking blobs with active keys.
func DecryptWithKeys(blob string, keysByVersion map[int]string) (*PixelPayload, error) {
	ciphertext, err := base64.RawURLEncoding.DecodeString(blob)
	if err != nil {
		return nil, fmt.Errorf("decode blob: %w", err)
	}

	versions := trackingKeyVersions(keysByVersion)
	if len(versions) == 0 {
		return nil, errNoTrackingKeys
	}

	if len(ciphertext) == 0 {
		return nil, errCiphertextTooShort
	}

	versionedOrder := prioritizeVersion(versions, int(ciphertext[0]))

	versionedPayload, versionedErr := tryDecryptVersions(ciphertext, keysByVersion, versionedOrder, 1)
	if versionedErr == nil {
		return versionedPayload, nil
	}

	payload, err := tryDecryptVersions(ciphertext, keysByVersion, versions, 0)
	if err != nil {
		return nil, fmt.Errorf("decrypt: %w", err)
	}

	return payload, nil
}

func tryDecryptVersions(ciphertext []byte, keysByVersion map[int]string, versions []int, nonceOffset int) (*PixelPayload, error) {
	var lastErr error

	for _, version := range versions {
		key := keysByVersion[version]
		if key == "" {
			continue
		}

		plaintext, err := decryptRaw(ciphertext, key, nonceOffset)
		if err != nil {
			lastErr = err
			continue
		}

		var payload PixelPayload
		if err := json.Unmarshal(plaintext, &payload); err != nil {
			lastErr = err
			continue
		}

		return &payload, nil
	}

	if lastErr == nil {
		lastErr = errNoTrackingKeys
	}

	return nil, lastErr
}

func decryptRaw(ciphertext []byte, keyBase64 string, nonceOffset int) ([]byte, error) {
	key, err := base64.StdEncoding.DecodeString(keyBase64)
	if err != nil {
		return nil, fmt.Errorf("decode key: %w", err)
	}

	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("new cipher: %w", err)
	}

	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("new gcm: %w", err)
	}

	if len(ciphertext) < nonceOffset+aead.NonceSize() {
		return nil, errCiphertextTooShort
	}

	nonce := ciphertext[nonceOffset : nonceOffset+aead.NonceSize()]
	ciphertext = ciphertext[nonceOffset+aead.NonceSize():]

	plaintext, err := aead.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return nil, fmt.Errorf("open payload: %w", err)
	}

	return plaintext, nil
}

func trackingKeyVersions(keysByVersion map[int]string) []int {
	versions := make([]int, 0, len(keysByVersion))
	for version, key := range keysByVersion {
		if version > 0 && version <= 255 && key != "" {
			versions = append(versions, version)
		}
	}

	sort.Ints(versions)

	return versions
}

func trackingKeyVersionByte(version int) (byte, error) {
	if version < 1 || version > 255 {
		return 0, fmt.Errorf("%w: %d", errInvalidTrackingKeyVersion, version)
	}

	return byte(version), nil // #nosec G115 -- version is constrained above.
}

func prioritizeVersion(versions []int, preferred int) []int {
	if preferred < 1 || preferred > 255 {
		return versions
	}

	prioritized := append([]int{}, versions...)
	for i, version := range prioritized {
		if version == preferred {
			return append([]int{version}, append(prioritized[:i], prioritized[i+1:]...)...)
		}
	}

	return prioritized
}

// GenerateKey generates a new 256-bit AES key as base64
func GenerateKey() (string, error) {
	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		return "", fmt.Errorf("generate key: %w", err)
	}

	return base64.StdEncoding.EncodeToString(key), nil
}
