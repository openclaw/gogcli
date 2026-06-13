package googleapi

import (
	"testing"

	"github.com/steipete/gogcli/internal/config"
	"github.com/steipete/gogcli/internal/secrets"
)

type tasksStubStore struct {
	tok secrets.Token
	err error
}

func (s *tasksStubStore) Keys() ([]string, error)                      { return nil, nil }
func (s *tasksStubStore) SetToken(string, string, secrets.Token) error { return nil }
func (s *tasksStubStore) DeleteToken(string, string) error             { return nil }
func (s *tasksStubStore) ListTokens() ([]secrets.Token, error)         { return nil, nil }
func (s *tasksStubStore) GetDefaultAccount(string) (string, error)     { return "", nil }
func (s *tasksStubStore) SetDefaultAccount(string, string) error       { return nil }
func (s *tasksStubStore) GetToken(string, string) (secrets.Token, error) {
	if s.err != nil {
		return secrets.Token{}, s.err
	}

	return s.tok, nil
}

func TestNewTasks(t *testing.T) {
	origRead := readClientCredentials
	origOpen := openSecretsStore

	t.Cleanup(func() {
		readClientCredentials = origRead
		openSecretsStore = origOpen
	})

	readClientCredentials = func(string) (config.ClientCredentials, error) {
		return config.ClientCredentials{ClientID: "id", ClientSecret: "secret"}, nil
	}
	openSecretsStore = func() (secrets.Store, error) {
		return &tasksStubStore{tok: secrets.Token{RefreshToken: "rt"}}, nil
	}

	svc, err := NewTasks(testClientResolverContext(), "a@b.com")
	if err != nil {
		t.Fatalf("NewTasks: %v", err)
	}

	if svc == nil {
		t.Fatalf("expected service")
	}
}
