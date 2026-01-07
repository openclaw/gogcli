package cmd

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/steipete/gogcli/internal/outfmt"
	"github.com/steipete/gogcli/internal/ui"
)

const (
	directoryReadMask       = "names,emailAddresses"
	directoryRequestTimeout = 20 * time.Second
)

type ContactsDirectoryCmd struct {
	List   ContactsDirectoryListCmd   `cmd:"" name:"list" help:"List people from the Workspace directory"`
	Search ContactsDirectorySearchCmd `cmd:"" name:"search" help:"Search people in the Workspace directory"`
}

type ContactsDirectoryListCmd struct {
	Max  int64  `name:"max" aliases:"limit" help:"Max results" default:"50"`
	Page string `name:"page" help:"Page token"`
}

func (c *ContactsDirectoryListCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}

	svc, err := newPeopleDirectoryService(ctx, account)
	if err != nil {
		return err
	}

	ctxTimeout, cancel := context.WithTimeout(ctx, directoryRequestTimeout)
	defer cancel()

	resp, err := svc.People.ListDirectoryPeople().
		Sources("DIRECTORY_SOURCE_TYPE_DOMAIN_PROFILE").
		ReadMask(directoryReadMask).
		PageSize(c.Max).
		PageToken(c.Page).
		Context(ctxTimeout).
		Do()
	if err != nil {
		return err
	}
	if outfmt.IsJSON(ctx) {
		type item struct {
			Resource string `json:"resource"`
			Name     string `json:"name,omitempty"`
			Email    string `json:"email,omitempty"`
		}
		items := make([]item, 0, len(resp.People))
		for _, p := range resp.People {
			if p == nil {
				continue
			}
			items = append(items, item{
				Resource: p.ResourceName,
				Name:     primaryName(p),
				Email:    primaryEmail(p),
			})
		}
		return outfmt.WriteJSON(os.Stdout, map[string]any{
			"people":        items,
			"nextPageToken": resp.NextPageToken,
		})
	}

	if len(resp.People) == 0 {
		u.Err().Println("No results")
		return nil
	}

	w, flush := tableWriter(ctx)
	defer flush()
	fmt.Fprintln(w, "RESOURCE\tNAME\tEMAIL")
	for _, p := range resp.People {
		if p == nil {
			continue
		}
		fmt.Fprintf(w, "%s\t%s\t%s\n",
			p.ResourceName,
			sanitizeTab(primaryName(p)),
			sanitizeTab(primaryEmail(p)),
		)
	}
	printNextPageHint(u, resp.NextPageToken)
	return nil
}

type ContactsDirectorySearchCmd struct {
	Query []string `arg:"" name:"query" help:"Search query"`
	Max   int64    `name:"max" aliases:"limit" help:"Max results" default:"50"`
	Page  string   `name:"page" help:"Page token"`
}

func (c *ContactsDirectorySearchCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}
	query := strings.Join(c.Query, " ")

	svc, err := newPeopleDirectoryService(ctx, account)
	if err != nil {
		return err
	}

	ctxTimeout, cancel := context.WithTimeout(ctx, directoryRequestTimeout)
	defer cancel()

	resp, err := svc.People.SearchDirectoryPeople().
		Query(query).
		Sources("DIRECTORY_SOURCE_TYPE_DOMAIN_PROFILE").
		ReadMask(directoryReadMask).
		PageSize(c.Max).
		PageToken(c.Page).
		Context(ctxTimeout).
		Do()
	if err != nil {
		return err
	}
	if outfmt.IsJSON(ctx) {
		type item struct {
			Resource string `json:"resource"`
			Name     string `json:"name,omitempty"`
			Email    string `json:"email,omitempty"`
		}
		items := make([]item, 0, len(resp.People))
		for _, p := range resp.People {
			if p == nil {
				continue
			}
			items = append(items, item{
				Resource: p.ResourceName,
				Name:     primaryName(p),
				Email:    primaryEmail(p),
			})
		}
		return outfmt.WriteJSON(os.Stdout, map[string]any{
			"people":        items,
			"nextPageToken": resp.NextPageToken,
		})
	}

	if len(resp.People) == 0 {
		u.Err().Println("No results")
		return nil
	}

	w, flush := tableWriter(ctx)
	defer flush()
	fmt.Fprintln(w, "RESOURCE\tNAME\tEMAIL")
	for _, p := range resp.People {
		if p == nil {
			continue
		}
		fmt.Fprintf(w, "%s\t%s\t%s\n",
			p.ResourceName,
			sanitizeTab(primaryName(p)),
			sanitizeTab(primaryEmail(p)),
		)
	}
	printNextPageHint(u, resp.NextPageToken)
	return nil
}

type ContactsOtherCmd struct {
	List   ContactsOtherListCmd   `cmd:"" name:"list" help:"List other contacts"`
	Search ContactsOtherSearchCmd `cmd:"" name:"search" help:"Search other contacts"`
}

type ContactsOtherListCmd struct {
	Max  int64  `name:"max" aliases:"limit" help:"Max results" default:"100"`
	Page string `name:"page" help:"Page token"`
}

func (c *ContactsOtherListCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}

	svc, err := newPeopleOtherContactsService(ctx, account)
	if err != nil {
		return err
	}

	resp, err := svc.OtherContacts.List().
		ReadMask(contactsReadMask).
		PageSize(c.Max).
		PageToken(c.Page).
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
		items := make([]item, 0, len(resp.OtherContacts))
		for _, p := range resp.OtherContacts {
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
		return outfmt.WriteJSON(os.Stdout, map[string]any{
			"contacts":      items,
			"nextPageToken": resp.NextPageToken,
		})
	}

	if len(resp.OtherContacts) == 0 {
		u.Err().Println("No results")
		return nil
	}

	w, flush := tableWriter(ctx)
	defer flush()
	fmt.Fprintln(w, "RESOURCE\tNAME\tEMAIL\tPHONE")
	for _, p := range resp.OtherContacts {
		if p == nil {
			continue
		}
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\n",
			p.ResourceName,
			sanitizeTab(primaryName(p)),
			sanitizeTab(primaryEmail(p)),
			sanitizeTab(primaryPhone(p)),
		)
	}
	printNextPageHint(u, resp.NextPageToken)
	return nil
}

type ContactsOtherSearchCmd struct {
	Query []string `arg:"" name:"query" help:"Search query"`
	Max   int64    `name:"max" aliases:"limit" help:"Max results" default:"50"`
}

func (c *ContactsOtherSearchCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}
	query := strings.Join(c.Query, " ")

	svc, err := newPeopleOtherContactsService(ctx, account)
	if err != nil {
		return err
	}

	resp, err := svc.OtherContacts.Search().
		Query(query).
		ReadMask(contactsReadMask).
		PageSize(c.Max).
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
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\n",
			p.ResourceName,
			sanitizeTab(primaryName(p)),
			sanitizeTab(primaryEmail(p)),
			sanitizeTab(primaryPhone(p)),
		)
	}
	return nil
}
