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
	List      ContactsListCmd      `cmd:"" name:"list" aliases:"ls" help:"List contacts"`
	Get       ContactsGetCmd       `cmd:"" name:"get" aliases:"info,show" help:"Get a contact"`
	Create    ContactsCreateCmd    `cmd:"" name:"create" aliases:"add,new" help:"Create a contact"`
	Update    ContactsUpdateCmd    `cmd:"" name:"update" aliases:"edit,set" help:"Update a contact"`
	Delete    ContactsDeleteCmd    `cmd:"" name:"delete" aliases:"rm,del,remove" help:"Delete a contact"`
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
		ReadMask(contactsReadMask).
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
		return outfmt.WriteJSON(ctx, os.Stdout, map[string]any{"contacts": items})
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

func primaryBirthday(p *people.Person) string {
	if p == nil || len(p.Birthdays) == 0 {
		return ""
	}
	var chosen *people.Birthday
	for _, b := range p.Birthdays {
		if b == nil {
			continue
		}
		if b.Metadata != nil && b.Metadata.Primary {
			chosen = b
			break
		}
		if chosen == nil {
			chosen = b
		}
	}
	if chosen == nil {
		return ""
	}
	if formatted := formatPartialDate(chosen.Date); formatted != "" {
		return formatted
	}
	return strings.TrimSpace(chosen.Text)
}

func formatPartialDate(d *people.Date) string {
	if d == nil {
		return ""
	}
	parts := make([]string, 0, 3)
	if d.Year > 0 {
		parts = append(parts, fmt.Sprintf("%04d", d.Year))
	}
	if d.Month > 0 {
		parts = append(parts, fmt.Sprintf("%02d", d.Month))
	}
	if d.Day > 0 {
		parts = append(parts, fmt.Sprintf("%02d", d.Day))
	}
	if len(parts) == 0 {
		return ""
	}
	return strings.Join(parts, "-")
}

func sanitizeTab(s string) string {
	return strings.ReplaceAll(s, "\t", " ")
}

func primaryOrganization(p *people.Person) (name, title string) {
	if p == nil || len(p.Organizations) == 0 || p.Organizations[0] == nil {
		return "", ""
	}
	return p.Organizations[0].Name, p.Organizations[0].Title
}

func allURLs(p *people.Person) []string {
	if p == nil || len(p.Urls) == 0 {
		return nil
	}
	urls := make([]string, 0, len(p.Urls))
	for _, u := range p.Urls {
		if u != nil && u.Value != "" {
			urls = append(urls, u.Value)
		}
	}
	return urls
}

func primaryBio(p *people.Person) string {
	if p == nil || len(p.Biographies) == 0 || p.Biographies[0] == nil {
		return ""
	}
	return p.Biographies[0].Value
}

func userDefinedFields(p *people.Person) map[string]string {
	if p == nil || len(p.UserDefined) == 0 {
		return nil
	}
	fields := make(map[string]string, len(p.UserDefined))
	for _, ud := range p.UserDefined {
		if ud != nil && ud.Key != "" {
			fields[ud.Key] = ud.Value
		}
	}
	return fields
}
