package googleapi

import (
	"testing"

	"github.com/openclaw/gogcli/internal/secrets"
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
	ctx := testClientResolverContext(t)
	dependencies, _ := authDependenciesFromContext(ctx)
	dependencies.OpenTokens = func() (secrets.Store, error) {
		return &tasksStubStore{tok: secrets.Token{RefreshToken: "rt"}}, nil
	}
	ctx = WithAuthDependencies(ctx, dependencies)

	svc, err := NewTasks(ctx, "a@b.com")
	if err != nil {
		t.Fatalf("NewTasks: %v", err)
	}

	if svc == nil {
		t.Fatalf("expected service")
	}
}
