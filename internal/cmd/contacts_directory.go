package cmd

import (
	"context"
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
	if c.Max <= 0 {
		return usage("max must be > 0")
	}
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}

	svc, err := peopleDirectoryService(ctx, account)
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

		resp, callErr := call.Do()
		if callErr != nil {
			return nil, "", callErr
		}
		return resp.People, resp.NextPageToken, nil
	}

	peopleList, nextPageToken, err := loadPagedItems(c.Page, c.All, fetch)
	if err != nil {
		return err
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
		if err := outfmt.WriteJSON(ctx, stdoutWriter(ctx), map[string]any{
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

	if err := outfmt.WriteTable(
		ctx,
		stdoutWriter(ctx),
		compactPeopleRows(peopleList),
		directoryPersonColumns(),
	); err != nil {
		return err
	}
	printNextPageHintWithAll(u, nextPageToken, "--all/--all-pages")
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
	query := strings.Join(c.Query, " ")
	if c.Max <= 0 {
		return usage("max must be > 0")
	}
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}

	svc, err := peopleDirectoryService(ctx, account)
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
		resp, callErr := call.Do()
		if callErr != nil {
			return nil, "", callErr
		}
		return resp.People, resp.NextPageToken, nil
	}

	peopleList, nextPageToken, err := loadPagedItems(c.Page, c.All, fetch)
	if err != nil {
		return err
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
		if err := outfmt.WriteJSON(ctx, stdoutWriter(ctx), map[string]any{
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

	if err := outfmt.WriteTable(
		ctx,
		stdoutWriter(ctx),
		compactPeopleRows(peopleList),
		directoryPersonColumns(),
	); err != nil {
		return err
	}
	printNextPageHintWithAll(u, nextPageToken, "--all/--all-pages")
	return nil
}

type ContactsOtherCmd struct {
	List   ContactsOtherListCmd   `cmd:"" name:"list" help:"List other contacts"`
	Search ContactsOtherSearchCmd `cmd:"" name:"search" help:"Search other contacts"`
}

const contactsOtherReadMask = "names,emailAddresses,phoneNumbers"

type ContactsOtherListCmd struct {
	Max       int64  `name:"max" aliases:"limit" help:"Max results" default:"100"`
	Page      string `name:"page" aliases:"cursor" help:"Page token"`
	All       bool   `name:"all" aliases:"all-pages,allpages" help:"Fetch all pages"`
	FailEmpty bool   `name:"fail-empty" aliases:"non-empty,require-results" help:"Exit with code 3 if no results"`
}

func (c *ContactsOtherListCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	if c.Max <= 0 {
		return usage("max must be > 0")
	}
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}

	svc, err := peopleOtherContactsService(ctx, account)
	if err != nil {
		return err
	}

	fetch := func(pageToken string) ([]*people.Person, string, error) {
		call := svc.OtherContacts.List().
			ReadMask(contactsOtherReadMask).
			PageSize(c.Max).
			Context(ctx)
		if strings.TrimSpace(pageToken) != "" {
			call = call.PageToken(pageToken)
		}
		resp, callErr := call.Do()
		if callErr != nil {
			return nil, "", callErr
		}
		return resp.OtherContacts, resp.NextPageToken, nil
	}

	contacts, nextPageToken, err := loadPagedItems(c.Page, c.All, fetch)
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
		if err := outfmt.WriteJSON(ctx, stdoutWriter(ctx), map[string]any{
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

	if err := outfmt.WriteTable(
		ctx,
		stdoutWriter(ctx),
		compactPeopleRows(contacts),
		otherContactColumns(),
	); err != nil {
		return err
	}
	printNextPageHintWithAll(u, nextPageToken, "--all/--all-pages")
	return nil
}

type ContactsOtherSearchCmd struct {
	Query []string `arg:"" name:"query" help:"Search query"`
	Max   int64    `name:"max" aliases:"limit" help:"Max results" default:"50"`
}

func (c *ContactsOtherSearchCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	query := strings.Join(c.Query, " ")
	if c.Max <= 0 {
		return usage("max must be > 0")
	}
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}

	svc, err := peopleOtherContactsService(ctx, account)
	if err != nil {
		return err
	}

	warmSearchOtherContactsCache(ctx, svc)
	resp, err := svc.OtherContacts.Search().
		Query(query).
		ReadMask(contactsOtherReadMask).
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
		return outfmt.WriteJSON(ctx, stdoutWriter(ctx), map[string]any{"contacts": items})
	}

	if len(resp.Results) == 0 {
		u.Err().Println("No results")
		return nil
	}

	return outfmt.WriteTable(ctx, stdoutWriter(ctx), contactSearchRows(resp.Results), otherContactColumns())
}
