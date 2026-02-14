package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"reflect"
	"sort"
	"strings"

	"google.golang.org/api/people/v1"

	"github.com/steipete/gogcli/internal/outfmt"
	"github.com/steipete/gogcli/internal/ui"
)

// contactsUpdateMaskFields matches the documented updatePersonFields values for
// people.people.updateContact.
var contactsUpdateMaskFields = map[string]struct{}{
	"addresses":      {},
	"biographies":    {},
	"birthdays":      {},
	"calendarUrls":   {},
	"clientData":     {},
	"emailAddresses": {},
	"events":         {},
	"externalIds":    {},
	"genders":        {},
	"imClients":      {},
	"interests":      {},
	"locales":        {},
	"locations":      {},
	"memberships":    {},
	"miscKeywords":   {},
	"names":          {},
	"nicknames":      {},
	"occupations":    {},
	"organizations":  {},
	"phoneNumbers":   {},
	"relations":      {},
	"sipAddresses":   {},
	"urls":           {},
	"userDefined":    {},
}

const (
	contactsJSONKeyContact  = "contact"
	contactsJSONKeyETag     = "etag"
	contactsJSONKeyMetadata = "metadata"
	contactsJSONKeyResource = "resourceName"
)

func jsonFieldToGoField(jsonField string) string {
	jsonField = strings.TrimSpace(jsonField)
	if jsonField == "" {
		return ""
	}
	return strings.ToUpper(jsonField[:1]) + jsonField[1:]
}

func appendUnique(ss []string, v string) []string {
	for _, cur := range ss {
		if cur == v {
			return ss
		}
	}
	return append(ss, v)
}

func forceSendEmptySliceField(p *people.Person, goField string) {
	if p == nil || strings.TrimSpace(goField) == "" {
		return
	}
	rv := reflect.ValueOf(p)
	if rv.Kind() != reflect.Pointer || rv.IsNil() {
		return
	}
	elem := rv.Elem()
	if elem.Kind() != reflect.Struct {
		return
	}
	fv := elem.FieldByName(goField)
	if !fv.IsValid() || fv.Kind() != reflect.Slice {
		return
	}
	if fv.IsNil() {
		fv.Set(reflect.MakeSlice(fv.Type(), 0, 0))
	}
	if fv.Len() == 0 {
		p.ForceSendFields = appendUnique(p.ForceSendFields, goField)
	}
}

func firstNonEmpty(ss ...string) string {
	for _, s := range ss {
		if strings.TrimSpace(s) != "" {
			return strings.TrimSpace(s)
		}
	}
	return ""
}

func contactSourceETag(p *people.Person) string {
	if p == nil || p.Metadata == nil {
		return ""
	}
	for _, s := range p.Metadata.Sources {
		if s == nil {
			continue
		}
		if strings.EqualFold(s.Type, "CONTACT") && strings.TrimSpace(s.Etag) != "" {
			return strings.TrimSpace(s.Etag)
		}
	}
	for _, s := range p.Metadata.Sources {
		if s == nil {
			continue
		}
		if strings.TrimSpace(s.Etag) != "" {
			return strings.TrimSpace(s.Etag)
		}
	}
	return ""
}

func openFileOrStdin(path string) (io.Reader, func(), error) {
	if strings.TrimSpace(path) == "" {
		return nil, nil, usage("missing --from-file path")
	}
	if path == "-" {
		return os.Stdin, nil, nil
	}
	// #nosec G304 -- user-controlled CLI input; reading arbitrary files is expected here.
	f, err := os.Open(path)
	if err != nil {
		return nil, nil, fmt.Errorf("open %s: %w", path, err)
	}
	return f, func() { _ = f.Close() }, nil
}

func parseContactsUpdateJSON(data []byte) (*people.Person, map[string]json.RawMessage, error) {
	data = []byte(strings.TrimSpace(string(data)))
	if len(data) == 0 {
		return nil, nil, usage("empty JSON input")
	}

	// Support wrapped format from `gog contacts get --json`: {"contact": {...}}.
	var outer map[string]json.RawMessage
	if err := json.Unmarshal(data, &outer); err != nil {
		return nil, nil, fmt.Errorf("parse JSON: %w", err)
	}
	if raw, ok := outer[contactsJSONKeyContact]; ok && len(raw) > 0 && raw[0] == '{' {
		data = raw
	}

	var present map[string]json.RawMessage
	if err := json.Unmarshal(data, &present); err != nil {
		return nil, nil, fmt.Errorf("parse JSON object: %w", err)
	}
	var p people.Person
	if err := json.Unmarshal(data, &p); err != nil {
		return nil, nil, fmt.Errorf("parse contact JSON: %w", err)
	}
	return &p, present, nil
}

func contactsUpdateMaskFromKeys(keys map[string]json.RawMessage) ([]string, error) {
	update := make([]string, 0, len(keys))
	unsupported := make([]string, 0)
	for k := range keys {
		if _, ok := contactsUpdateMaskFields[k]; ok {
			update = append(update, k)
			continue
		}
		switch k {
		case contactsJSONKeyResource, contactsJSONKeyETag, contactsJSONKeyMetadata:
			// Allowed (but not part of updatePersonFields).
			continue
		default:
			unsupported = append(unsupported, k)
		}
	}
	if len(unsupported) > 0 {
		sort.Strings(unsupported)
		return nil, usage("JSON contains fields that aren't updatable via people.updateContact updatePersonFields: " + strings.Join(unsupported, ", "))
	}
	sort.Strings(update)
	return update, nil
}

func (c *ContactsUpdateCmd) updateFromJSON(ctx context.Context, svc *people.Service, resourceName string, u *ui.UI) error {
	reader, closeFn, err := openFileOrStdin(strings.TrimSpace(c.FromFile))
	if err != nil {
		return err
	}
	if closeFn != nil {
		defer closeFn()
	}
	data, err := io.ReadAll(reader)
	if err != nil {
		return fmt.Errorf("read JSON: %w", err)
	}

	inputPerson, presentKeys, err := parseContactsUpdateJSON(data)
	if err != nil {
		return err
	}

	updateFields, err := contactsUpdateMaskFromKeys(presentKeys)
	if err != nil {
		return err
	}
	if len(updateFields) == 0 {
		return usage("no updatable fields found in JSON (needs one of updatePersonFields fields like urls, biographies, ...)")
	}

	// Fetch current metadata/etag (required by updateContact).
	cur, err := svc.People.Get(resourceName).PersonFields("metadata").Do()
	if err != nil {
		return err
	}
	curETag := firstNonEmpty(contactSourceETag(cur), strings.TrimSpace(cur.Etag))
	inputETag := firstNonEmpty(contactSourceETag(inputPerson), strings.TrimSpace(inputPerson.Etag))
	if inputETag == "" {
		u.Err().Println("warning: JSON input is missing an etag; consider starting from `gog contacts get ... --json`")
	} else if !c.IgnoreETag && curETag != "" && inputETag != curETag {
		return usage("etag mismatch (contact changed). Re-run `gog contacts get ... --json`, re-apply edits, retry (or pass --ignore-etag).")
	}

	if strings.TrimSpace(inputPerson.ResourceName) != "" && strings.TrimSpace(inputPerson.ResourceName) != resourceName {
		return usage("resourceName in JSON does not match CLI argument")
	}

	// Enforce resourceName and required metadata.
	inputPerson.ResourceName = resourceName
	inputPerson.Metadata = cur.Metadata
	if curETag != "" {
		inputPerson.Etag = curETag
	}

	for _, f := range updateFields {
		forceSendEmptySliceField(inputPerson, jsonFieldToGoField(f))
	}

	updated, err := svc.People.UpdateContact(resourceName, inputPerson).
		UpdatePersonFields(strings.Join(updateFields, ",")).
		Do()
	if err != nil {
		return err
	}
	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(ctx, os.Stdout, map[string]any{"contact": updated})
	}
	u.Out().Printf("resource\t%s", updated.ResourceName)
	return nil
}
