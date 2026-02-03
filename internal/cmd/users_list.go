package cmd

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/steipete/gogcli/internal/outfmt"
	"github.com/steipete/gogcli/internal/ui"
)

type UsersListCmd struct {
	Domain     string       `name:"domain" short:"d" help:"Domain to list users from"`
	Query      string       `name:"query" short:"q" help:"Search query (e.g., 'email:admin*', 'name:John*', 'orgUnitPath=/Sales')"`
	OrgUnit    string       `name:"org-unit" aliases:"ou" help:"Organizational unit path"`
	Max        int64        `name:"max" aliases:"limit" default:"100" help:"Maximum users to return"`
	Page       string       `name:"page" help:"Page token for pagination"`
	Suspended  *bool        `name:"suspended" help:"Filter by suspended state"`
	Admin      *bool        `name:"admin" help:"Filter by admin status"`
	OrderBy    string       `name:"order-by" default:"email" enum:"email,familyName,givenName" help:"Sort field"`
	SortOrder  string       `name:"sort-order" default:"ASCENDING" enum:"ASCENDING,DESCENDING" help:"Sort direction"`
	Projection string       `name:"projection" default:"basic" enum:"basic,full,custom" help:"Amount of user data to return"`
	Fields     string       `name:"fields" help:"Custom fields to return (comma-separated)"`
	ToDrive    ToDriveFlags `embed:""`
}

func (c *UsersListCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}

	svc, err := newAdminDirectory(ctx, account)
	if err != nil {
		return err
	}

	call := svc.Users.List()

	domain := strings.TrimSpace(c.Domain)
	if domain == "" {
		domain = extractDomain(account)
	}
	call = call.Domain(domain)

	var queryParts []string
	if c.Query != "" {
		queryParts = append(queryParts, c.Query)
	}
	if c.OrgUnit != "" {
		queryParts = append(queryParts, fmt.Sprintf("orgUnitPath='%s'", c.OrgUnit))
	}
	if c.Suspended != nil {
		queryParts = append(queryParts, fmt.Sprintf("isSuspended=%v", *c.Suspended))
	}
	if c.Admin != nil {
		queryParts = append(queryParts, fmt.Sprintf("isAdmin=%v", *c.Admin))
	}
	if len(queryParts) > 0 {
		call = call.Query(strings.Join(queryParts, " "))
	}

	call = call.MaxResults(c.Max)
	call = call.OrderBy(c.OrderBy)
	call = call.SortOrder(c.SortOrder)
	call = call.Projection(c.Projection)
	if c.Page != "" {
		call = call.PageToken(c.Page)
	}
	if c.Fields != "" {
		call = call.CustomFieldMask(c.Fields)
	}

	resp, err := call.Context(ctx).Do()
	if err != nil {
		return fmt.Errorf("list users: %w", err)
	}

	if len(resp.Users) == 0 {
		u.Err().Println("No users found")
		return nil
	}

	rows := make([][]string, 0, len(resp.Users))
	for _, user := range resp.Users {
		if user == nil {
			continue
		}
		suspended := ""
		if user.Suspended {
			suspended = "yes"
		}
		admin := ""
		if user.IsAdmin {
			admin = "yes"
		}
		name := ""
		if user.Name != nil {
			name = strings.TrimSpace(strings.Join([]string{user.Name.GivenName, user.Name.FamilyName}, " "))
		}
		rows = append(rows, toDriveRow(
			user.PrimaryEmail,
			name,
			suspended,
			admin,
			user.OrgUnitPath,
			formatDateTime(user.LastLoginTime),
		))
	}

	if ok, err := writeToDrive(ctx, flags, toDriveTitle("Users", c.ToDrive), []string{"EMAIL", "NAME", "SUSPENDED", "ADMIN", "ORG UNIT", "LAST LOGIN"}, rows, c.ToDrive); ok {
		return err
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(os.Stdout, resp)
	}

	tw, flush := tableWriter(ctx)
	defer flush()

	fmt.Fprintln(tw, "EMAIL\tNAME\tSUSPENDED\tADMIN\tORG UNIT\tLAST LOGIN")
	for _, row := range rows {
		if len(row) < 6 {
			continue
		}
		fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\t%s\n",
			sanitizeTab(row[0]),
			sanitizeTab(row[1]),
			sanitizeTab(row[2]),
			sanitizeTab(row[3]),
			sanitizeTab(row[4]),
			sanitizeTab(row[5]),
		)
	}

	printNextPageHint(u, resp.NextPageToken)
	return nil
}
