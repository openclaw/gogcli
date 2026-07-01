package secrets

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"os"
	"sort"
	"strings"
	"time"

	onepassword "github.com/1password/onepassword-sdk-go"
	"github.com/99designs/keyring"

	"github.com/steipete/gogcli/internal/config"
)

const (
	onePasswordTag               = "gogcli-keyring" //nolint:gosec // 1Password item tag, not a credential
	onePasswordUsernameFieldID   = "username"
	onePasswordCredentialFieldID = "credential"
	onePasswordTypeFieldID       = "type"
	onePasswordFilenameFieldID   = "filename"
	onePasswordValidFromFieldID  = "validFrom"
	onePasswordExpiresFieldID    = "expires"
	onePasswordHostnameFieldID   = "hostname"
)

const (
	onePasswordAuthAuto           = "auto"
	onePasswordAuthDesktop        = "desktop"
	onePasswordAuthServiceAccount = "service-account"
)

var (
	errMissingOnePasswordToken   = errors.New("missing 1Password service account token")
	errMissingOnePasswordAccount = errors.New("missing 1Password account")
	errMissingOnePasswordVault   = errors.New("missing 1Password vault")
	errInvalidOnePasswordAuth    = errors.New("invalid 1Password auth mode")
	errInvalidOnePasswordTimeout = errors.New("invalid 1Password timeout")
	errMissingOnePasswordValue   = errors.New("missing 1Password keyring value")
)

// The github.com/99designs/keyring interface has no parent context parameter.
// Every SDK call derived from this root is still bounded by the configured
// operation timeout.
var onePasswordRootContext = context.Background()

type onePasswordItemsClient interface {
	List(context.Context, string, ...onepassword.ItemListFilter) ([]onepassword.ItemOverview, error)
	Get(context.Context, string, string) (onepassword.Item, error)
	Create(context.Context, onepassword.ItemCreateParams) (onepassword.Item, error)
	Put(context.Context, onepassword.Item) (onepassword.Item, error)
	Delete(context.Context, string, string) error
}

type onePasswordKeyring struct {
	items   onePasswordItemsClient
	vaultID string
	title   string
	timeout time.Duration
}

type onePasswordConfig struct {
	authMode            string
	serviceAccountToken string
	accountName         string
	vaultID             string
	itemTitle           string
	timeout             time.Duration
}

var newOnePasswordItemsClient = func(ctx context.Context, cfg onePasswordConfig) (onePasswordItemsClient, error) {
	opts := []onepassword.ClientOption{
		onepassword.WithIntegrationInfo("gogcli", "keyring"),
	}

	switch cfg.authMode {
	case onePasswordAuthDesktop:
		opts = append(opts, onepassword.WithDesktopAppIntegration(cfg.accountName))
	case onePasswordAuthServiceAccount:
		opts = append(opts, onepassword.WithServiceAccountToken(cfg.serviceAccountToken))
	default:
		return nil, fmt.Errorf("%w: %q", errInvalidOnePasswordAuth, cfg.authMode)
	}

	client, err := onepassword.NewClient(ctx, opts...)
	if err != nil {
		return nil, fmt.Errorf("new 1Password client: %w", err)
	}

	return client.Items(), nil
}

func openOnePasswordKeyring(store *config.ConfigStore, serviceName string) (keyring.Keyring, error) {
	cfg, err := onePasswordConfigFromEnv(store, serviceName)
	if err != nil {
		return nil, err
	}

	ctx, cancel := context.WithTimeout(onePasswordRootContext, cfg.timeout)
	defer cancel()

	items, err := newOnePasswordItemsClient(ctx, cfg)
	if err != nil {
		return nil, fmt.Errorf("open 1Password keyring: %w", err)
	}

	return newOnePasswordKeyring(items, cfg), nil
}

func onePasswordConfigFromEnv(store *config.ConfigStore, serviceName string) (onePasswordConfig, error) {
	fileCfg, err := readOnePasswordConfig(store)
	if err != nil && !onePasswordEnvHasRequiredValues() {
		return onePasswordConfig{}, err
	}

	if err != nil {
		fileCfg = config.File{}
	}

	token := strings.TrimSpace(os.Getenv(OnePasswordServiceAccountEnv))
	accountName := onePasswordConfigValue(OnePasswordAccountEnv, fileCfg.OnePasswordAccount)

	authMode, err := resolveOnePasswordAuthMode(onePasswordConfigValue(OnePasswordAuthEnv, fileCfg.OnePasswordAuth), accountName)
	if err != nil {
		return onePasswordConfig{}, err
	}

	switch authMode {
	case onePasswordAuthDesktop:
		if accountName == "" {
			return onePasswordConfig{}, fmt.Errorf("%w: set %s or config onepassword_account to your 1Password account name or UUID", errMissingOnePasswordAccount, OnePasswordAccountEnv)
		}
	case onePasswordAuthServiceAccount:
		if token == "" {
			return onePasswordConfig{}, fmt.Errorf("%w: set %s", errMissingOnePasswordToken, OnePasswordServiceAccountEnv)
		}
	default:
		return onePasswordConfig{}, fmt.Errorf("%w: %q", errInvalidOnePasswordAuth, authMode)
	}

	vaultID := onePasswordConfigValue(OnePasswordVaultEnv, fileCfg.OnePasswordVault)
	if vaultID == "" {
		return onePasswordConfig{}, fmt.Errorf("%w: set %s or config onepassword_vault to a vault ID", errMissingOnePasswordVault, OnePasswordVaultEnv)
	}

	timeout, err := onePasswordOperationTimeoutFromRaw(onePasswordConfigValue(OnePasswordOperationTimeoutEnv, fileCfg.OnePasswordTimeout))
	if err != nil {
		return onePasswordConfig{}, err
	}

	itemTitle := onePasswordConfigValue(OnePasswordItemTitleEnv, fileCfg.OnePasswordItemTitle)
	if itemTitle == "" {
		itemTitle = onePasswordDefaultItemTitleForService(serviceName)
	}

	return onePasswordConfig{
		authMode:            authMode,
		serviceAccountToken: token,
		accountName:         accountName,
		vaultID:             vaultID,
		itemTitle:           itemTitle,
		timeout:             timeout,
	}, nil
}

func onePasswordDefaultItemTitleForService(serviceName string) string {
	serviceName = strings.TrimSpace(serviceName)
	if serviceName == "" || serviceName == config.AppName {
		return onePasswordDefaultItemTitle
	}

	return serviceName + "-keyring"
}

func readOnePasswordConfig(store *config.ConfigStore) (config.File, error) {
	if store == nil {
		return config.File{}, nil
	}

	cfg, err := store.Read()
	if err != nil {
		return config.File{}, fmt.Errorf("read 1Password config: %w", err)
	}

	return cfg, nil
}

func onePasswordConfigValue(envName string, configValue string) string {
	if value := strings.TrimSpace(os.Getenv(envName)); value != "" {
		return value
	}

	return strings.TrimSpace(configValue)
}

func onePasswordEnvHasRequiredValues() bool {
	vaultID := strings.TrimSpace(os.Getenv(OnePasswordVaultEnv))
	accountName := strings.TrimSpace(os.Getenv(OnePasswordAccountEnv))
	token := strings.TrimSpace(os.Getenv(OnePasswordServiceAccountEnv))
	authMode := NormalizeOnePasswordAuthMode(os.Getenv(OnePasswordAuthEnv))

	if vaultID == "" {
		return false
	}

	switch authMode {
	case "", onePasswordAuthAuto:
		return accountName != "" || token != ""
	case onePasswordAuthDesktop:
		return accountName != ""
	case onePasswordAuthServiceAccount:
		return token != ""
	default:
		return false
	}
}

func resolveOnePasswordAuthMode(raw string, accountName string) (string, error) {
	switch NormalizeOnePasswordAuthMode(raw) {
	case "", onePasswordAuthAuto:
		if strings.TrimSpace(accountName) != "" {
			return onePasswordAuthDesktop, nil
		}

		return onePasswordAuthServiceAccount, nil
	case onePasswordAuthDesktop:
		return onePasswordAuthDesktop, nil
	case onePasswordAuthServiceAccount:
		return onePasswordAuthServiceAccount, nil
	default:
		return "", fmt.Errorf("%w: %q (expected auto, desktop, or service-account)", errInvalidOnePasswordAuth, raw)
	}
}

// NormalizeOnePasswordAuthMode canonicalizes accepted 1Password auth mode aliases.
func NormalizeOnePasswordAuthMode(raw string) string {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "service_account", "serviceaccount", "sa":
		return onePasswordAuthServiceAccount
	case "app", "local", "desktop-app", "desktop_app":
		return onePasswordAuthDesktop
	default:
		return strings.ToLower(strings.TrimSpace(raw))
	}
}

func onePasswordOperationTimeoutFromRaw(raw string) (time.Duration, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return keyringOpenTimeout, nil
	}

	timeout, err := time.ParseDuration(raw)
	if err != nil || timeout <= 0 {
		return 0, fmt.Errorf("%w: 1Password timeout must be a positive duration such as 10s", errInvalidOnePasswordTimeout)
	}

	return timeout, nil
}

func newOnePasswordKeyring(items onePasswordItemsClient, cfg onePasswordConfig) keyring.Keyring {
	timeout := cfg.timeout
	if timeout <= 0 {
		timeout = keyringOpenTimeout
	}

	itemTitle := strings.TrimSpace(cfg.itemTitle)
	if itemTitle == "" {
		itemTitle = onePasswordDefaultItemTitle
	}

	return &onePasswordKeyring{
		items:   items,
		vaultID: strings.TrimSpace(cfg.vaultID),
		title:   itemTitle,
		timeout: timeout,
	}
}

func (k *onePasswordKeyring) Get(key string) (keyring.Item, error) {
	item, err := k.findItem(key)
	if err != nil {
		return keyring.Item{}, err
	}

	return keyringItemFromOnePassword(item)
}

func (k *onePasswordKeyring) GetMetadata(key string) (keyring.Metadata, error) {
	item, err := k.findItem(key)
	if err != nil {
		return keyring.Metadata{}, err
	}

	out, err := keyringItemFromOnePassword(item)
	if err != nil {
		return keyring.Metadata{}, err
	}
	out.Data = nil

	modified := item.UpdatedAt
	if modified.IsZero() {
		modified = item.CreatedAt
	}

	return keyring.Metadata{Item: &out, ModificationTime: modified}, nil
}

func (k *onePasswordKeyring) Set(item keyring.Item) error {
	if strings.TrimSpace(item.Key) == "" {
		return errMissingSecretKey
	}

	existing, err := k.findItem(item.Key)
	if err != nil && !errors.Is(err, keyring.ErrKeyNotFound) {
		return err
	}

	if errors.Is(err, keyring.ErrKeyNotFound) {
		template, templateErr := k.findEmptyItem()
		if templateErr != nil && !errors.Is(templateErr, keyring.ErrKeyNotFound) {
			return templateErr
		}

		if templateErr == nil {
			existing = template
			err = nil
		}
	}

	ctx, cancel := k.context()
	defer cancel()

	if errors.Is(err, keyring.ErrKeyNotFound) {
		_, err = k.items.Create(ctx, onepassword.ItemCreateParams{
			Category: onepassword.ItemCategoryAPICredentials,
			VaultID:  k.vaultID,
			Title:    k.itemTitle(),
			Fields:   onePasswordFields(item),
			Tags:     []string{onePasswordTag},
		})
		if err != nil {
			return fmt.Errorf("create 1Password keyring item: %w", err)
		}

		return nil
	}

	existing.Title = k.itemTitle()
	existing.Category = onepassword.ItemCategoryAPICredentials
	existing.VaultID = k.vaultID
	existing.Fields = onePasswordFields(item)
	existing.Tags = appendOnePasswordTag(existing.Tags)

	if _, err = k.items.Put(ctx, existing); err != nil {
		return fmt.Errorf("update 1Password keyring item: %w", err)
	}

	return nil
}

func (k *onePasswordKeyring) SetTrusted(item keyring.Item) error {
	return k.Set(item)
}

func (k *onePasswordKeyring) Remove(key string) error {
	item, err := k.findItem(key)
	if err != nil {
		return err
	}

	ctx, cancel := k.context()
	defer cancel()

	if err = k.items.Delete(ctx, k.vaultID, item.ID); err != nil {
		if isOnePasswordNotFound(err) {
			return keyring.ErrKeyNotFound
		}

		return fmt.Errorf("delete 1Password keyring item: %w", err)
	}

	return nil
}

func (k *onePasswordKeyring) Keys() ([]string, error) {
	overviews, err := k.listManagedItems()
	if err != nil {
		return nil, err
	}

	keys := make([]string, 0, len(overviews))
	for _, overview := range overviews {
		item, getErr := k.getItem(overview.ID)
		if getErr != nil {
			if isOnePasswordNotFound(getErr) {
				continue
			}

			return nil, fmt.Errorf("read 1Password keyring item: %w", getErr)
		}

		key, ok := onePasswordKeyField(item)
		if !ok || strings.TrimSpace(key) == "" {
			continue
		}
		keys = append(keys, key)
	}

	sort.Strings(keys)

	return keys, nil
}

func (k *onePasswordKeyring) findItem(key string) (onepassword.Item, error) {
	key = strings.TrimSpace(key)
	if key == "" {
		return onepassword.Item{}, errMissingSecretKey
	}

	overviews, err := k.listManagedItems()
	if err != nil {
		return onepassword.Item{}, err
	}

	for _, overview := range overviews {
		item, err := k.getItemIfKeyMatches(overview.ID, key)
		if err != nil && !errors.Is(err, keyring.ErrKeyNotFound) {
			return onepassword.Item{}, err
		}

		if err == nil {
			return item, nil
		}
	}

	return onepassword.Item{}, keyring.ErrKeyNotFound
}

func (k *onePasswordKeyring) findEmptyItem() (onepassword.Item, error) {
	overviews, err := k.listActiveItems()
	if err != nil {
		return onepassword.Item{}, err
	}

	for _, overview := range overviews {
		if overview.Title != k.itemTitle() {
			continue
		}

		item, err := k.getItem(overview.ID)
		if err != nil {
			if isOnePasswordNotFound(err) {
				continue
			}

			return onepassword.Item{}, fmt.Errorf("read 1Password keyring item: %w", err)
		}

		if !isReusableOnePasswordTemplateItem(item, k.itemTitle()) {
			continue
		}

		return item, nil
	}

	return onepassword.Item{}, keyring.ErrKeyNotFound
}

func (k *onePasswordKeyring) getItemIfKeyMatches(itemID string, key string) (onepassword.Item, error) {
	item, err := k.getItem(itemID)
	if err != nil {
		if isOnePasswordNotFound(err) {
			return onepassword.Item{}, keyring.ErrKeyNotFound
		}

		return onepassword.Item{}, fmt.Errorf("read 1Password keyring item: %w", err)
	}

	storedKey, ok := onePasswordKeyField(item)
	if !ok || storedKey != key {
		return onepassword.Item{}, keyring.ErrKeyNotFound
	}

	return item, nil
}

func (k *onePasswordKeyring) getItem(itemID string) (onepassword.Item, error) {
	ctx, cancel := k.context()
	defer cancel()

	item, err := k.items.Get(ctx, k.vaultID, itemID)
	if err != nil {
		return onepassword.Item{}, fmt.Errorf("get 1Password keyring item: %w", err)
	}

	return item, nil
}

func (k *onePasswordKeyring) listManagedItems() ([]onepassword.ItemOverview, error) {
	overviews, err := k.listActiveItems()
	if err != nil {
		return nil, err
	}

	tagged := make([]onepassword.ItemOverview, 0, len(overviews))
	for _, overview := range overviews {
		if isOnePasswordKeyringItem(overview, k.itemTitle()) {
			tagged = append(tagged, overview)
		}
	}

	return tagged, nil
}

func (k *onePasswordKeyring) listActiveItems() ([]onepassword.ItemOverview, error) {
	ctx, cancel := k.context()
	defer cancel()

	overviews, err := k.items.List(ctx, k.vaultID, onepassword.NewItemListFilterTypeVariantByState(&onepassword.ItemListFilterByStateInner{
		Active:   true,
		Archived: false,
	}))
	if err != nil {
		return nil, fmt.Errorf("list 1Password keyring items: %w", err)
	}

	return overviews, nil
}

func (k *onePasswordKeyring) itemTitle() string {
	return k.title
}

func (k *onePasswordKeyring) context() (context.Context, context.CancelFunc) {
	return context.WithTimeout(onePasswordRootContext, k.timeout)
}

func onePasswordFields(item keyring.Item) []onepassword.ItemField {
	value := base64.StdEncoding.EncodeToString(item.Data)

	return []onepassword.ItemField{
		{
			ID:        onePasswordUsernameFieldID,
			Title:     "username",
			Value:     item.Key,
			FieldType: onepassword.ItemFieldTypeText,
		},
		{
			ID:        onePasswordCredentialFieldID,
			Title:     "credential",
			Value:     value,
			FieldType: onepassword.ItemFieldTypeConcealed,
		},
		{
			ID:        onePasswordTypeFieldID,
			Title:     "type",
			Value:     item.Label,
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
			Value:     item.Description,
			FieldType: onepassword.ItemFieldTypeText,
		},
	}
}

func keyringItemFromOnePassword(item onepassword.Item) (keyring.Item, error) {
	key, ok := onePasswordKeyField(item)
	if !ok || strings.TrimSpace(key) == "" {
		return keyring.Item{}, keyring.ErrKeyNotFound
	}

	encodedValue, ok := onePasswordValueField(item)
	if !ok {
		return keyring.Item{}, fmt.Errorf("%w: item %q has no value field", errMissingOnePasswordValue, item.ID)
	}

	data, err := base64.StdEncoding.DecodeString(encodedValue)
	if err != nil {
		return keyring.Item{}, fmt.Errorf("decode 1Password keyring value: %w", err)
	}

	label, _ := onePasswordLabelField(item)
	description, _ := onePasswordDescriptionField(item)

	return keyring.Item{
		Key:         key,
		Data:        data,
		Label:       label,
		Description: description,
	}, nil
}

func onePasswordKeyField(item onepassword.Item) (string, bool) {
	return onePasswordField(item, onePasswordUsernameFieldID)
}

func onePasswordValueField(item onepassword.Item) (string, bool) {
	return onePasswordField(item, onePasswordCredentialFieldID)
}

func onePasswordLabelField(item onepassword.Item) (string, bool) {
	return onePasswordField(item, onePasswordTypeFieldID)
}

func onePasswordDescriptionField(item onepassword.Item) (string, bool) {
	return onePasswordField(item, onePasswordHostnameFieldID)
}

func onePasswordItemHasKeyringValue(item onepassword.Item) bool {
	key, keyOK := onePasswordKeyField(item)
	value, valueOK := onePasswordValueField(item)

	return (keyOK && strings.TrimSpace(key) != "") || (valueOK && strings.TrimSpace(value) != "")
}

func isReusableOnePasswordTemplateItem(item onepassword.Item, title string) bool {
	return item.Title == title &&
		item.Category == onepassword.ItemCategoryAPICredentials &&
		!onePasswordItemHasKeyringValue(item)
}

func onePasswordField(item onepassword.Item, id string) (string, bool) {
	for _, field := range item.Fields {
		if field.ID == id || field.Title == id {
			return field.Value, true
		}
	}

	return "", false
}

func appendOnePasswordTag(tags []string) []string {
	if hasOnePasswordTag(tags) {
		return tags
	}

	return append(tags, onePasswordTag)
}

func hasOnePasswordTag(tags []string) bool {
	for _, tag := range tags {
		if tag == onePasswordTag {
			return true
		}
	}

	return false
}

func isOnePasswordKeyringItem(overview onepassword.ItemOverview, title string) bool {
	return overview.Title == title &&
		overview.Category == onepassword.ItemCategoryAPICredentials &&
		hasOnePasswordTag(overview.Tags)
}

func isOnePasswordNotFound(err error) bool {
	if err == nil {
		return false
	}

	msg := strings.ToLower(err.Error())

	return strings.Contains(msg, "not found") || strings.Contains(msg, "notfound")
}
