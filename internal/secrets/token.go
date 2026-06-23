package secrets

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/99designs/keyring"

	"github.com/steipete/gogcli/internal/config"
)

type Token struct {
	Client               string    `json:"client,omitempty"`
	Subject              string    `json:"subject,omitempty"`
	Email                string    `json:"email"`
	Services             []string  `json:"services,omitempty"`
	Scopes               []string  `json:"scopes,omitempty"`
	CreatedAt            time.Time `json:"created_at,omitempty"`
	RefreshToken         string    `json:"-"`
	AccessToken          string    `json:"-"`
	AccessTokenExpiresAt time.Time `json:"-"`
}

var (
	errMissingEmail        = errors.New("missing email")
	errMissingRefreshToken = errors.New("missing refresh token")
)

// ErrCorruptStoredToken marks a present OAuth token entry whose JSON payload
// cannot be decoded.
var ErrCorruptStoredToken = errors.New("corrupt stored token")

type storedToken struct {
	RefreshToken         string    `json:"refresh_token"`
	Subject              string    `json:"subject,omitempty"`
	Email                string    `json:"email,omitempty"`
	Services             []string  `json:"services,omitempty"`
	Scopes               []string  `json:"scopes,omitempty"`
	CreatedAt            time.Time `json:"created_at,omitempty"`
	AccessToken          string    `json:"access_token,omitempty"`
	AccessTokenExpiresAt time.Time `json:"access_token_expires_at,omitempty"`
}

func (s *KeyringStore) SetToken(client string, email string, tok Token) error {
	return s.withWriteLock(func() error {
		return s.setTokenNoLock(client, email, tok)
	})
}

func (s *KeyringStore) setTokenNoLock(client string, email string, tok Token) error {
	email = normalize(email)
	if email == "" {
		return errMissingEmail
	}

	if tok.RefreshToken == "" {
		return errMissingRefreshToken
	}

	normalizedClient, err := normalizeClient(client)
	if err != nil {
		return err
	}

	if tok.CreatedAt.IsZero() {
		tok.CreatedAt = time.Now().UTC()
	}
	tok.Subject = strings.TrimSpace(tok.Subject)
	tok.Email = email

	oldSubject := ""
	if existing, getErr := s.getTokenNoLock(normalizedClient, email); getErr == nil {
		oldSubject = strings.TrimSpace(existing.Subject)
	}

	payload, err := json.Marshal(storedToken{ //nolint:gosec // persisted token schema intentionally includes refresh_token
		RefreshToken:         tok.RefreshToken,
		Subject:              tok.Subject,
		Email:                tok.Email,
		Services:             tok.Services,
		Scopes:               tok.Scopes,
		CreatedAt:            tok.CreatedAt,
		AccessToken:          strings.TrimSpace(tok.AccessToken),
		AccessTokenExpiresAt: tok.AccessTokenExpiresAt,
	})
	if err != nil {
		return fmt.Errorf("encode token: %w", err)
	}

	primaryKey := tokenKey(normalizedClient, email)
	if err := verifiedSet(s.ring, primaryKey, payload, "token"); err != nil {
		return wrapKeychainError(fmt.Errorf("store token: %w", err))
	}

	if normalizedClient == config.DefaultClientName {
		if err := verifiedSetAlias(s.ring, legacyTokenKey(email), payload, "legacy token"); err != nil {
			return wrapKeychainError(fmt.Errorf("store legacy token: %w", err))
		}
	}

	if tok.Subject != "" {
		if err := verifiedSetAlias(s.ring, subjectTokenKey(normalizedClient, tok.Subject), payload, "subject token"); err != nil {
			return wrapKeychainError(fmt.Errorf("store subject token: %w", err))
		}
	}

	if oldSubject != "" && oldSubject != tok.Subject {
		if err := s.ring.Remove(subjectTokenKey(normalizedClient, oldSubject)); err != nil && !errors.Is(err, keyring.ErrKeyNotFound) {
			return fmt.Errorf("delete stale subject token: %w", err)
		}
	}

	return nil
}

func (s *KeyringStore) GetToken(client string, email string) (Token, error) {
	var tok Token

	err := s.withWriteLock(func() error {
		var getErr error
		tok, getErr = s.getTokenNoLock(client, email)

		return getErr
	})
	if err != nil {
		return Token{}, err
	}

	return tok, nil
}

// GetTokenNoMigrate reads a token without repairing legacy default-client keys.
func (s *KeyringStore) GetTokenNoMigrate(client string, email string) (Token, error) {
	var tok Token

	err := s.withReadLock(func() error {
		var getErr error
		tok, getErr = s.getTokenNoLockOptions(client, email, false)

		return getErr
	})
	if err != nil {
		return Token{}, err
	}

	return tok, nil
}

func (s *KeyringStore) getTokenNoLock(client string, email string) (Token, error) {
	return s.getTokenNoLockOptions(client, email, true)
}

func (s *KeyringStore) getTokenNoLockOptions(client string, email string, migrateLegacy bool) (Token, error) {
	email = normalize(email)
	if email == "" {
		return Token{}, errMissingEmail
	}

	normalizedClient, err := normalizeClient(client)
	if err != nil {
		return Token{}, err
	}

	primaryKey := tokenKey(normalizedClient, email)

	item, err := s.ring.Get(primaryKey)
	if err != nil {
		if normalizedClient != config.DefaultClientName || !errors.Is(err, keyring.ErrKeyNotFound) {
			return Token{}, fmt.Errorf("read token: %w", err)
		}

		legacyItem, legacyErr := s.ring.Get(legacyTokenKey(email))
		if legacyErr != nil {
			if !errors.Is(legacyErr, keyring.ErrKeyNotFound) {
				return Token{}, fmt.Errorf("read legacy token: %w", legacyErr)
			}

			return Token{}, fmt.Errorf("read token: %w", err)
		}

		item = legacyItem
		if migrateLegacy {
			if migrateErr := verifiedSet(s.ring, primaryKey, legacyItem.Data, "migrated token"); migrateErr != nil {
				return Token{}, wrapKeychainError(fmt.Errorf("migrate token: %w", migrateErr))
			}
		}
	}

	var st storedToken
	if err := json.Unmarshal(item.Data, &st); err != nil {
		return Token{}, fmt.Errorf("%w: decode token: %w", ErrCorruptStoredToken, err)
	}

	return Token{
		Client:               normalizedClient,
		Subject:              strings.TrimSpace(st.Subject),
		Email:                storedEmailOrFallback(st.Email, email),
		Services:             st.Services,
		Scopes:               st.Scopes,
		CreatedAt:            st.CreatedAt,
		RefreshToken:         st.RefreshToken,
		AccessToken:          strings.TrimSpace(st.AccessToken),
		AccessTokenExpiresAt: st.AccessTokenExpiresAt,
	}, nil
}

func (s *KeyringStore) DeleteToken(client string, email string) error {
	return s.withWriteLock(func() error {
		return s.deleteTokenNoLock(client, email)
	})
}

func (s *KeyringStore) deleteTokenNoLock(client string, email string) error {
	email = normalize(email)
	if email == "" {
		return errMissingEmail
	}

	normalizedClient, err := normalizeClient(client)
	if err != nil {
		return err
	}

	var subject string
	if tok, getErr := s.getTokenNoLock(normalizedClient, email); getErr == nil {
		subject = strings.TrimSpace(tok.Subject)
	}

	if err := s.ring.Remove(tokenKey(normalizedClient, email)); err != nil && !errors.Is(err, keyring.ErrKeyNotFound) {
		return fmt.Errorf("delete token: %w", err)
	}

	if normalizedClient == config.DefaultClientName {
		if err := s.ring.Remove(legacyTokenKey(email)); err != nil && !errors.Is(err, keyring.ErrKeyNotFound) {
			return fmt.Errorf("delete legacy token: %w", err)
		}
	}

	if subject != "" {
		if err := s.ring.Remove(subjectTokenKey(normalizedClient, subject)); err != nil && !errors.Is(err, keyring.ErrKeyNotFound) {
			return fmt.Errorf("delete subject token: %w", err)
		}
	}

	return nil
}

// DeleteTokenAlias removes only the email-address key for a token, preserving
// the subject-keyed canonical copy.
func (s *KeyringStore) DeleteTokenAlias(client string, email string) error {
	return s.withWriteLock(func() error {
		return s.deleteTokenAliasNoLock(client, email)
	})
}

func (s *KeyringStore) deleteTokenAliasNoLock(client string, email string) error {
	email = normalize(email)
	if email == "" {
		return errMissingEmail
	}

	normalizedClient, err := normalizeClient(client)
	if err != nil {
		return err
	}

	if err := s.ring.Remove(tokenKey(normalizedClient, email)); err != nil && !errors.Is(err, keyring.ErrKeyNotFound) {
		return fmt.Errorf("delete token alias: %w", err)
	}

	if normalizedClient == config.DefaultClientName {
		if err := s.ring.Remove(legacyTokenKey(email)); err != nil && !errors.Is(err, keyring.ErrKeyNotFound) {
			return fmt.Errorf("delete legacy token alias: %w", err)
		}
	}

	return nil
}

func (s *KeyringStore) ListTokens() ([]Token, error) {
	var tokens []Token

	err := s.withWriteLock(func() error {
		var listErr error

		tokens, listErr = s.listTokensNoLock()

		return listErr
	})
	if err != nil {
		return nil, err
	}

	return tokens, nil
}

func (s *KeyringStore) listTokensNoLock() ([]Token, error) {
	keys, err := s.keysNoLock()
	if err != nil {
		return nil, fmt.Errorf("list tokens: %w", err)
	}
	out := make([]Token, 0)
	seen := make(map[string]struct{})

	for _, k := range keys {
		client, email, ok := ParseTokenKey(k)
		var subject string

		if !ok {
			if parsedClient, parsedSubject, subjectOK := parseSubjectTokenKey(k); subjectOK {
				client = parsedClient
				subject = parsedSubject
			} else {
				continue
			}
		}

		key := client + "\n" + email
		if subject != "" {
			key = client + "\nsub:" + subject
		}

		if _, ok := seen[key]; ok {
			continue
		}

		var tok Token

		if subject != "" {
			t, err := s.getTokenBySubjectNoLock(client, subject)
			if err != nil {
				return nil, fmt.Errorf("read token for subject %s: %w", subject, err)
			}
			tok = t
		} else if t, err := s.getTokenNoLock(client, email); err != nil {
			return nil, fmt.Errorf("read token for %s: %w", email, err)
		} else {
			tok = t
		}

		if tok.Subject != "" {
			key = tok.Client + "\nsub:" + tok.Subject
			if _, ok := seen[key]; ok {
				continue
			}
		}
		seen[key] = struct{}{}

		out = append(out, tok)
	}

	return out, nil
}

func ParseTokenKey(k string) (client string, email string, ok bool) {
	const prefix = "token:"
	if !strings.HasPrefix(k, prefix) {
		return "", "", false
	}
	rest := strings.TrimPrefix(k, prefix)

	if strings.TrimSpace(rest) == "" {
		return "", "", false
	}

	if !strings.Contains(rest, ":") {
		return config.DefaultClientName, rest, true
	}

	parts := strings.SplitN(rest, ":", 2)
	if len(parts) != 2 {
		return "", "", false
	}

	if strings.TrimSpace(parts[0]) == "" || strings.TrimSpace(parts[1]) == "" {
		return "", "", false
	}

	return parts[0], parts[1], true
}

func tokenKey(client string, email string) string {
	return fmt.Sprintf("token:%s:%s", client, email)
}

func subjectTokenKey(client string, subject string) string {
	return fmt.Sprintf("token-sub:%s:%s", client, strings.TrimSpace(subject))
}

func legacyTokenKey(email string) string {
	return fmt.Sprintf("token:%s", email)
}

func TokenKey(client string, email string) string {
	return tokenKey(client, normalize(email))
}

func parseSubjectTokenKey(k string) (client string, subject string, ok bool) {
	const prefix = "token-sub:"
	if !strings.HasPrefix(k, prefix) {
		return "", "", false
	}

	rest := strings.TrimPrefix(k, prefix)
	parts := strings.SplitN(rest, ":", 2)

	if len(parts) != 2 {
		return "", "", false
	}

	if strings.TrimSpace(parts[0]) == "" || strings.TrimSpace(parts[1]) == "" {
		return "", "", false
	}

	return parts[0], parts[1], true
}

func (s *KeyringStore) getTokenBySubjectNoLock(client string, subject string) (Token, error) {
	normalizedClient, err := normalizeClient(client)
	if err != nil {
		return Token{}, err
	}

	subject = strings.TrimSpace(subject)
	if subject == "" {
		return Token{}, errMissingEmail
	}

	item, err := s.ring.Get(subjectTokenKey(normalizedClient, subject))
	if err != nil {
		return Token{}, fmt.Errorf("read token: %w", err)
	}

	var st storedToken
	if err := json.Unmarshal(item.Data, &st); err != nil {
		return Token{}, fmt.Errorf("%w: decode token: %w", ErrCorruptStoredToken, err)
	}

	return Token{
		Client:               normalizedClient,
		Subject:              strings.TrimSpace(st.Subject),
		Email:                storedEmailOrFallback(st.Email, ""),
		Services:             st.Services,
		Scopes:               st.Scopes,
		CreatedAt:            st.CreatedAt,
		RefreshToken:         st.RefreshToken,
		AccessToken:          strings.TrimSpace(st.AccessToken),
		AccessTokenExpiresAt: st.AccessTokenExpiresAt,
	}, nil
}

func storedEmailOrFallback(stored string, fallback string) string {
	if email := normalize(stored); email != "" {
		return email
	}

	return normalize(fallback)
}

func normalize(s string) string {
	return strings.ToLower(strings.TrimSpace(s))
}

func normalizeClient(raw string) (string, error) {
	client, err := config.NormalizeClientNameOrDefault(raw)
	if err != nil {
		return "", fmt.Errorf("normalize client: %w", err)
	}

	return client, nil
}
