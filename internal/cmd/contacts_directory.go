package cmd

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"google.golang.org/api/people/v1"

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
	Max       int64  `name:"max" aliases:"limit" help:"Max results" default:"50"`
	Page      string `name:"page" aliases:"cursor" help:"Page token"`
	All       bool   `name:"all" aliases:"all-pages,allpages" help:"Fetch all pages"`
	FailEmpty bool   `name:"fail-empty" aliases:"non-empty,require-results" help:"Exit with code 3 if no results"`
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

	fetch := func(pageToken string) ([]*people.Person, string, error) {
		ctxTimeout, cancel := context.WithTimeout(ctx, directoryRequestTimeout)
		defer cancel()

		call := svc.People.ListDirectoryPeople().
			Sources("DIRECTORY_SOURCE_TYPE_DOMAIN_PROFILE").
			ReadMask(directoryReadMask).
			PageSize(c.Max).
			Context(ctxTimeout)
		if strings.TrimSpace(pageToken) != "" {
			call = call.PageToken(pageToken)
		}

		resp, err := call.Do()
		if err != nil {
			return nil, "", err
		}
		return resp.People, resp.NextPageToken, nil
	}

	var peopleList []*people.Person
	nextPageToken := ""
	if c.All {
		all, err := collectAllPages(c.Page, fetch)
		if err != nil {
			return err
		}
		peopleList = all
	} else {
		var err error
		peopleList, nextPageToken, err = fetch(c.Page)
		if err != nil {
			return err
		}
	}
	if outfmt.IsJSON(ctx) {
		type item struct {
			Resource string `json:"resource"`
			Name     string `json:"name,omitempty"`
			Email    string `json:"email,omitempty"`
		}
		items := make([]item, 0, len(peopleList))
		for _, p := range peopleList {
			if p == nil {
				continue
			}
			items = append(items, item{
				Resource: p.ResourceName,
				Name:     primaryName(p),
				Email:    primaryEmail(p),
			})
		}
		if err := outfmt.WriteJSON(ctx, os.Stdout, map[string]any{
			"people":        items,
			"nextPageToken": nextPageToken,
		}); err != nil {
			return err
		}
		if len(items) == 0 {
			return failEmptyExit(c.FailEmpty)
		}
		return nil
	}

	if len(peopleList) == 0 {
		u.Err().Println("No results")
		return failEmptyExit(c.FailEmpty)
	}

	w, flush := tableWriter(ctx)
	defer flush()
	fmt.Fprintln(w, "RESOURCE\tNAME\tEMAIL")
	for _, p := range peopleList {
		if p == nil {
			continue
		}
		fmt.Fprintf(w, "%s\t%s\t%s\n",
			p.ResourceName,
			sanitizeTab(primaryName(p)),
			sanitizeTab(primaryEmail(p)),
		)
	}
	printNextPageHint(u, nextPageToken)
	return nil
}

type ContactsDirectorySearchCmd struct {
	Query     []string `arg:"" name:"query" help:"Search query"`
	Max       int64    `name:"max" aliases:"limit" help:"Max results" default:"50"`
	Page      string   `name:"page" aliases:"cursor" help:"Page token"`
	All       bool     `name:"all" aliases:"all-pages,allpages" help:"Fetch all pages"`
	FailEmpty bool     `name:"fail-empty" aliases:"non-empty,require-results" help:"Exit with code 3 if no results"`
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

	fetch := func(pageToken string) ([]*people.Person, string, error) {
		ctxTimeout, cancel := context.WithTimeout(ctx, directoryRequestTimeout)
		defer cancel()

		call := svc.People.SearchDirectoryPeople().
			Query(query).
			Sources("DIRECTORY_SOURCE_TYPE_DOMAIN_PROFILE").
			ReadMask(directoryReadMask).
			PageSize(c.Max).
			Context(ctxTimeout)
		if strings.TrimSpace(pageToken) != "" {
			call = call.PageToken(pageToken)
		}
		resp, err := call.Do()
		if err != nil {
			return nil, "", err
		}
		return resp.People, resp.NextPageToken, nil
	}

	var peopleList []*people.Person
	nextPageToken := ""
	if c.All {
		all, err := collectAllPages(c.Page, fetch)
		if err != nil {
			return err
		}
		peopleList = all
	} else {
		var err error
		peopleList, nextPageToken, err = fetch(c.Page)
		if err != nil {
			return err
		}
	}
	if outfmt.IsJSON(ctx) {
		type item struct {
			Resource string `json:"resource"`
			Name     string `json:"name,omitempty"`
			Email    string `json:"email,omitempty"`
		}
		items := make([]item, 0, len(peopleList))
		for _, p := range peopleList {
			if p == nil {
				continue
			}
			items = append(items, item{
				Resource: p.ResourceName,
				Name:     primaryName(p),
				Email:    primaryEmail(p),
			})
		}
		if err := outfmt.WriteJSON(ctx, os.Stdout, map[string]any{
			"people":        items,
			"nextPageToken": nextPageToken,
		}); err != nil {
			return err
		}
		if len(items) == 0 {
			return failEmptyExit(c.FailEmpty)
		}
		return nil
	}

	if len(peopleList) == 0 {
		u.Err().Println("No results")
		return failEmptyExit(c.FailEmpty)
	}

	w, flush := tableWriter(ctx)
	defer flush()
	fmt.Fprintln(w, "RESOURCE\tNAME\tEMAIL")
	for _, p := range peopleList {
		if p == nil {
			continue
		}
		fmt.Fprintf(w, "%s\t%s\t%s\n",
			p.ResourceName,
			sanitizeTab(primaryName(p)),
			sanitizeTab(primaryEmail(p)),
		)
	}
	printNextPageHint(u, nextPageToken)
	return nil
}

type ContactsOtherCmd struct {
	List   ContactsOtherListCmd   `cmd:"" name:"list" help:"List other contacts"`
	Search ContactsOtherSearchCmd `cmd:"" name:"search" help:"Search other contacts"`
	Delete ContactsOtherDeleteCmd `cmd:"" name:"delete" help:"Delete an other contact"`
}

type ContactsOtherListCmd struct {
	Max       int64  `name:"max" aliases:"limit" help:"Max results" default:"100"`
	Page      string `name:"page" aliases:"cursor" help:"Page token"`
	All       bool   `name:"all" aliases:"all-pages,allpages" help:"Fetch all pages"`
	FailEmpty bool   `name:"fail-empty" aliases:"non-empty,require-results" help:"Exit with code 3 if no results"`
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

	fetch := func(pageToken string) ([]*people.Person, string, error) {
		call := svc.OtherContacts.List().
			ReadMask(contactsReadMask).
			PageSize(c.Max).
			Context(ctx)
		if strings.TrimSpace(pageToken) != "" {
			call = call.PageToken(pageToken)
		}
		resp, err := call.Do()
		if err != nil {
			return nil, "", err
		}
		return resp.OtherContacts, resp.NextPageToken, nil
	}

	var contacts []*people.Person
	nextPageToken := ""
	if c.All {
		all, err := collectAllPages(c.Page, fetch)
		if err != nil {
			return err
		}
		contacts = all
	} else {
		var err error
		contacts, nextPageToken, err = fetch(c.Page)
		if err != nil {
			return err
		}
	}
	if outfmt.IsJSON(ctx) {
		type item struct {
			Resource string `json:"resource"`
			Name     string `json:"name,omitempty"`
			Email    string `json:"email,omitempty"`
			Phone    string `json:"phone,omitempty"`
		}
		items := make([]item, 0, len(contacts))
		for _, p := range contacts {
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
		if err := outfmt.WriteJSON(ctx, os.Stdout, map[string]any{
			"contacts":      items,
			"nextPageToken": nextPageToken,
		}); err != nil {
			return err
		}
		if len(items) == 0 {
			return failEmptyExit(c.FailEmpty)
		}
		return nil
	}

	if len(contacts) == 0 {
		u.Err().Println("No results")
		return failEmptyExit(c.FailEmpty)
	}

	w, flush := tableWriter(ctx)
	defer flush()
	fmt.Fprintln(w, "RESOURCE\tNAME\tEMAIL\tPHONE")
	for _, p := range contacts {
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
	printNextPageHint(u, nextPageToken)
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
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\n",
			p.ResourceName,
			sanitizeTab(primaryName(p)),
			sanitizeTab(primaryEmail(p)),
			sanitizeTab(primaryPhone(p)),
		)
	}
	return nil
}

type ContactsOtherDeleteCmd struct {
	ResourceName string `arg:"" name:"resourceName" help:"Resource name (otherContacts/...)"`
}

func (c *ContactsOtherDeleteCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	resourceName := strings.TrimSpace(c.ResourceName)
	if !strings.HasPrefix(resourceName, "otherContacts/") {
		return usage("resourceName must start with otherContacts/")
	}

	if confirmErr := confirmDestructive(ctx, flags, fmt.Sprintf("delete other contact %s", resourceName)); confirmErr != nil {
		return confirmErr
	}

	account, err := requireAccount(flags)
	if err != nil {
		return err
	}

	if err := deleteOtherContact(ctx, account, resourceName); err != nil {
		return err
	}
	return writeDeleteResult(ctx, u, resourceName)
}

func deleteOtherContact(ctx context.Context, account, resourceName string) error {
	otherSvc, err := newPeopleOtherContactsService(ctx, account)
	if err != nil {
		return err
	}
	copied, err := otherSvc.OtherContacts.CopyOtherContactToMyContactsGroup(
		resourceName,
		&people.CopyOtherContactToMyContactsGroupRequest{},
	).Do()
	if err != nil {
		return fmt.Errorf("copy to my contacts: %w", err)
	}
	copiedResource := ""
	if copied != nil {
		copiedResource = strings.TrimSpace(copied.ResourceName)
	}
	if copiedResource == "" {
		return fmt.Errorf("copy to my contacts: empty resource name")
	}

	contactsSvc, err := newPeopleContactsService(ctx, account)
	if err != nil {
		return err
	}
	if _, err := contactsSvc.People.DeleteContact(copiedResource).Do(); err != nil {
		return fmt.Errorf("delete copied contact %s: %w", copiedResource, err)
	}
	return nil
}
