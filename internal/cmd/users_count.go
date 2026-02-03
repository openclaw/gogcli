package cmd

import (
	"context"
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/steipete/gogcli/internal/outfmt"
	"github.com/steipete/gogcli/internal/ui"
)

type UsersCountCmd struct {
	Domain  string `name:"domain" short:"d" help:"Domain to count users from"`
	OrgUnit string `name:"org-unit" aliases:"ou" help:"Organizational unit path to filter"`
}

func (c *UsersCountCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}

	svc, err := newAdminDirectory(ctx, account)
	if err != nil {
		return err
	}

	domain := strings.TrimSpace(c.Domain)
	if domain == "" {
		domain = extractDomain(account)
	}

	call := svc.Users.List().Domain(domain).MaxResults(500).Projection("basic").Fields("users(orgUnitPath),nextPageToken")
	if c.OrgUnit != "" {
		call = call.Query(fmt.Sprintf("orgUnitPath='%s'", c.OrgUnit))
	}

	counts := make(map[string]int)
	for {
		resp, err := call.Context(ctx).Do()
		if err != nil {
			return fmt.Errorf("list users: %w", err)
		}
		for _, user := range resp.Users {
			if user == nil {
				continue
			}
			path := user.OrgUnitPath
			if path == "" {
				path = "/"
			}
			counts[path]++
		}
		if resp.NextPageToken == "" {
			break
		}
		call = call.PageToken(resp.NextPageToken)
	}

	if outfmt.IsJSON(ctx) {
		type item struct {
			OrgUnitPath string `json:"orgUnitPath"`
			Count       int    `json:"count"`
		}
		items := make([]item, 0, len(counts))
		for path, count := range counts {
			items = append(items, item{OrgUnitPath: path, Count: count})
		}
		sort.Slice(items, func(i, j int) bool { return items[i].OrgUnitPath < items[j].OrgUnitPath })
		return outfmt.WriteJSON(os.Stdout, map[string]any{"counts": items})
	}

	if len(counts) == 0 {
		u.Err().Println("No users found")
		return nil
	}

	paths := make([]string, 0, len(counts))
	for path := range counts {
		paths = append(paths, path)
	}
	sort.Strings(paths)

	tw, flush := tableWriter(ctx)
	defer flush()
	fmt.Fprintln(tw, "ORG UNIT\tCOUNT")
	for _, path := range paths {
		fmt.Fprintf(tw, "%s\t%d\n", sanitizeTab(path), counts[path])
	}

	return nil
}
