package secrets

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"runtime"
	"slices"
	"strings"
	"testing"
	"time"

	onepassword "github.com/1password/onepassword-sdk-go"
	"github.com/99designs/keyring"

	"github.com/steipete/gogcli/internal/config"
)

type fakeOnePasswordItems struct {
	items map[string]onepassword.Item
	next  int
}

func newFakeOnePasswordItems() *fakeOnePasswordItems {
	return &fakeOnePasswordItems{items: make(map[string]onepassword.Item)}
}

func TestOnePasswordItemsClientRetainsSDKClient(t *testing.T) {
	finalized := make(chan struct{}, 1)
	client := &onepassword.Client{}
	runtime.SetFinalizer(client, func(*onepassword.Client) {
		finalized <- struct{}{}
	})

	items := &retainedOnePasswordItemsClient{
		items:  newFakeOnePasswordItems(),
		client: client,
	}
	client = nil

	for range 3 {
		runtime.GC()
		runtime.Gosched()
	}

	select {
	case <-finalized:
		t.Fatal("SDK client finalized while its items client remained reachable")
	default:
	}

	if _, err := items.List(context.Background(), "vault"); err != nil {
		t.Fatalf("List: %v", err)
	}
	runtime.KeepAlive(items)
}

func (f *fakeOnePasswordItems) List(_ context.Context, vaultID string, _ ...onepassword.ItemListFilter) ([]onepassword.ItemOverview, error) {
	out := make([]onepassword.ItemOverview, 0, len(f.items))
	for _, item := range f.items {
		if item.VaultID != vaultID {
			continue
		}

		out = append(out, onepassword.ItemOverview{
			ID:        item.ID,
			Title:     item.Title,
			Category:  item.Category,
			VaultID:   item.VaultID,
			Tags:      item.Tags,
			CreatedAt: item.CreatedAt,
			UpdatedAt: item.UpdatedAt,
			State:     onepassword.ItemStateActive,
		})
	}

	return out, nil
}

func (f *fakeOnePasswordItems) Get(_ context.Context, vaultID string, itemID string) (onepassword.Item, error) {
	item, ok := f.items[itemID]
	if !ok || item.VaultID != vaultID {
		return onepassword.Item{}, keyring.ErrKeyNotFound
	}

	return item, nil
}

func (f *fakeOnePasswordItems) Create(_ context.Context, params onepassword.ItemCreateParams) (onepassword.Item, error) {
	var id string

	for {
		f.next++
		id = fmt.Sprintf("item-%d", f.next)

		if _, exists := f.items[id]; !exists {
			break
		}
	}

	now := time.Unix(int64(f.next), 0).UTC()
	item := onepassword.Item{
		ID:        id,
		Title:     params.Title,
		Category:  params.Category,
		VaultID:   params.VaultID,
		Fields:    slices.Clone(params.Fields),
		Tags:      slices.Clone(params.Tags),
		Version:   1,
		CreatedAt: now,
		UpdatedAt: now,
	}
	f.items[item.ID] = item

	return item, nil
}

func (f *fakeOnePasswordItems) Put(_ context.Context, item onepassword.Item) (onepassword.Item, error) {
	if _, ok := f.items[item.ID]; !ok {
		return onepassword.Item{}, keyring.ErrKeyNotFound
	}

	item.Version++
	item.UpdatedAt = item.UpdatedAt.Add(time.Second)
	f.items[item.ID] = item

	return item, nil
}

func (f *fakeOnePasswordItems) Delete(_ context.Context, vaultID string, itemID string) error {
	item, ok := f.items[itemID]
	if !ok || item.VaultID != vaultID {
		return keyring.ErrKeyNotFound
	}

	delete(f.items, itemID)

	return nil
}

func (f *fakeOnePasswordItems) onlyItem(t *testing.T) onepassword.Item {
	t.Helper()

	if len(f.items) != 1 {
		t.Fatalf("expected one item, got %d", len(f.items))
	}

	for _, item := range f.items {
		return item
	}

	t.Fatal("unreachable")

	return onepassword.Item{}
}

func TestOnePasswordKeyringRoundTrip(t *testing.T) {
	fake := newFakeOnePasswordItems()
	ring := newOnePasswordKeyring(fake, onePasswordConfig{
		vaultID: "vault",
		timeout: time.Second,
	})

	key := "token:default:user@example.com"
	in := keyring.Item{
		Key:         key,
		Data:        []byte{0, 1, 's', 'e', 'c', 'r', 'e', 't'},
		Label:       "token",
		Description: "refresh token",
	}

	if err := ring.Set(in); err != nil {
		t.Fatalf("Set: %v", err)
	}

	stored := fake.onlyItem(t)
	if strings.Contains(stored.Title, key) {
		t.Fatalf("item title should not contain raw key: %q", stored.Title)
	}

	if stored.Title != onePasswordDefaultItemTitle {
		t.Fatalf("unexpected item title: %q", stored.Title)
	}

	if stored.Category != onepassword.ItemCategoryAPICredentials {
		t.Fatalf("unexpected item category: %q", stored.Category)
	}

	if !hasOnePasswordTag(stored.Tags) {
		t.Fatalf("expected gog tag, got %#v", stored.Tags)
	}

	if username, _ := onePasswordField(stored, onePasswordUsernameFieldID); username != key {
		t.Fatalf("unexpected username field: %q", username)
	}

	if credential, _ := onePasswordField(stored, onePasswordCredentialFieldID); credential != base64.StdEncoding.EncodeToString(in.Data) {
		t.Fatalf("unexpected credential field: %q", credential)
	}

	if typeValue, _ := onePasswordField(stored, onePasswordTypeFieldID); typeValue != in.Label {
		t.Fatalf("unexpected type field: %q", typeValue)
	}

	if hostname, _ := onePasswordField(stored, onePasswordHostnameFieldID); hostname != in.Description {
		t.Fatalf("unexpected hostname field: %q", hostname)
	}

	got, err := ring.Get(key)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}

	if got.Key != in.Key || got.Label != in.Label || got.Description != in.Description || !slices.Equal(got.Data, in.Data) {
		t.Fatalf("unexpected item: %#v", got)
	}

	metadata, err := ring.GetMetadata(key)
	if err != nil {
		t.Fatalf("GetMetadata: %v", err)
	}

	if metadata.Item == nil || metadata.Key != key || len(metadata.Data) != 0 {
		t.Fatalf("unexpected metadata: %#v", metadata)
	}

	keys, err := ring.Keys()
	if err != nil {
		t.Fatalf("Keys: %v", err)
	}

	if !slices.Equal(keys, []string{key}) {
		t.Fatalf("unexpected keys: %#v", keys)
	}

	if err = ring.Remove(key); err != nil {
		t.Fatalf("Remove: %v", err)
	}

	if _, err = ring.Get(key); !errors.Is(err, keyring.ErrKeyNotFound) {
		t.Fatalf("expected key not found after remove, got %v", err)
	}
}

func TestOnePasswordKeyringSetUpdatesExistingItem(t *testing.T) {
	fake := newFakeOnePasswordItems()
	ring := newOnePasswordKeyring(fake, onePasswordConfig{
		vaultID: "vault",
		timeout: time.Second,
	})

	key := "default_account:default"
	if err := ring.Set(keyring.Item{Key: key, Data: []byte("old")}); err != nil {
		t.Fatalf("Set old: %v", err)
	}

	if err := ring.Set(keyring.Item{Key: key, Data: []byte("new")}); err != nil {
		t.Fatalf("Set new: %v", err)
	}

	if len(fake.items) != 1 {
		t.Fatalf("expected update in place, got %d items", len(fake.items))
	}

	got, err := ring.Get(key)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}

	if string(got.Data) != "new" {
		t.Fatalf("expected updated value, got %q", string(got.Data))
	}
}

func TestOnePasswordKeyringIgnoresPopulatedAPICredentialItemWithoutTag(t *testing.T) {
	fake := newFakeOnePasswordItems()
	key := "token:default:user@example.com"
	fake.items["item-1"] = onepassword.Item{
		ID:       "item-1",
		Title:    onePasswordDefaultItemTitle,
		Category: onepassword.ItemCategoryAPICredentials,
		VaultID:  "vault",
		Fields: []onepassword.ItemField{
			{
				ID:        onePasswordUsernameFieldID,
				Title:     "username",
				Value:     key,
				FieldType: onepassword.ItemFieldTypeText,
			},
			{
				ID:        onePasswordCredentialFieldID,
				Title:     "credential",
				Value:     base64.StdEncoding.EncodeToString([]byte("secret")),
				FieldType: onepassword.ItemFieldTypeConcealed,
			},
			{
				ID:        onePasswordTypeFieldID,
				Title:     "type",
				Value:     "token",
				FieldType: onepassword.ItemFieldTypeMenu,
			},
			{
				ID:        onePasswordHostnameFieldID,
				Title:     "hostname",
				Value:     "google",
				FieldType: onepassword.ItemFieldTypeText,
			},
		},
	}
	ring := newOnePasswordKeyring(fake, onePasswordConfig{
		vaultID: "vault",
		timeout: time.Second,
	})

	_, err := ring.Get(key)
	if !errors.Is(err, keyring.ErrKeyNotFound) {
		t.Fatalf("expected untagged item to be ignored, got %v", err)
	}

	if err = ring.Set(keyring.Item{Key: key, Data: []byte("updated"), Label: "token"}); err != nil {
		t.Fatalf("Set: %v", err)
	}

	if len(fake.items) != 2 {
		t.Fatalf("expected new managed item, got %d items", len(fake.items))
	}

	original := fake.items["item-1"]
	if hasOnePasswordTag(original.Tags) {
		t.Fatalf("untagged item was adopted: %#v", original.Tags)
	}

	if credential, _ := onePasswordField(original, onePasswordCredentialFieldID); credential != base64.StdEncoding.EncodeToString([]byte("secret")) {
		t.Fatalf("untagged item was mutated: %q", credential)
	}
}

func TestOnePasswordKeyringSetReusesEmptyAPICredentialTemplateItem(t *testing.T) {
	fake := newFakeOnePasswordItems()
	fake.items["item-1"] = onepassword.Item{
		ID:       "item-1",
		Title:    onePasswordDefaultItemTitle,
		Category: onepassword.ItemCategoryAPICredentials,
		VaultID:  "vault",
		Fields: []onepassword.ItemField{
			{
				ID:        onePasswordUsernameFieldID,
				Title:     "username",
				FieldType: onepassword.ItemFieldTypeText,
			},
			{
				ID:        onePasswordCredentialFieldID,
				Title:     "credential",
				FieldType: onepassword.ItemFieldTypeConcealed,
			},
			{
				ID:        onePasswordTypeFieldID,
				Title:     "type",
				FieldType: onepassword.ItemFieldTypeMenu,
			},
			{
				ID:        onePasswordFilenameFieldID,
				Title:     "filename",
				FieldType: onepassword.ItemFieldTypeText,
			},
			{
				ID:        onePasswordValidFromFieldID,
				Title:     "valid from",
				FieldType: onepassword.ItemFieldTypeDate,
			},
			{
				ID:        onePasswordExpiresFieldID,
				Title:     "expires",
				FieldType: onepassword.ItemFieldTypeDate,
			},
			{
				ID:        onePasswordHostnameFieldID,
				Title:     "hostname",
				FieldType: onepassword.ItemFieldTypeText,
			},
		},
	}
	ring := newOnePasswordKeyring(fake, onePasswordConfig{
		vaultID: "vault",
		timeout: time.Second,
	})

	key := "token:default:user@example.com"
	if err := ring.Set(keyring.Item{Key: key, Data: []byte("secret"), Label: "token"}); err != nil {
		t.Fatalf("Set: %v", err)
	}

	if len(fake.items) != 1 {
		t.Fatalf("expected template item to be reused, got %d items", len(fake.items))
	}

	updated := fake.items["item-1"]
	if username, _ := onePasswordField(updated, onePasswordUsernameFieldID); username != key {
		t.Fatalf("unexpected username field: %q", username)
	}

	if !hasOnePasswordTag(updated.Tags) {
		t.Fatalf("expected reused template item to be tagged, got %#v", updated.Tags)
	}
}

func TestOnePasswordKeyringSetDoesNotReuseTaggedSecureNoteTemplate(t *testing.T) {
	fake := newFakeOnePasswordItems()
	fake.items["item-1"] = onepassword.Item{
		ID:       "item-1",
		Title:    onePasswordDefaultItemTitle,
		Category: onepassword.ItemCategorySecureNote,
		VaultID:  "vault",
		Tags:     []string{onePasswordTag},
	}
	ring := newOnePasswordKeyring(fake, onePasswordConfig{
		vaultID: "vault",
		timeout: time.Second,
	})

	key := "token:default:user@example.com"
	if err := ring.Set(keyring.Item{Key: key, Data: []byte("secret"), Label: "token"}); err != nil {
		t.Fatalf("Set: %v", err)
	}

	if len(fake.items) != 2 {
		t.Fatalf("expected secure note to be preserved and new API credential to be created, got %d items", len(fake.items))
	}

	if fake.items["item-1"].Category != onepassword.ItemCategorySecureNote {
		t.Fatalf("secure note template was unexpectedly mutated: %#v", fake.items["item-1"])
	}
}

func TestOnePasswordKeyringRespectsConfiguredItemTitle(t *testing.T) {
	fake := newFakeOnePasswordItems()
	key := "token:default:user@example.com"
	fake.items["item-1"] = onepassword.Item{
		ID:       "item-1",
		Title:    "other-title",
		Category: onepassword.ItemCategoryAPICredentials,
		VaultID:  "vault",
		Tags:     []string{onePasswordTag},
		Fields: []onepassword.ItemField{
			{
				ID:        onePasswordUsernameFieldID,
				Title:     "username",
				Value:     key,
				FieldType: onepassword.ItemFieldTypeText,
			},
			{
				ID:        onePasswordCredentialFieldID,
				Title:     "credential",
				Value:     base64.StdEncoding.EncodeToString([]byte("secret")),
				FieldType: onepassword.ItemFieldTypeConcealed,
			},
		},
	}
	ring := newOnePasswordKeyring(fake, onePasswordConfig{
		vaultID: "vault",
		timeout: time.Second,
	})

	_, err := ring.Get(key)
	if !errors.Is(err, keyring.ErrKeyNotFound) {
		t.Fatalf("expected configured title to isolate lookups, got %v", err)
	}

	keys, err := ring.Keys()
	if err != nil {
		t.Fatalf("Keys: %v", err)
	}

	if len(keys) != 0 {
		t.Fatalf("expected configured title to isolate keys, got %#v", keys)
	}
}

func TestOnePasswordConfigFromEnv(t *testing.T) {
	t.Setenv(OnePasswordServiceAccountEnv, "")
	t.Setenv(OnePasswordAccountEnv, "")
	t.Setenv(OnePasswordAuthEnv, "")
	t.Setenv(OnePasswordVaultEnv, "")
	t.Setenv(OnePasswordItemTitleEnv, "")
	t.Setenv(OnePasswordOperationTimeoutEnv, "")

	if _, err := onePasswordConfigFromEnv(nil, config.AppName); !errors.Is(err, errMissingOnePasswordToken) {
		t.Fatalf("expected missing token, got %v", err)
	}

	t.Setenv(OnePasswordServiceAccountEnv, "token")

	if _, err := onePasswordConfigFromEnv(nil, config.AppName); !errors.Is(err, errMissingOnePasswordVault) {
		t.Fatalf("expected missing vault, got %v", err)
	}

	t.Setenv(OnePasswordVaultEnv, "vault")
	t.Setenv(OnePasswordItemTitleEnv, " custom ")
	t.Setenv(OnePasswordOperationTimeoutEnv, "250ms")

	cfg, err := onePasswordConfigFromEnv(nil, config.AppName)
	if err != nil {
		t.Fatalf("onePasswordConfigFromEnv: %v", err)
	}

	if cfg.serviceAccountToken != "token" || cfg.vaultID != "vault" || cfg.itemTitle != "custom" || cfg.timeout != 250*time.Millisecond {
		t.Fatalf("unexpected config: %#v", cfg)
	}

	if cfg.authMode != onePasswordAuthServiceAccount {
		t.Fatalf("expected service-account auth, got %q", cfg.authMode)
	}
}

func TestOnePasswordConfigFromEnv_DesktopAuth(t *testing.T) {
	t.Setenv(OnePasswordServiceAccountEnv, "")
	t.Setenv(OnePasswordAccountEnv, " example-account ")
	t.Setenv(OnePasswordAuthEnv, "")
	t.Setenv(OnePasswordVaultEnv, "vault")
	t.Setenv(OnePasswordOperationTimeoutEnv, "")

	cfg, err := onePasswordConfigFromEnv(nil, config.AppName)
	if err != nil {
		t.Fatalf("onePasswordConfigFromEnv: %v", err)
	}

	if cfg.authMode != onePasswordAuthDesktop || cfg.accountName != "example-account" {
		t.Fatalf("unexpected desktop config: %#v", cfg)
	}
}

func TestOnePasswordConfigFromEnv_ExplicitDesktopRequiresAccount(t *testing.T) {
	t.Setenv(OnePasswordServiceAccountEnv, "token")
	t.Setenv(OnePasswordAccountEnv, "")
	t.Setenv(OnePasswordAuthEnv, "desktop")
	t.Setenv(OnePasswordVaultEnv, "vault")
	t.Setenv(OnePasswordOperationTimeoutEnv, "")

	_, err := onePasswordConfigFromEnv(nil, config.AppName)
	if !errors.Is(err, errMissingOnePasswordAccount) {
		t.Fatalf("expected missing account, got %v", err)
	}
}

func TestOnePasswordConfigFromConfigAndEnvOverride(t *testing.T) {
	tests := []struct {
		name string
		env  map[string]string
		file config.File
		want onePasswordConfig
	}{
		{
			name: "config",
			file: config.File{
				OnePasswordAuth:      "desktop",
				OnePasswordAccount:   "config-account",
				OnePasswordVault:     "config-vault",
				OnePasswordItemTitle: "config-title",
				OnePasswordTimeout:   "750ms",
			},
			want: onePasswordConfig{
				authMode:    onePasswordAuthDesktop,
				accountName: "config-account",
				vaultID:     "config-vault",
				itemTitle:   "config-title",
				timeout:     750 * time.Millisecond,
			},
		},
		{
			name: "env override",
			env: map[string]string{
				OnePasswordAccountEnv:          "env-account",
				OnePasswordAuthEnv:             "desktop",
				OnePasswordVaultEnv:            "env-vault",
				OnePasswordItemTitleEnv:        "env-title",
				OnePasswordOperationTimeoutEnv: "250ms",
			},
			file: config.File{
				OnePasswordAuth:      "service-account",
				OnePasswordAccount:   "config-account",
				OnePasswordVault:     "config-vault",
				OnePasswordItemTitle: "config-title",
				OnePasswordTimeout:   "750ms",
			},
			want: onePasswordConfig{
				authMode:    onePasswordAuthDesktop,
				accountName: "env-account",
				vaultID:     "env-vault",
				itemTitle:   "env-title",
				timeout:     250 * time.Millisecond,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv(OnePasswordServiceAccountEnv, "")
			t.Setenv(OnePasswordAccountEnv, "")
			t.Setenv(OnePasswordAuthEnv, "")
			t.Setenv(OnePasswordVaultEnv, "")
			t.Setenv(OnePasswordItemTitleEnv, "")
			t.Setenv(OnePasswordOperationTimeoutEnv, "")

			for key, value := range tt.env {
				t.Setenv(key, value)
			}

			store := config.NewConfigStore(config.Layout{ConfigDir: t.TempDir()})
			if err := store.Write(tt.file); err != nil {
				t.Fatalf("write config: %v", err)
			}

			cfg, err := onePasswordConfigFromEnv(store, config.AppName)
			if err != nil {
				t.Fatalf("onePasswordConfigFromEnv: %v", err)
			}

			if cfg.authMode != tt.want.authMode || cfg.accountName != tt.want.accountName ||
				cfg.vaultID != tt.want.vaultID || cfg.itemTitle != tt.want.itemTitle || cfg.timeout != tt.want.timeout {
				t.Fatalf("unexpected config: %#v", cfg)
			}
		})
	}
}

func TestResolveOnePasswordAuthModeAliases(t *testing.T) {
	tests := []struct {
		raw     string
		account string
		want    string
	}{
		{raw: "", want: onePasswordAuthServiceAccount},
		{raw: "", account: "me", want: onePasswordAuthDesktop},
		{raw: "auto", want: onePasswordAuthServiceAccount},
		{raw: "local", want: onePasswordAuthDesktop},
		{raw: "desktop-app", want: onePasswordAuthDesktop},
		{raw: "service_account", want: onePasswordAuthServiceAccount},
		{raw: "sa", want: onePasswordAuthServiceAccount},
	}

	for _, tt := range tests {
		t.Run(tt.raw+"/"+tt.account, func(t *testing.T) {
			got, err := resolveOnePasswordAuthMode(tt.raw, tt.account)
			if err != nil {
				t.Fatalf("resolveOnePasswordAuthMode: %v", err)
			}

			if got != tt.want {
				t.Fatalf("got %q, want %q", got, tt.want)
			}
		})
	}
}

func TestOpenKeyringUsesOnePasswordBackend(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", home)
	t.Setenv(keyringBackendEnv, KeyringBackendOnePassword)
	t.Setenv(keyringServiceNameEnv, "custom-gog")
	t.Setenv(OnePasswordServiceAccountEnv, "token")
	t.Setenv(OnePasswordVaultEnv, "vault")

	origNewClient := newOnePasswordItemsClient

	t.Cleanup(func() { newOnePasswordItemsClient = origNewClient })

	fake := newFakeOnePasswordItems()
	newOnePasswordItemsClient = func(_ context.Context, cfg onePasswordConfig) (onePasswordItemsClient, error) {
		if cfg.serviceAccountToken != "token" {
			t.Fatalf("unexpected token: %q", cfg.serviceAccountToken)
		}

		if cfg.authMode != onePasswordAuthServiceAccount {
			t.Fatalf("unexpected auth mode: %q", cfg.authMode)
		}

		if cfg.itemTitle != "custom-gog-keyring" {
			t.Fatalf("unexpected item title: %q", cfg.itemTitle)
		}

		return fake, nil
	}

	layout := testSystemLayout(t, config.PathKindConfig, config.PathKindData)
	store := config.NewConfigStore(layout)

	ring, err := openKeyringWithOptions(systemTestOpenOptions(layout, store))
	if err != nil {
		t.Fatalf("openKeyring: %v", err)
	}

	if _, ok := ring.(*onePasswordKeyring); !ok {
		t.Fatalf("expected onePasswordKeyring, got %T", ring)
	}
}
