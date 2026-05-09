//nolint:err113,govet,wrapcheck,wsl_v5 // Crypto helpers keep age/gzip errors intact for diagnosis.
package backup

import (
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"filippo.io/age"
)

func EnsureIdentity(path string) (string, error) {
	path = expandHome(path)
	if data, err := os.ReadFile(path); err == nil { // #nosec G304 -- path is the configured local age identity file.
		identity, err := parseIdentity(data)
		if err != nil {
			return "", err
		}
		return identity.Recipient().String(), nil
	} else if !os.IsNotExist(err) {
		return "", err
	}

	identity, err := age.GenerateX25519Identity()
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return "", err
	}
	data := []byte(identity.String() + "\n")
	if err := os.WriteFile(path, data, 0o600); err != nil {
		return "", err
	}
	return identity.Recipient().String(), nil
}

func RecipientFromIdentity(path string) (string, error) {
	data, err := os.ReadFile(expandHome(path))
	if err != nil {
		return "", err
	}
	identity, err := parseIdentity(data)
	if err != nil {
		return "", err
	}
	return identity.Recipient().String(), nil
}

func encryptShard(plaintext []byte, recipientStrings []string) ([]byte, string, error) {
	recipients, err := parseRecipients(recipientStrings)
	if err != nil {
		return nil, "", err
	}
	var compressed bytes.Buffer
	gz := gzip.NewWriter(&compressed)
	gz.ModTime = time.Unix(0, 0).UTC()
	_, _ = gz.Write(plaintext)
	_ = gz.Close()

	var encrypted bytes.Buffer
	w, err := age.Encrypt(&encrypted, recipients...)
	if err != nil {
		return nil, "", err
	}
	_, _ = w.Write(compressed.Bytes())
	if err := w.Close(); err != nil {
		return nil, "", err
	}
	return encrypted.Bytes(), sha256Hex(plaintext), nil
}

func encryptShardToFile(openPlaintext func() (io.ReadCloser, error), dst string, recipientStrings []string) (int64, error) {
	recipients, err := parseRecipients(recipientStrings)
	if err != nil {
		return 0, err
	}
	in, err := openPlaintext()
	if err != nil {
		return 0, err
	}
	defer func() { _ = in.Close() }()
	tmp, err := os.CreateTemp(filepath.Dir(dst), ".shard-*.age")
	if err != nil {
		return 0, err
	}
	tmpPath := tmp.Name()
	cw := &countingWriter{w: tmp}
	ageWriter, err := age.Encrypt(cw, recipients...)
	if err != nil {
		_ = tmp.Close()
		_ = os.Remove(tmpPath)
		return 0, err
	}
	gz := gzip.NewWriter(ageWriter)
	gz.ModTime = time.Unix(0, 0).UTC()
	if _, err := io.Copy(gz, in); err != nil {
		_ = gz.Close()
		_ = ageWriter.Close()
		_ = tmp.Close()
		_ = os.Remove(tmpPath)
		return 0, err
	}
	if err := gz.Close(); err != nil {
		_ = ageWriter.Close()
		_ = tmp.Close()
		_ = os.Remove(tmpPath)
		return 0, err
	}
	if err := ageWriter.Close(); err != nil {
		_ = tmp.Close()
		_ = os.Remove(tmpPath)
		return 0, err
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmpPath)
		return 0, err
	}
	if err := os.Chmod(tmpPath, 0o600); err != nil {
		_ = os.Remove(tmpPath)
		return 0, err
	}
	if err := os.Rename(tmpPath, dst); err != nil {
		_ = os.Remove(tmpPath)
		return 0, err
	}
	return cw.n, nil
}

type countingWriter struct {
	w io.Writer
	n int64
}

func (w *countingWriter) Write(p []byte) (int, error) {
	n, err := w.w.Write(p)
	w.n += int64(n)
	return n, err
}

func decryptShard(ciphertext []byte, identityPath string) ([]byte, error) {
	data, err := os.ReadFile(expandHome(identityPath)) // #nosec G304 -- path is the configured local age identity file.
	if err != nil {
		return nil, err
	}
	identity, err := parseIdentity(data)
	if err != nil {
		return nil, err
	}
	r, err := age.Decrypt(bytes.NewReader(ciphertext), identity)
	if err != nil {
		return nil, err
	}
	gz, err := gzip.NewReader(r)
	if err != nil {
		return nil, err
	}
	defer func() { _ = gz.Close() }()
	plaintext, err := io.ReadAll(gz)
	if err != nil {
		return nil, err
	}
	return plaintext, nil
}

func parseRecipients(values []string) ([]age.Recipient, error) {
	var out []age.Recipient
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		recipient, err := age.ParseX25519Recipient(value)
		if err != nil {
			return nil, fmt.Errorf("parse age recipient: %w", err)
		}
		out = append(out, recipient)
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("at least one age recipient is required")
	}
	return out, nil
}

func parseIdentity(data []byte) (*age.X25519Identity, error) {
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		identity, err := age.ParseX25519Identity(line)
		if err != nil {
			return nil, fmt.Errorf("parse age identity: %w", err)
		}
		return identity, nil
	}
	return nil, fmt.Errorf("age identity file is empty")
}

func sha256Hex(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}
