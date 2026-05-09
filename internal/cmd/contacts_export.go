package cmd

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"strings"

	"google.golang.org/api/people/v1"

	"github.com/steipete/gogcli/internal/ui"
)

const contactsExportReadMask = "names,nicknames,emailAddresses,phoneNumbers,addresses,birthdays,organizations,urls,biographies,memberships,userDefined,metadata"

type ContactsExportCmd struct {
	Selector string `arg:"" optional:"" name:"selector" help:"Contact resource name (people/...), email, or name"`
	Query    string `name:"query" help:"Search query to export (max 30 results)"`
	All      bool   `name:"all" help:"Export all personal contacts"`
	Out      string `name:"out" short:"o" help:"Output path (.vcf), or - for stdout" default:"-"`
	Max      int64  `name:"max" aliases:"limit" help:"Max results for --query (1-30)" default:"30"`
	PageSize int64  `name:"page-size" help:"Page size for --all (1-1000)" default:"1000"`
	Page     string `name:"page" help:"Start page token for --all"`
}

func (c *ContactsExportCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}
	validateErr := c.validate()
	if validateErr != nil {
		return validateErr
	}

	svc, err := newPeopleContactsService(ctx, account)
	if err != nil {
		return err
	}

	contacts, err := c.loadContacts(ctx, svc)
	if err != nil {
		return err
	}
	if len(contacts) == 0 {
		return usage("no contacts matched")
	}

	groups, err := contactGroupNamesForExport(ctx, svc, contacts)
	if err != nil {
		return err
	}

	var buf bytes.Buffer
	writeErr := writeContactsVCard(&buf, contacts, groups)
	if writeErr != nil {
		return writeErr
	}
	if c.Out == "-" || strings.TrimSpace(c.Out) == "" {
		_, err = os.Stdout.Write(buf.Bytes())
		return err
	}
	if err := os.WriteFile(c.Out, buf.Bytes(), 0o600); err != nil {
		return err
	}
	u.Err().Printf("Exported %d contact%s to %s\n", len(contacts), pluralS(len(contacts)), c.Out)
	return nil
}

func (c *ContactsExportCmd) validate() error {
	selectors := 0
	if strings.TrimSpace(c.Selector) != "" {
		selectors++
	}
	if strings.TrimSpace(c.Query) != "" {
		selectors++
	}
	if c.All {
		selectors++
	}
	if selectors != 1 {
		return usage("provide exactly one of selector, --query, or --all")
	}
	if c.Max < 1 || c.Max > 30 {
		return usage("--max must be between 1 and 30")
	}
	if c.PageSize < 1 || c.PageSize > 1000 {
		return usage("--page-size must be between 1 and 1000")
	}
	if strings.TrimSpace(c.Page) != "" && !c.All {
		return usage("--page is only valid with --all")
	}
	return nil
}

func (c *ContactsExportCmd) loadContacts(ctx context.Context, svc *people.Service) ([]*people.Person, error) {
	switch {
	case c.All:
		return c.loadAllContacts(ctx, svc)
	case strings.TrimSpace(c.Query) != "":
		return c.searchContacts(ctx, svc, strings.TrimSpace(c.Query), c.Max)
	default:
		return c.loadSelectedContact(ctx, svc, strings.TrimSpace(c.Selector))
	}
}

func (c *ContactsExportCmd) loadAllContacts(ctx context.Context, svc *people.Service) ([]*people.Person, error) {
	contacts, _, err := loadPagedItems(c.Page, true, func(pageToken string) ([]*people.Person, string, error) {
		call := svc.People.Connections.List(peopleMeResource).
			PersonFields(contactsExportReadMask).
			PageSize(c.PageSize).
			Context(ctx)
		if pageToken != "" {
			call = call.PageToken(pageToken)
		}
		resp, err := call.Do()
		if err != nil {
			return nil, "", err
		}
		return resp.Connections, resp.NextPageToken, nil
	})
	return contacts, err
}

func (c *ContactsExportCmd) searchContacts(ctx context.Context, svc *people.Service, query string, limit int64) ([]*people.Person, error) {
	warmSearchContactsCache(ctx, svc)
	resp, err := svc.People.SearchContacts().
		Query(query).
		PageSize(limit).
		ReadMask(contactsExportReadMask).
		Context(ctx).
		Do()
	if err != nil {
		return nil, err
	}
	contacts := make([]*people.Person, 0, len(resp.Results))
	for _, result := range resp.Results {
		if result != nil && result.Person != nil {
			contacts = append(contacts, result.Person)
		}
	}
	return contacts, nil
}

func (c *ContactsExportCmd) loadSelectedContact(ctx context.Context, svc *people.Service, selector string) ([]*people.Person, error) {
	if strings.HasPrefix(selector, "people/") {
		p, err := svc.People.Get(selector).
			PersonFields(contactsExportReadMask).
			Context(ctx).
			Do()
		if err != nil {
			return nil, err
		}
		return []*people.Person{p}, nil
	}

	contacts, err := c.searchContacts(ctx, svc, selector, 30)
	if err != nil {
		return nil, err
	}
	if len(contacts) == 0 {
		return nil, nil
	}
	if strings.Contains(selector, "@") {
		exact := contacts[:0]
		for _, p := range contacts {
			if personHasEmail(p, selector) {
				exact = append(exact, p)
			}
		}
		if len(exact) == 1 {
			return exact, nil
		}
		if len(exact) > 1 {
			return nil, fmt.Errorf("ambiguous contact selector %q matched %d contacts with that email", selector, len(exact))
		}
	}
	if len(contacts) == 1 {
		return contacts, nil
	}
	return nil, fmt.Errorf("ambiguous contact selector %q matched %d contacts; use people/... or --query", selector, len(contacts))
}

func warmSearchContactsCache(ctx context.Context, svc *people.Service) {
	_, _ = svc.People.SearchContacts().
		Query("").
		PageSize(1).
		ReadMask("names").
		Context(ctx).
		Do()
}

func personHasEmail(p *people.Person, email string) bool {
	for _, e := range p.EmailAddresses {
		if e != nil && strings.EqualFold(strings.TrimSpace(e.Value), strings.TrimSpace(email)) {
			return true
		}
	}
	return false
}

func contactGroupNamesForExport(ctx context.Context, svc *people.Service, contacts []*people.Person) (map[string]string, error) {
	if !contactsHaveGroupMemberships(contacts) {
		return map[string]string{}, nil
	}
	groups, err := fetchExportContactGroups(ctx, svc)
	if err != nil {
		return nil, fmt.Errorf("list contact groups: %w", err)
	}
	return groups, nil
}

func contactsHaveGroupMemberships(contacts []*people.Person) bool {
	for _, p := range contacts {
		for _, m := range p.Memberships {
			if m != nil && m.ContactGroupMembership != nil && m.ContactGroupMembership.ContactGroupResourceName != "" {
				return true
			}
		}
	}
	return false
}

func fetchExportContactGroups(ctx context.Context, svc *people.Service) (map[string]string, error) {
	out := map[string]string{}
	pageToken := ""
	for {
		call := svc.ContactGroups.List().
			PageSize(1000).
			GroupFields("groupType,metadata,name").
			Context(ctx)
		if pageToken != "" {
			call = call.PageToken(pageToken)
		}
		resp, err := call.Do()
		if err != nil {
			return nil, err
		}
		for _, group := range resp.ContactGroups {
			if group == nil || group.GroupType != "USER_CONTACT_GROUP" || group.Name == "" {
				continue
			}
			if group.ResourceName != "" {
				out[group.ResourceName] = group.Name
			}
		}
		if resp.NextPageToken == "" {
			break
		}
		pageToken = resp.NextPageToken
	}
	return out, nil
}
