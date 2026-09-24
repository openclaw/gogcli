package cmd

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"sort"
	"strings"

	"google.golang.org/api/people/v1"
)

const contactsBatchInputLimit = 32 << 20

func readContactsBatchInput(ctx context.Context, path string) ([]byte, error) {
	reader, closeFn, err := openFileOrStdin(ctx, path)
	if err != nil {
		return nil, err
	}
	if closeFn != nil {
		defer closeFn()
	}
	data, err := io.ReadAll(io.LimitReader(reader, contactsBatchInputLimit+1))
	if err != nil {
		return nil, fmt.Errorf("read contacts JSON: %w", err)
	}
	if len(data) > contactsBatchInputLimit {
		return nil, usage("contacts JSON exceeds 32 MiB")
	}
	if !json.Valid(data) {
		return nil, usage("invalid contacts JSON")
	}
	if err := checkContactsBatchJSONKeys(json.NewDecoder(bytes.NewReader(data))); err != nil {
		return nil, usagef("invalid contacts JSON: %v", err)
	}
	return data, nil
}

// Encoding/json otherwise silently keeps the last occurrence, including resource keys.
func checkContactsBatchJSONKeys(dec *json.Decoder) error {
	token, err := dec.Token()
	if err != nil {
		return err
	}
	delim, ok := token.(json.Delim)
	if !ok {
		return nil
	}
	seen := map[string]bool{}
	for dec.More() {
		if delim == '{' {
			key, keyErr := dec.Token()
			if keyErr != nil {
				return keyErr
			}
			name, _ := key.(string)
			if seen[name] {
				return fmt.Errorf("duplicate key %q", name)
			}
			seen[name] = true
		}
		if valueErr := checkContactsBatchJSONKeys(dec); valueErr != nil {
			return valueErr
		}
	}
	_, err = dec.Token()
	return err
}

func normalizeContactsBatchResources(values []string) ([]string, error) {
	if len(values) == 0 {
		return nil, usage("at least one contact resource name is required")
	}
	names := make([]string, 0, len(values))
	seen := map[string]bool{}
	for _, value := range values {
		name := strings.TrimSpace(value)
		id, found := strings.CutPrefix(name, "people/")
		if !found || id == "" || strings.ContainsAny(id, "/ \t\r\n?#") {
			return nil, usagef("invalid contact resource name %q (expected people/...)", value)
		}
		if seen[name] {
			return nil, usagef("duplicate contact resource name %q", name)
		}
		seen[name] = true
		names = append(names, name)
	}
	return names, nil
}

func parseContactsBatchPerson(raw json.RawMessage, update bool) (*people.Person, []string, error) {
	var keys map[string]json.RawMessage
	if err := json.Unmarshal(raw, &keys); err != nil || keys == nil {
		return nil, nil, usage("each contact must be a Person object")
	}
	fields, err := contactsUpdateMaskFromKeys(keys)
	if err != nil {
		return nil, nil, err
	}
	if len(fields) == 0 {
		return nil, nil, usage("contact has no mutable fields")
	}
	if !update {
		for _, key := range []string{contactsJSONKeyResource, contactsJSONKeyETag, contactsJSONKeyMetadata} {
			if _, found := keys[key]; found {
				return nil, nil, usagef("created contacts must not include %s", key)
			}
		}
	}
	var person people.Person
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&person); err != nil {
		return nil, nil, usagef("invalid Person: %v", err)
	}
	for _, field := range fields {
		var values []json.RawMessage
		if err := json.Unmarshal(keys[field], &values); err != nil {
			return nil, nil, usagef("%s must be an array or null", field)
		}
		for _, value := range values {
			if !bytes.HasPrefix(bytes.TrimSpace(value), []byte("{")) {
				return nil, nil, usagef("%s entries must be objects", field)
			}
		}
		switch field {
		case "biographies", "birthdays", "genders", "names":
			if len(values) > 1 {
				return nil, nil, usagef("%s allows at most one entry", field)
			}
		case "memberships":
			found := false
			for _, membership := range person.Memberships {
				if membership.ContactGroupMembership != nil && membership.ContactGroupMembership.ContactGroupResourceName != "" {
					found = true
				}
			}
			if !found {
				return nil, nil, usage("memberships must include a contact group membership")
			}
		}
	}
	forceSendEmptyPersonListFields(&person, fields)
	return &person, fields, nil
}

func contactsBatchContactSource(p *people.Person) *people.Source {
	if p == nil || p.Metadata == nil {
		return nil
	}
	var contact *people.Source
	for _, source := range p.Metadata.Sources {
		if source != nil && source.Type == "CONTACT" {
			if contact != nil || strings.TrimSpace(source.Etag) == "" {
				return nil
			}
			contact = source
		}
	}
	return contact
}

func parseContactsBatchCreate(data []byte) ([]*people.ContactToCreate, error) {
	var inputs []json.RawMessage
	if err := json.Unmarshal(data, &inputs); err != nil || len(inputs) == 0 {
		return nil, usage("--from-file must contain a nonempty JSON array of Person objects")
	}
	contacts := make([]*people.ContactToCreate, 0, len(inputs))
	for i, raw := range inputs {
		person, _, err := parseContactsBatchPerson(raw, false)
		if err != nil {
			return nil, usagef("contact %d: %v", i, err)
		}
		contacts = append(contacts, &people.ContactToCreate{ContactPerson: person})
	}
	return contacts, nil
}

func parseContactsBatchUpdates(data []byte) ([]*people.BatchUpdateContactsRequest, error) {
	var inputs map[string]json.RawMessage
	if err := json.Unmarshal(data, &inputs); err != nil || len(inputs) == 0 {
		return nil, usage("--from-file must contain a nonempty object keyed by contact resource name")
	}
	names := make([]string, 0, len(inputs))
	for name := range inputs {
		names = append(names, name)
	}
	sort.Strings(names)
	if _, err := normalizeContactsBatchResources(names); err != nil {
		return nil, err
	}
	var batches []*people.BatchUpdateContactsRequest
	byMask := map[string]*people.BatchUpdateContactsRequest{}
	for _, name := range names {
		if name != strings.TrimSpace(name) {
			return nil, usagef("contact resource key %q contains surrounding whitespace", name)
		}
		person, fields, err := parseContactsBatchPerson(inputs[name], true)
		if err != nil {
			return nil, usagef("contact %s: %v", name, err)
		}
		if person.ResourceName != "" && person.ResourceName != name {
			return nil, usagef("contact %s: resourceName does not match its object key", name)
		}
		source := contactsBatchContactSource(person)
		if source == nil {
			return nil, usagef("contact %s requires metadata.sources with one CONTACT source and its etag; start with contacts batch get", name)
		}
		person.ResourceName = name
		person.Metadata = &people.PersonMetadata{Sources: []*people.Source{source}}
		mask := strings.Join(fields, ",")
		batch := byMask[mask]
		if batch == nil || len(batch.Contacts) == contactsBatchReadLimit {
			batch = &people.BatchUpdateContactsRequest{
				Contacts: map[string]people.Person{}, UpdateMask: mask,
				ReadMask: contactsBatchReadMask(), Sources: []string{contactsDedupeContactSource},
			}
			byMask[mask] = batch
			batches = append(batches, batch)
		}
		// A shared union mask would clear fields omitted by other contacts.
		batch.Contacts[name] = *person
	}
	return batches, nil
}
