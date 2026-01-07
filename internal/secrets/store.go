package secrets

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"runtime"
	"strings"
	"time"

	"github.com/99designs/keyring"
	"golang.org/x/term"

	"github.com/steipete/gogcli/internal/config"
)

type Store interface {
	Keys() ([]string, error)
	SetToken(email string, tok Token) error
	GetToken(email string) (Token, error)
	DeleteToken(email string) error
	ListTokens() ([]Token, error)
	GetDefaultAccount() (string, error)
	SetDefaultAccount(email string) error
}

type KeyringStore struct {
	ring keyring.Keyring
}

type Token struct {
	Email        string    `json:"email"`
	Services     []string  `json:"services,omitempty"`
	Scopes       []string  `json:"scopes,omitempty"`
	CreatedAt    time.Time `json:"created_at,omitempty"`
	RefreshToken string    `json:"-"`
}

const (
	keyringPasswordEnv = "GOG_KEYRING_PASSWORD" //nolint:gosec // env var name, not a credential
	keyringBackendEnv  = "GOG_KEYRING_BACKEND"  //nolint:gosec // env var name, not a credential
)

var (
	errMissingEmail          = errors.New("missing email")
	errMissingRefreshToken   = errors.New("missing refresh token")
	errNoTTY                 = errors.New("no TTY available for keyring file backend password prompt")
	errInvalidKeyringBackend = errors.New("invalid keyring backend")
)

type KeyringBackendInfo struct {
	Value  string
	Source string
}

const (
	keyringBackendSourceEnv     = "env"
	keyringBackendSourceConfig  = "config"
	keyringBackendSourceDefault = "default"
)

func ResolveKeyringBackendInfo() (KeyringBackendInfo, error) {
	if v := strings.TrimSpace(os.Getenv(keyringBackendEnv)); v != "" {
		return KeyringBackendInfo{Value: strings.ToLower(v), Source: keyringBackendSourceEnv}, nil
	}

	cfg, err := config.ReadConfig()
	if err != nil {
		return KeyringBackendInfo{}, fmt.Errorf("read config: %w", err)
	}

	if cfg.KeyringBackend != "" {
		return KeyringBackendInfo{Value: cfg.KeyringBackend, Source: keyringBackendSourceConfig}, nil
	}

	return KeyringBackendInfo{Value: "auto", Source: keyringBackendSourceDefault}, nil
}

func allowedBackends(info KeyringBackendInfo) ([]keyring.BackendType, error) {
	switch info.Value {
	case "", "auto":
		return nil, nil
	case "keychain":
		return []keyring.BackendType{keyring.KeychainBackend}, nil
	case "file":
		return []keyring.BackendType{keyring.FileBackend}, nil
	default:
		return nil, fmt.Errorf("%w: %q (expected auto, keychain, or file)", errInvalidKeyringBackend, info.Value)
	}
}

// wrapKeychainError wraps keychain errors with helpful guidance on macOS.
func wrapKeychainError(err error) error {
	if err == nil {
		return nil
	}

	if IsKeychainLockedError(err.Error()) {
		return fmt.Errorf("%w\n\nYour macOS keychain is locked. To unlock it, run:\n  security unlock-keychain ~/Library/Keychains/login.keychain-db", err)
	}

	return err
}

func fileKeyringPasswordFuncFrom(password string, isTTY bool) keyring.PromptFunc {
	if password != "" {
		return keyring.FixedStringPrompt(password)
	}

	if isTTY {
		return keyring.TerminalPrompt
	}

	return func(_ string) (string, error) {
		return "", fmt.Errorf("%w; set %s", errNoTTY, keyringPasswordEnv)
	}
}

func fileKeyringPasswordFunc() keyring.PromptFunc {
	return fileKeyringPasswordFuncFrom(os.Getenv(keyringPasswordEnv), term.IsTerminal(int(os.Stdin.Fd())))
}

func OpenDefault() (Store, error) {
	// On Linux/WSL/containers, OS keychains (secret-service/kwallet) may be unavailable.
	// In that case github.com/99designs/keyring falls back to the "file" backend,
	// which *requires* both a directory and a password prompt function.
	keyringDir, err := config.EnsureKeyringDir()
	if err != nil {
		return nil, fmt.Errorf("ensure keyring dir: %w", err)
	}

	backendInfo, err := ResolveKeyringBackendInfo()
	if err != nil {
		return nil, err
	}

	allowedBackends, err := allowedBackends(backendInfo)
	if err != nil {
		return nil, err
	}

	ring, err := keyring.Open(keyring.Config{
		ServiceName:              config.AppName,
		KeychainTrustApplication: runtime.GOOS == "darwin",
		AllowedBackends:          allowedBackends,
		FileDir:                  keyringDir,
		FilePasswordFunc:         fileKeyringPasswordFunc(),
	})
	if err != nil {
		return nil, fmt.Errorf("open keyring: %w", err)
	}

	return &KeyringStore{ring: ring}, nil
}

func (s *KeyringStore) Keys() ([]string, error) {
	keys, err := s.ring.Keys()
	if err != nil {
		return nil, fmt.Errorf("list keyring keys: %w", err)
	}

	return keys, nil
}

type storedToken struct {
	RefreshToken string    `json:"refresh_token"`
	Services     []string  `json:"services,omitempty"`
	Scopes       []string  `json:"scopes,omitempty"`
	CreatedAt    time.Time `json:"created_at,omitempty"`
}

func (s *KeyringStore) SetToken(email string, tok Token) error {
	email = normalize(email)
	if email == "" {
		return errMissingEmail
	}

	if tok.RefreshToken == "" {
		return errMissingRefreshToken
	}

	if tok.CreatedAt.IsZero() {
		tok.CreatedAt = time.Now().UTC()
	}

	payload, err := json.Marshal(storedToken{
		RefreshToken: tok.RefreshToken,
		Services:     tok.Services,
		Scopes:       tok.Scopes,
		CreatedAt:    tok.CreatedAt,
	})
	if err != nil {
		return fmt.Errorf("encode token: %w", err)
	}

	if err := s.ring.Set(keyring.Item{
		Key:  tokenKey(email),
		Data: payload,
	}); err != nil {
		return wrapKeychainError(fmt.Errorf("store token: %w", err))
	}

	return nil
}

func (s *KeyringStore) GetToken(email string) (Token, error) {
	email = normalize(email)
	if email == "" {
		return Token{}, errMissingEmail
	}

	var it keyring.Item

	if item, err := s.ring.Get(tokenKey(email)); err != nil {
		return Token{}, fmt.Errorf("read token: %w", err)
	} else {
		it = item
	}
	var st storedToken

	if err := json.Unmarshal(it.Data, &st); err != nil {
		return Token{}, fmt.Errorf("decode token: %w", err)
	}

	return Token{
		Email:        email,
		Services:     st.Services,
		Scopes:       st.Scopes,
		CreatedAt:    st.CreatedAt,
		RefreshToken: st.RefreshToken,
	}, nil
}

func (s *KeyringStore) DeleteToken(email string) error {
	email = normalize(email)
	if email == "" {
		return errMissingEmail
	}

	if err := s.ring.Remove(tokenKey(email)); err != nil {
		return fmt.Errorf("delete token: %w", err)
	}

	return nil
}

func (s *KeyringStore) ListTokens() ([]Token, error) {
	keys, err := s.Keys()
	if err != nil {
		return nil, fmt.Errorf("list tokens: %w", err)
	}
	out := make([]Token, 0)

	for _, k := range keys {
		email, ok := ParseTokenKey(k)
		if !ok {
			continue
		}

		var tok Token

		if t, err := s.GetToken(email); err != nil {
			return nil, fmt.Errorf("read token for %s: %w", email, err)
		} else {
			tok = t
		}

		out = append(out, tok)
	}

	return out, nil
}

func ParseTokenKey(k string) (email string, ok bool) {
	const prefix = "token:"
	if !strings.HasPrefix(k, prefix) {
		return "", false
	}
	rest := strings.TrimPrefix(k, prefix)

	if strings.TrimSpace(rest) == "" {
		return "", false
	}

	return rest, true
}

func tokenKey(email string) string {
	return fmt.Sprintf("token:%s", email)
}

func normalize(s string) string {
	return strings.ToLower(strings.TrimSpace(s))
}

const defaultAccountKey = "default_account"

func (s *KeyringStore) GetDefaultAccount() (string, error) {
	it, err := s.ring.Get(defaultAccountKey)
	if err != nil {
		if errors.Is(err, keyring.ErrKeyNotFound) {
			return "", nil
		}

		return "", fmt.Errorf("read default account: %w", err)
	}

	return string(it.Data), nil
}

func (s *KeyringStore) SetDefaultAccount(email string) error {
	email = normalize(email)
	if email == "" {
		return errMissingEmail
	}

	if err := s.ring.Set(keyring.Item{
		Key:  defaultAccountKey,
		Data: []byte(email),
	}); err != nil {
		return fmt.Errorf("store default account: %w", err)
	}

	return nil
}
