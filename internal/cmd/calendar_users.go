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

const calendarUsersRequestTimeout = 20 * time.Second

type CalendarUsersCmd struct {
	Max  int64  `name:"max" aliases:"limit" help:"Max results" default:"100"`
	Page string `name:"page" help:"Page token"`
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

	ctxTimeout, cancel := context.WithTimeout(ctx, calendarUsersRequestTimeout)
	defer cancel()

	resp, err := svc.People.ListDirectoryPeople().
		Sources("DIRECTORY_SOURCE_TYPE_DOMAIN_PROFILE").
		ReadMask("names,emailAddresses").
		PageSize(c.Max).
		PageToken(c.Page).
		Context(ctxTimeout).
		Do()
	if err != nil {
		if strings.Contains(err.Error(), "accessNotConfigured") ||
			strings.Contains(err.Error(), "People API has not been used") {
			return fmt.Errorf("people API is not enabled; enable it at: https://console.developers.google.com/apis/api/people.googleapis.com/overview (%w)", err)
		}
		return err
	}

	if outfmt.IsJSON(ctx) {
		type item struct {
			Email string `json:"email"`
			Name  string `json:"name,omitempty"`
		}
		items := make([]item, 0, len(resp.People))
		for _, p := range resp.People {
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
		return outfmt.WriteJSON(os.Stdout, map[string]any{
			"users":         items,
			"nextPageToken": resp.NextPageToken,
		})
	}

	if len(resp.People) == 0 {
		u.Err().Println("No workspace users found")
		return nil
	}

	w, flush := tableWriter(ctx)
	defer flush()
	fmt.Fprintln(w, "EMAIL\tNAME")
	for _, p := range resp.People {
		if p == nil {
			continue
		}
		email := primaryEmail(p)
		if email == "" {
			continue
		}
		fmt.Fprintf(w, "%s\t%s\n",
			sanitizeTab(email),
			sanitizeTab(primaryName(p)),
		)
	}
	printNextPageHint(u, resp.NextPageToken)

	u.Err().Println("\nTip: Use any email above as a calendar ID, e.g.:")
	u.Err().Printf("  gog calendar events %s", primaryEmail(resp.People[0]))

	return nil
}
