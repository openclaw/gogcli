package cmd

import (
	"context"
	"fmt"
	"strings"

	"google.golang.org/api/gmail/v1"
	"google.golang.org/api/people/v1"

	"github.com/steipete/gogcli/internal/outfmt"
	"github.com/steipete/gogcli/internal/ui"
)

type GmailSearchCmd struct {
	Query       []string `arg:"" name:"query" help:"Search query"`
	FromContact string   `name:"from-contact" help:"Resolve a Google Contact and add from:(email OR email) to the Gmail query"`
	Max         int64    `name:"max" aliases:"limit" help:"Max results" default:"10"`
	Page        string   `name:"page" aliases:"cursor" help:"Page token"`
	All         bool     `name:"all" aliases:"all-pages,allpages" help:"Fetch all pages"`
	FailEmpty   bool     `name:"fail-empty" aliases:"non-empty,require-results" help:"Exit with code 3 if no results"`
	Oldest      bool     `name:"oldest" help:"Show first message date instead of last"`
	Timezone    string   `name:"timezone" short:"z" help:"Output timezone (IANA name, e.g. America/New_York, UTC). Default: GOG_TIMEZONE, config, then local"`
	Local       bool     `name:"local" help:"Use local timezone (default behavior, useful to override --timezone)"`
}

func (c *GmailSearchCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	if err := validateGmailMaxResults(c.Max); err != nil {
		return err
	}
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}
	query := strings.TrimSpace(strings.Join(c.Query, " "))
	if strings.TrimSpace(c.FromContact) != "" {
		expanded, expandErr := gmailFromContactQuery(ctx, account, c.FromContact)
		if expandErr != nil {
			return expandErr
		}
		query = strings.TrimSpace(strings.Join([]string{query, expanded}, " "))
	}
	if query == "" {
		return usage("missing query")
	}

	svc, err := gmailService(ctx, account)
	if err != nil {
		return err
	}

	fetch := func(pageToken string) ([]*gmail.Thread, string, error) {
		opts := newGmailSearchRequestOptions(query, c.Max, pageToken)
		call := applyGmailThreadListOptions(svc.Users.Threads.List("me"), opts).Context(ctx)
		resp, callErr := call.Do()
		if callErr != nil {
			return nil, "", callErr
		}
		return resp.Threads, resp.NextPageToken, nil
	}

	threads, nextPageToken, err := loadPagedItems(c.Page, c.All, fetch)
	if err != nil {
		return err
	}

	if len(threads) == 0 {
		if outfmt.IsJSON(ctx) {
			return writePagedJSONResult(ctx, map[string]any{
				"threads":       []threadItem{},
				"nextPageToken": nextPageToken,
			}, 0, c.FailEmpty)
		}
		u.Err().Println("No results")
		return failEmptyExit(c.FailEmpty)
	}

	idToName, err := fetchLabelIDToName(svc)
	if err != nil {
		return err
	}

	loc, err := resolveOutputLocation(ctx, c.Timezone, c.Local, stderrWriter(ctx))
	if err != nil {
		return err
	}

	items, err := fetchThreadDetails(ctx, svc, threads, idToName, c.Oldest, loc)
	if err != nil {
		return err
	}

	if outfmt.IsJSON(ctx) {
		return writePagedJSONResult(ctx, map[string]any{
			"threads":       items,
			"nextPageToken": nextPageToken,
		}, len(items), c.FailEmpty)
	}

	if len(items) == 0 {
		u.Err().Println("No results")
		return failEmptyExit(c.FailEmpty)
	}

	if err := outfmt.WriteTable(ctx, stdoutWriter(ctx), items, gmailThreadColumns()); err != nil {
		return err
	}
	printNextPageHintWithAll(u, nextPageToken, "--all/--all-pages")
	return nil
}

func gmailFromContactQuery(ctx context.Context, account, selector string) (string, error) {
	selector = strings.TrimSpace(selector)
	if selector == "" {
		return "", usage("empty --from-contact")
	}
	svc, err := peopleContactsService(ctx, account)
	if err != nil {
		return "", err
	}
	warmSearchContactsCache(ctx, svc)
	resp, err := svc.People.SearchContacts().
		Query(selector).
		PageSize(10).
		ReadMask("names,emailAddresses").
		Context(ctx).
		Do()
	if err != nil {
		return "", fmt.Errorf("resolve --from-contact: %w", err)
	}

	matches := selectGmailFromContactPeople(selector, resp)
	if len(matches) == 0 {
		listed, listErr := listExactGmailFromContactPeople(ctx, svc, selector)
		if listErr != nil {
			return "", listErr
		}
		matches = listed
	}
	if len(matches) == 0 {
		return "", fmt.Errorf("resolve --from-contact: no contact found for %q", selector)
	}
	if len(matches) > 1 {
		names := make([]string, 0, len(matches))
		for _, p := range matches {
			names = append(names, firstNonEmpty(primaryName(p), primaryEmail(p), p.ResourceName))
		}
		return "", fmt.Errorf("resolve --from-contact: %q matched multiple contacts (%s); use a more specific name or email", selector, strings.Join(names, ", "))
	}
	emails := allContactEmails(matches[0])
	if len(emails) == 0 {
		return "", fmt.Errorf("resolve --from-contact: %q has no email addresses", selector)
	}
	return buildGmailFromEmailsQuery(emails), nil
}

func listExactGmailFromContactPeople(ctx context.Context, svc *people.Service, selector string) ([]*people.Person, error) {
	selectorLower := strings.ToLower(strings.TrimSpace(selector))
	var matches []*people.Person
	pageToken := ""
	for {
		call := svc.People.Connections.List(peopleMeResource).
			PersonFields("names,emailAddresses").
			PageSize(200).
			Context(ctx)
		if pageToken != "" {
			call = call.PageToken(pageToken)
		}
		resp, err := call.Do()
		if err != nil {
			return nil, fmt.Errorf("resolve --from-contact fallback list: %w", err)
		}
		for _, p := range resp.Connections {
			if p == nil {
				continue
			}
			if strings.ToLower(primaryName(p)) == selectorLower || contactHasEmail(p, selectorLower) {
				matches = append(matches, p)
			}
		}
		if resp.NextPageToken == "" {
			return matches, nil
		}
		pageToken = resp.NextPageToken
	}
}

func selectGmailFromContactPeople(selector string, resp *people.SearchResponse) []*people.Person {
	if resp == nil {
		return nil
	}
	selectorLower := strings.ToLower(strings.TrimSpace(selector))
	exact := make([]*people.Person, 0, len(resp.Results))
	fallback := make([]*people.Person, 0, len(resp.Results))
	for _, result := range resp.Results {
		if result == nil || result.Person == nil {
			continue
		}
		p := result.Person
		fallback = append(fallback, p)
		if strings.ToLower(primaryName(p)) == selectorLower || contactHasEmail(p, selectorLower) {
			exact = append(exact, p)
		}
	}
	if len(exact) > 0 {
		return exact
	}
	if len(fallback) == 1 {
		return fallback
	}
	return fallback
}

func contactHasEmail(p *people.Person, emailLower string) bool {
	for _, email := range p.EmailAddresses {
		if email != nil && strings.ToLower(strings.TrimSpace(email.Value)) == emailLower {
			return true
		}
	}
	return false
}

func allContactEmails(p *people.Person) []string {
	seen := map[string]bool{}
	var emails []string
	for _, email := range p.EmailAddresses {
		if email == nil {
			continue
		}
		value := strings.TrimSpace(email.Value)
		key := strings.ToLower(value)
		if value == "" || seen[key] {
			continue
		}
		seen[key] = true
		emails = append(emails, value)
	}
	return emails
}

func buildGmailFromEmailsQuery(emails []string) string {
	if len(emails) == 1 {
		return "from:" + emails[0]
	}
	return "from:(" + strings.Join(emails, " OR ") + ")"
}
