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

type ContactsCmd struct {
	Search    ContactsSearchCmd    `cmd:"" name:"search" help:"Search contacts by name/email/phone"`
	List      ContactsListCmd      `cmd:"" name:"list" help:"List contacts"`
	Get       ContactsGetCmd       `cmd:"" name:"get" help:"Get a contact"`
	Create    ContactsCreateCmd    `cmd:"" name:"create" help:"Create a contact"`
	Update    ContactsUpdateCmd    `cmd:"" name:"update" help:"Update a contact"`
	Delete    ContactsDeleteCmd    `cmd:"" name:"delete" help:"Delete a contact"`
	Directory ContactsDirectoryCmd `cmd:"" name:"directory" help:"Directory contacts"`
	Other     ContactsOtherCmd     `cmd:"" name:"other" help:"Other contacts"`
}

type ContactsSearchCmd struct {
	Query []string `arg:"" name:"query" help:"Search query"`
	Max   int64    `name:"max" aliases:"limit" help:"Max results" default:"50"`
}

func (c *ContactsSearchCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}
	query := strings.Join(c.Query, " ")

	svc, err := newPeopleContactsService(ctx, account)
	if err != nil {
		return err
	}

	resp, err := svc.People.SearchContacts().
		Query(query).
		PageSize(c.Max).
		ReadMask("names,emailAddresses,phoneNumbers").
		Do()
	if err != nil {
		return err
	}
	if outfmt.IsJSON(ctx) {
		type item struct {
			Resource string `json:"resource"`
			Name     string `json:"name,omitempty"`
			Email    string `json:"email,omitempty"`
			Phone    string `json:"phone,omitempty"`
		}
		items := make([]item, 0, len(resp.Results))
		for _, r := range resp.Results {
			p := r.Person
			if p == nil {
				continue
			}
			items = append(items, item{
				Resource: p.ResourceName,
				Name:     primaryName(p),
				Email:    primaryEmail(p),
				Phone:    primaryPhone(p),
			})
		}
		return outfmt.WriteJSON(os.Stdout, map[string]any{"contacts": items})
	}
	if len(resp.Results) == 0 {
		u.Err().Println("No results")
		return nil
	}

	w, flush := tableWriter(ctx)
	defer flush()
	fmt.Fprintln(w, "RESOURCE\tNAME\tEMAIL\tPHONE")
	for _, r := range resp.Results {
		p := r.Person
		if p == nil {
			continue
		}
		fmt.Fprintf(
			w,
			"%s\t%s\t%s\t%s\n",
			p.ResourceName,
			sanitizeTab(primaryName(p)),
			sanitizeTab(primaryEmail(p)),
			sanitizeTab(primaryPhone(p)),
		)
	}
	return nil
}

func primaryName(p *people.Person) string {
	if p == nil || len(p.Names) == 0 || p.Names[0] == nil {
		return ""
	}
	if p.Names[0].DisplayName != "" {
		return p.Names[0].DisplayName
	}
	return strings.TrimSpace(strings.Join([]string{p.Names[0].GivenName, p.Names[0].FamilyName}, " "))
}

func primaryEmail(p *people.Person) string {
	if p == nil || len(p.EmailAddresses) == 0 || p.EmailAddresses[0] == nil {
		return ""
	}
	return p.EmailAddresses[0].Value
}

func primaryPhone(p *people.Person) string {
	if p == nil || len(p.PhoneNumbers) == 0 || p.PhoneNumbers[0] == nil {
		return ""
	}
	return p.PhoneNumbers[0].Value
}

func sanitizeTab(s string) string {
	return strings.ReplaceAll(s, "\t", " ")
}
