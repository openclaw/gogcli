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
	"strconv"
	"strings"
)

var errCiphertextTooShort = errors.New("ciphertext too short")

// PixelPayload is encrypted into the tracking pixel URL
// to be decrypted by the worker.
type PixelPayload struct {
	Recipient   string `json:"r"`
	SubjectHash string `json:"s"`
	SentAt      int64  `json:"t"`
}

const (
	defaultTrackingKeyVersion = "1"
)

// Encrypt encrypts a PixelPayload into a URL-safe base64 blob using AES-GCM.
func Encrypt(payload *PixelPayload, keyBase64 string) (string, error) {
	return EncryptWithVersion(payload, keyBase64, defaultTrackingKeyVersion)
}

// EncryptWithVersion encrypts with an explicit 1-byte key version prefix.
func EncryptWithVersion(payload *PixelPayload, keyBase64 string, keyVersion string) (string, error) {
	version, err := strconv.Atoi(strings.TrimSpace(keyVersion))
	if err != nil {
		return "", fmt.Errorf("invalid key version: %w", err)
	}
	if version < 1 || version > 255 {
		return "", fmt.Errorf("invalid key version: %d", version)
	}

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

	payloadWithVersion := make([]byte, 0, 1+len(nonce)+aead.Overhead()+len(plaintext))
	payloadWithVersion = append(payloadWithVersion, byte(version))
	payloadWithVersion = append(payloadWithVersion, nonce...)
	ciphertext := aead.Seal(payloadWithVersion, nonce, plaintext, nil)

	// URL-safe base64 encode
	return base64.RawURLEncoding.EncodeToString(ciphertext), nil
}

// Decrypt decrypts a URL-safe base64 blob using AES-GCM.
func Decrypt(blob string, keyBase64 string) (*PixelPayload, error) {
	return DecryptWithVersions(blob, map[string]string{
		defaultTrackingKeyVersion: keyBase64,
	})
}

// DecryptWithVersions decrypts a blob by trying the available key versions.
func DecryptWithVersions(blob string, keysByVersion map[string]string) (*PixelPayload, error) {
	ciphertext, err := base64.RawURLEncoding.DecodeString(blob)
	if err != nil {
		return nil, fmt.Errorf("decode blob: %w", err)
	}

	if len(ciphertext) == 0 {
		return nil, errCiphertextTooShort
	}

	orders, err := decryptionVersionOrder(ciphertext, keysByVersion)
	if err != nil {
		return nil, err
	}

	for _, order := range orders {
		key, ok := keysByVersion[order]
		if !ok || key == "" {
			continue
		}

		plaintext, decryptErr := decryptWithOffset(ciphertext, key, 1)
		if decryptErr == nil {
			var payload PixelPayload
			unmarshalErr := json.Unmarshal(plaintext, &payload)
			if unmarshalErr == nil {
				return &payload, nil
			}
			if err == nil {
				err = unmarshalErr
			}
			continue
		}
		if err == nil {
			err = decryptErr
		}
	}

	for _, order := range keyVersions(keysByVersion) {
		key, ok := keysByVersion[order]
		if !ok || key == "" {
			continue
		}

		plaintext, decryptErr := decryptWithOffset(ciphertext, key, 0)
		if decryptErr == nil {
			var payload PixelPayload
			unmarshalErr := json.Unmarshal(plaintext, &payload)
			if unmarshalErr == nil {
				return &payload, nil
			}
			if err == nil {
				err = unmarshalErr
			}
			continue
		}
		if err == nil {
			err = decryptErr
		}
	}

	if err == nil {
		return nil, errCiphertextTooShort
	}

	return nil, fmt.Errorf("decrypt: %w", err)
}

func decryptionVersionOrder(ciphertext []byte, keysByVersion map[string]string) ([]string, error) {
	versions := keyVersions(keysByVersion)
	if len(versions) == 0 {
		return nil, errors.New("no tracking keys configured")
	}

	prefix := int(ciphertext[0])
	prefixVersion := strconv.Itoa(prefix)
	for i, version := range versions {
		if version == prefixVersion {
			result := make([]string, 0, len(versions))
			result = append(result, prefixVersion)
			for j, v := range versions {
				if j != i {
					result = append(result, v)
				}
			}
			return result, nil
		}
	}

	return versions, nil
}

func decryptWithOffset(blob []byte, keyBase64 string, nonceOffset int) ([]byte, error) {
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

	if len(blob) <= nonceOffset {
		return nil, errCiphertextTooShort
	}

	if len(blob) < nonceOffset+aead.NonceSize() {
		return nil, errCiphertextTooShort
	}

	nonce := blob[nonceOffset : nonceOffset+aead.NonceSize()]
	cipherPayload := blob[nonceOffset+aead.NonceSize():]

	plaintext, err := aead.Open(nil, nonce, cipherPayload, nil)
	if err != nil {
		return nil, fmt.Errorf("decrypt: %w", err)
	}

	return plaintext, nil
}

func keyVersions(keysByVersion map[string]string) []string {
	versions := make([]string, 0, len(keysByVersion))
	for version := range keysByVersion {
		if _, err := strconv.Atoi(version); err == nil {
			versions = append(versions, version)
		}
	}

	sort.Slice(versions, func(i, j int) bool {
		iv, _ := strconv.Atoi(versions[i])
		jv, _ := strconv.Atoi(versions[j])
		return iv < jv
	})

	return versions
}

// GenerateKey generates a new 256-bit AES key as base64
func GenerateKey() (string, error) {
	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		return "", fmt.Errorf("generate key: %w", err)
	}

	return base64.StdEncoding.EncodeToString(key), nil
}

