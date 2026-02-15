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

const calendarUsersRequestTimeout = 20 * time.Second

type CalendarUsersCmd struct {
	Max       int64  `name:"max" aliases:"limit" help:"Max results" default:"100"`
	Page      string `name:"page" aliases:"cursor" help:"Page token"`
	All       bool   `name:"all" aliases:"all-pages,allpages" help:"Fetch all pages"`
	FailEmpty bool   `name:"fail-empty" aliases:"non-empty,require-results" help:"Exit with code 3 if no results"`
}

func (c *CalendarUsersCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}

	svc, err := newPeopleDirectoryService(ctx, account)
	if err != nil {
		if strings.Contains(err.Error(), "accessNotConfigured") ||
			strings.Contains(err.Error(), "People API has not been used") {
			return fmt.Errorf("people API is not enabled; enable it at: https://console.developers.google.com/apis/api/people.googleapis.com/overview (%w)", err)
		}
		return err
	}

	fetch := func(pageToken string) ([]*people.Person, string, error) {
		ctxTimeout, cancel := context.WithTimeout(ctx, calendarUsersRequestTimeout)
		defer cancel()

		call := svc.People.ListDirectoryPeople().
			Sources("DIRECTORY_SOURCE_TYPE_DOMAIN_PROFILE").
			ReadMask("names,emailAddresses").
			PageSize(c.Max).
			Context(ctxTimeout)
		if strings.TrimSpace(pageToken) != "" {
			call = call.PageToken(pageToken)
		}
		resp, err := call.Do()
		if err != nil {
			if strings.Contains(err.Error(), "accessNotConfigured") ||
				strings.Contains(err.Error(), "People API has not been used") {
				return nil, "", fmt.Errorf("people API is not enabled; enable it at: https://console.developers.google.com/apis/api/people.googleapis.com/overview (%w)", err)
			}
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
			Email string `json:"email"`
			Name  string `json:"name,omitempty"`
		}
		items := make([]item, 0, len(peopleList))
		for _, p := range peopleList {
			if p == nil {
				continue
			}
			email := primaryEmail(p)
			if email == "" {
				continue
			}
			items = append(items, item{
				Email: email,
				Name:  primaryName(p),
			})
		}
		if err := outfmt.WriteJSON(ctx, os.Stdout, map[string]any{
			"users":         items,
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
		u.Err().Println("No workspace users found")
		return failEmptyExit(c.FailEmpty)
	}

	w, flush := tableWriter(ctx)
	defer flush()
	fmt.Fprintln(w, "EMAIL\tNAME")
	firstEmail := ""
	for _, p := range peopleList {
		if p == nil {
			continue
		}
		email := primaryEmail(p)
		if email == "" {
			continue
		}
		if firstEmail == "" {
			firstEmail = email
		}
		fmt.Fprintf(w, "%s\t%s\n",
			sanitizeTab(email),
			sanitizeTab(primaryName(p)),
		)
	}
	printNextPageHint(u, nextPageToken)

	u.Err().Println("\nTip: Use any email above as a calendar ID, e.g.:")
	if firstEmail != "" {
		u.Err().Printf("  gog calendar events %s", firstEmail)
	}

	return nil
}
