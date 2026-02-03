package cmd

import (
	"context"
	"fmt"
	"os"
	"strings"

	"google.golang.org/api/people/v1"

	"github.com/steipete/gogcli/internal/outfmt"
	"github.com/steipete/gogcli/internal/ui"
)

type ContactsDomainCmd struct {
	List   ContactsDomainListCmd   `cmd:"" name:"list" help:"List domain shared contacts"`
	Create ContactsDomainCreateCmd `cmd:"" name:"create" help:"Create a domain shared contact"`
	Delete ContactsDomainDeleteCmd `cmd:"" name:"delete" help:"Delete a domain shared contact"`
}

type ContactsDomainListCmd struct {
	Max  int64  `name:"max" aliases:"limit" default:"50" help:"Max results"`
	Page string `name:"page" help:"Page token"`
}

func (c *ContactsDomainListCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}

	svc, err := newPeopleDirectoryService(ctx, account)
	if err != nil {
		return err
	}

	call := svc.People.ListDirectoryPeople().ReadMask(contactsReadMask).PageSize(c.Max)
	if c.Page != "" {
		call = call.PageToken(c.Page)
	}

	resp, err := call.Do()
	if err != nil {
		return err
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(os.Stdout, map[string]any{
			"people":        resp.People,
			"nextPageToken": resp.NextPageToken,
		})
	}

	if len(resp.People) == 0 {
		u.Err().Println("No domain contacts found")
		return nil
	}

	w, flush := tableWriter(ctx)
	defer flush()
	fmt.Fprintln(w, "RESOURCE\tNAME\tEMAIL\tPHONE")
	for _, p := range resp.People {
		if p == nil {
			continue
		}
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\n",
			sanitizeTab(p.ResourceName),
			sanitizeTab(primaryName(p)),
			sanitizeTab(primaryEmail(p)),
			sanitizeTab(primaryPhone(p)),
		)
	}
	printNextPageHint(u, resp.NextPageToken)
	return nil
}

type ContactsDomainCreateCmd struct {
	Email string `name:"email" help:"Email address" required:""`
	Name  string `name:"name" help:"Contact name" required:""`
}

func (c *ContactsDomainCreateCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}

	email := strings.TrimSpace(c.Email)
	name := strings.TrimSpace(c.Name)
	if email == "" || name == "" {
		return usage("--email and --name are required")
	}

	svc, err := newPeopleContactsService(ctx, account)
	if err != nil {
		return err
	}

	person := &people.Person{
		Names:          []*people.Name{{DisplayName: name}},
		EmailAddresses: []*people.EmailAddress{{Value: email}},
	}
	created, err := svc.People.CreateContact(person).Do()
	if err != nil {
		return fmt.Errorf("create domain contact: %w", err)
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(os.Stdout, created)
	}

	u.Out().Printf("Created contact: %s\n", created.ResourceName)
	return nil
}

type ContactsDomainDeleteCmd struct {
	Email string `arg:"" name:"email" help:"Email or resource name"`
}

func (c *ContactsDomainDeleteCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}

	identifier := strings.TrimSpace(c.Email)
	if identifier == "" {
		return usage("email is required")
	}

	svc, err := newPeopleContactsService(ctx, account)
	if err != nil {
		return err
	}

	resource := identifier
	if !strings.HasPrefix(resource, "people/") {
		search, err := svc.People.SearchContacts().Query(identifier).ReadMask(contactsReadMask).PageSize(1).Do()
		if err != nil {
			return fmt.Errorf("search contact: %w", err)
		}
		if len(search.Results) == 0 || search.Results[0].Person == nil {
			return fmt.Errorf("no contact found for %s", identifier)
		}
		resource = search.Results[0].Person.ResourceName
	}

	if err := confirmDestructive(ctx, flags, fmt.Sprintf("delete contact %s", resource)); err != nil {
		return err
	}

	if _, err := svc.People.DeleteContact(resource).Do(); err != nil {
		return fmt.Errorf("delete contact: %w", err)
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(os.Stdout, map[string]any{"resource": resource, "deleted": true})
	}

	u.Out().Printf("Deleted contact: %s\n", resource)
	return nil
}
