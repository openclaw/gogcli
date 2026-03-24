package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
)

var (
	errInvalidCredentials = errors.New("invalid credentials.json (expected installed/web client_id and client_secret)")
	errMissingClientID    = errors.New("stored credentials.json is missing client_id/client_secret")
)

type ClientCredentials struct {
	ClientID     string `json:"client_id"`
	ClientSecret string `json:"client_secret"`
}

type googleCredentialsFile struct {
	Installed *struct {
		ClientID     string `json:"client_id"`
		ClientSecret string `json:"client_secret"`
	} `json:"installed"`
	Web *struct {
		ClientID     string `json:"client_id"`
		ClientSecret string `json:"client_secret"`
	} `json:"web"`
}

func ParseGoogleOAuthClientJSON(b []byte) (ClientCredentials, error) {
	var f googleCredentialsFile
	if err := json.Unmarshal(b, &f); err != nil {
		return ClientCredentials{}, fmt.Errorf("decode credentials json: %w", err)
	}

	var clientID, clientSecret string
	if f.Installed != nil {
		clientID, clientSecret = f.Installed.ClientID, f.Installed.ClientSecret
	} else if f.Web != nil {
		clientID, clientSecret = f.Web.ClientID, f.Web.ClientSecret
	}

	if clientID == "" || clientSecret == "" {
		return ClientCredentials{}, errInvalidCredentials
	}

	return ClientCredentials{ClientID: clientID, ClientSecret: clientSecret}, nil
}

func WriteClientCredentials(c ClientCredentials) error {
	return WriteClientCredentialsFor(DefaultClientName, c)
}

func WriteClientCredentialsFor(client string, c ClientCredentials) error {
	_, err := EnsureDir()
	if err != nil {
		return fmt.Errorf("ensure config dir: %w", err)
	}

	path, err := ClientCredentialsPathFor(client)
	if err != nil {
		return fmt.Errorf("resolve credentials path: %w", err)
	}

	b, err := json.MarshalIndent(c, "", "  ") //nolint:gosec // required OAuth client credentials payload
	if err != nil {
		return fmt.Errorf("encode credentials json: %w", err)
	}

	b = append(b, '\n')

	tmp := path + ".tmp"

	if err := os.WriteFile(tmp, b, 0o600); err != nil {
		return fmt.Errorf("write credentials: %w", err)
	}

	if err := os.Rename(tmp, path); err != nil {
		return fmt.Errorf("commit credentials: %w", err)
	}

	return nil
}

func ReadClientCredentials() (ClientCredentials, error) {
	return ReadClientCredentialsFor(DefaultClientName)
}

func ReadClientCredentialsFor(client string) (ClientCredentials, error) {
	path, err := ClientCredentialsPathFor(client)
	if err != nil {
		return ClientCredentials{}, fmt.Errorf("resolve credentials path: %w", err)
	}
	var b []byte

	if b, err = os.ReadFile(path); err != nil { //nolint:gosec // user-provided path
		if os.IsNotExist(err) {
			return ClientCredentials{}, &CredentialsMissingError{Path: path, Cause: err}
		}

		return ClientCredentials{}, fmt.Errorf("read credentials: %w", err)
	}

	var c ClientCredentials
	if err := json.Unmarshal(b, &c); err != nil {
		return ClientCredentials{}, fmt.Errorf("decode credentials: %w", err)
	}

	if c.ClientID == "" || c.ClientSecret == "" {
		return ClientCredentials{}, errMissingClientID
	}

	return c, nil
}

func DeleteClientCredentialsFor(client string) error {
	path, err := ClientCredentialsPathFor(client)
	if err != nil {
		return fmt.Errorf("resolve credentials path: %w", err)
	}

	if err := os.Remove(path); err != nil {
		if os.IsNotExist(err) {
			return &CredentialsMissingError{Path: path, Cause: err}
		}

		return fmt.Errorf("delete credentials: %w", err)
	}

	return nil
}

func ClientCredentialsExists(client string) (bool, error) {
	path, err := ClientCredentialsPathFor(client)
	if err != nil {
		return false, err
	}

	if _, err := os.Stat(path); err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}

		return false, fmt.Errorf("stat credentials: %w", err)
	}

	return true, nil
}

type CredentialsMissingError struct {
	Path  string
	Cause error
}

func (e *CredentialsMissingError) Error() string {
	return "oauth credentials missing"
}

func (e *CredentialsMissingError) Unwrap() error {
	return e.Cause
}
