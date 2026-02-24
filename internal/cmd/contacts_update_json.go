package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
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

func contactsPersonFieldToGoField(personField string) string {
	personField = strings.TrimSpace(personField)
	if personField == "" {
		return ""
	}
	return strings.ToUpper(personField[:1]) + personField[1:]
}

func appendUnique(ss []string, v string) []string {
	for _, cur := range ss {
		if cur == v {
			return ss
		}
	}
	return append(ss, v)
}

var contactsPersonListForceSend = map[string]func(*people.Person) bool{
	"addresses": func(p *people.Person) bool {
		if p.Addresses == nil {
			p.Addresses = []*people.Address{}
		}
		return len(p.Addresses) == 0
	},
	"biographies": func(p *people.Person) bool {
		if p.Biographies == nil {
			p.Biographies = []*people.Biography{}
		}
		return len(p.Biographies) == 0
	},
	"birthdays": func(p *people.Person) bool {
		if p.Birthdays == nil {
			p.Birthdays = []*people.Birthday{}
		}
		return len(p.Birthdays) == 0
	},
	"calendarUrls": func(p *people.Person) bool {
		if p.CalendarUrls == nil {
			p.CalendarUrls = []*people.CalendarUrl{}
		}
		return len(p.CalendarUrls) == 0
	},
	"clientData": func(p *people.Person) bool {
		if p.ClientData == nil {
			p.ClientData = []*people.ClientData{}
		}
		return len(p.ClientData) == 0
	},
	"emailAddresses": func(p *people.Person) bool {
		if p.EmailAddresses == nil {
			p.EmailAddresses = []*people.EmailAddress{}
		}
		return len(p.EmailAddresses) == 0
	},
	"events": func(p *people.Person) bool {
		if p.Events == nil {
			p.Events = []*people.Event{}
		}
		return len(p.Events) == 0
	},
	"externalIds": func(p *people.Person) bool {
		if p.ExternalIds == nil {
			p.ExternalIds = []*people.ExternalId{}
		}
		return len(p.ExternalIds) == 0
	},
	"genders": func(p *people.Person) bool {
		if p.Genders == nil {
			p.Genders = []*people.Gender{}
		}
		return len(p.Genders) == 0
	},
	"imClients": func(p *people.Person) bool {
		if p.ImClients == nil {
			p.ImClients = []*people.ImClient{}
		}
		return len(p.ImClients) == 0
	},
	"interests": func(p *people.Person) bool {
		if p.Interests == nil {
			p.Interests = []*people.Interest{}
		}
		return len(p.Interests) == 0
	},
	"locales": func(p *people.Person) bool {
		if p.Locales == nil {
			p.Locales = []*people.Locale{}
		}
		return len(p.Locales) == 0
	},
	"locations": func(p *people.Person) bool {
		if p.Locations == nil {
			p.Locations = []*people.Location{}
		}
		return len(p.Locations) == 0
	},
	"memberships": func(p *people.Person) bool {
		if p.Memberships == nil {
			p.Memberships = []*people.Membership{}
		}
		return len(p.Memberships) == 0
	},
	"miscKeywords": func(p *people.Person) bool {
		if p.MiscKeywords == nil {
			p.MiscKeywords = []*people.MiscKeyword{}
		}
		return len(p.MiscKeywords) == 0
	},
	"names": func(p *people.Person) bool {
		if p.Names == nil {
			p.Names = []*people.Name{}
		}
		return len(p.Names) == 0
	},
	"nicknames": func(p *people.Person) bool {
		if p.Nicknames == nil {
			p.Nicknames = []*people.Nickname{}
		}
		return len(p.Nicknames) == 0
	},
	"occupations": func(p *people.Person) bool {
		if p.Occupations == nil {
			p.Occupations = []*people.Occupation{}
		}
		return len(p.Occupations) == 0
	},
	"organizations": func(p *people.Person) bool {
		if p.Organizations == nil {
			p.Organizations = []*people.Organization{}
		}
		return len(p.Organizations) == 0
	},
	"phoneNumbers": func(p *people.Person) bool {
		if p.PhoneNumbers == nil {
			p.PhoneNumbers = []*people.PhoneNumber{}
		}
		return len(p.PhoneNumbers) == 0
	},
	"relations": func(p *people.Person) bool {
		if p.Relations == nil {
			p.Relations = []*people.Relation{}
		}
		return len(p.Relations) == 0
	},
	"sipAddresses": func(p *people.Person) bool {
		if p.SipAddresses == nil {
			p.SipAddresses = []*people.SipAddress{}
		}
		return len(p.SipAddresses) == 0
	},
	"urls": func(p *people.Person) bool {
		if p.Urls == nil {
			p.Urls = []*people.Url{}
		}
		return len(p.Urls) == 0
	},
	"userDefined": func(p *people.Person) bool {
		if p.UserDefined == nil {
			p.UserDefined = []*people.UserDefined{}
		}
		return len(p.UserDefined) == 0
	},
}

func forceSendEmptyPersonListField(p *people.Person, personField string) {
	if p == nil {
		return
	}
	personField = strings.TrimSpace(personField)
	if personField == "" {
		return
	}

	ensureFn := contactsPersonListForceSend[personField]
	if ensureFn == nil {
		return
	}
	if !ensureFn(p) {
		return
	}

	goField := contactsPersonFieldToGoField(personField)
	p.ForceSendFields = appendUnique(p.ForceSendFields, goField)
}

func forceSendEmptyPersonListFields(p *people.Person, personFields []string) {
	for _, f := range personFields {
		forceSendEmptyPersonListField(p, f)
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
		return nil, usage("JSON contains unsupported keys for contacts update: " + strings.Join(unsupported, ", ") + ". Include only fields you want to change (for example: urls, biographies, addresses, organizations, ...). Tip: start from `gog contacts get ... --json` and delete keys you don't want to update.")
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

	forceSendEmptyPersonListFields(inputPerson, updateFields)

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
