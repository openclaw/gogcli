package cmd

import (
	"context"
	"fmt"
	"os"
	"strings"

	admin "google.golang.org/api/admin/directory/v1"

	"github.com/steipete/gogcli/internal/outfmt"
	"github.com/steipete/gogcli/internal/ui"
)

// AdminGroupsCmd manages Workspace groups.
type AdminGroupsCmd struct {
	List    AdminGroupsListCmd    `cmd:"" name:"list" aliases:"ls" help:"List groups in a domain"`
	Members AdminGroupsMembersCmd `cmd:"" name:"members" help:"Manage group members"`
}

type AdminGroupsListCmd struct {
	Domain    string `name:"domain" help:"Domain to list groups from (e.g., example.com)"`
	Max       int64  `name:"max" aliases:"limit" help:"Max results" default:"100"`
	Page      string `name:"page" aliases:"cursor" help:"Page token"`
	All       bool   `name:"all" aliases:"all-pages,allpages" help:"Fetch all pages"`
	FailEmpty bool   `name:"fail-empty" aliases:"non-empty,require-results" help:"Exit with code 3 if no results"`
}

func (c *AdminGroupsListCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	account, err := requireAdminAccount(flags)
	if err != nil {
		return err
	}

	domain := strings.TrimSpace(c.Domain)
	if domain == "" {
		return usage("domain required (e.g., --domain example.com)")
	}

	svc, err := newAdminDirectoryService(ctx, account)
	if err != nil {
		return wrapAdminDirectoryError(err, account)
	}

	fetch := func(pageToken string) ([]*admin.Group, string, error) {
		call := svc.Groups.List().
			Domain(domain).
			MaxResults(c.Max).
			Context(ctx)
		if strings.TrimSpace(pageToken) != "" {
			call = call.PageToken(pageToken)
		}
		resp, fetchErr := call.Do()
		if fetchErr != nil {
			return nil, "", wrapAdminDirectoryError(fetchErr, account)
		}
		return resp.Groups, resp.NextPageToken, nil
	}

	var groups []*admin.Group
	nextPageToken := ""
	if c.All {
		all, collectErr := collectAllPages(c.Page, fetch)
		if collectErr != nil {
			return collectErr
		}
		groups = all
	} else {
		groups, nextPageToken, err = fetch(c.Page)
		if err != nil {
			return err
		}
	}

	if outfmt.IsJSON(ctx) {
		type item struct {
			Email              string `json:"email"`
			Name               string `json:"name,omitempty"`
			Description        string `json:"description,omitempty"`
			DirectMembersCount int64  `json:"directMembersCount"`
		}
		items := make([]item, 0, len(groups))
		for _, group := range groups {
			if group == nil {
				continue
			}
			items = append(items, item{
				Email:              group.Email,
				Name:               group.Name,
				Description:        group.Description,
				DirectMembersCount: group.DirectMembersCount,
			})
		}
		if err := outfmt.WriteJSON(ctx, os.Stdout, map[string]any{
			"groups":        items,
			"nextPageToken": nextPageToken,
		}); err != nil {
			return err
		}
		if len(items) == 0 {
			return failEmptyExit(c.FailEmpty)
		}
		return nil
	}

	if len(groups) == 0 {
		u.Err().Println("No groups found")
		return failEmptyExit(c.FailEmpty)
	}

	w, flush := tableWriter(ctx)
	defer flush()
	fmt.Fprintln(w, "EMAIL\tNAME\tMEMBERS\tDESCRIPTION")
	for _, group := range groups {
		if group == nil {
			continue
		}
		desc := group.Description
		if len(desc) > 50 {
			desc = desc[:47] + "..."
		}
		fmt.Fprintf(w, "%s\t%s\t%d\t%s\n",
			sanitizeTab(group.Email),
			sanitizeTab(group.Name),
			group.DirectMembersCount,
			sanitizeTab(desc),
		)
	}
	printNextPageHint(u, nextPageToken)
	return nil
}

type AdminGroupsMembersCmd struct {
	List   AdminGroupsMembersListCmd   `cmd:"" name:"list" aliases:"ls" help:"List group members"`
	Add    AdminGroupsMembersAddCmd    `cmd:"" name:"add" aliases:"invite" help:"Add a member to a group"`
	Remove AdminGroupsMembersRemoveCmd `cmd:"" name:"remove" aliases:"rm,del,delete" help:"Remove a member from a group"`
}

type AdminGroupsMembersListCmd struct {
	GroupEmail string `arg:"" name:"groupEmail" help:"Group email (e.g., engineering@example.com)"`
	Max        int64  `name:"max" aliases:"limit" help:"Max results" default:"100"`
	Page       string `name:"page" aliases:"cursor" help:"Page token"`
	All        bool   `name:"all" aliases:"all-pages,allpages" help:"Fetch all pages"`
	FailEmpty  bool   `name:"fail-empty" aliases:"non-empty,require-results" help:"Exit with code 3 if no results"`
}

func (c *AdminGroupsMembersListCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	account, err := requireAdminAccount(flags)
	if err != nil {
		return err
	}

	groupEmail := strings.TrimSpace(c.GroupEmail)
	if groupEmail == "" {
		return usage("group email required")
	}

	svc, err := newAdminDirectoryService(ctx, account)
	if err != nil {
		return wrapAdminDirectoryError(err, account)
	}

	fetch := func(pageToken string) ([]*admin.Member, string, error) {
		call := svc.Members.List(groupEmail).
			MaxResults(c.Max).
			Context(ctx)
		if strings.TrimSpace(pageToken) != "" {
			call = call.PageToken(pageToken)
		}
		resp, fetchErr := call.Do()
		if fetchErr != nil {
			return nil, "", wrapAdminDirectoryError(fetchErr, account)
		}
		return resp.Members, resp.NextPageToken, nil
	}

	var members []*admin.Member
	nextPageToken := ""
	if c.All {
		all, collectErr := collectAllPages(c.Page, fetch)
		if collectErr != nil {
			return collectErr
		}
		members = all
	} else {
		members, nextPageToken, err = fetch(c.Page)
		if err != nil {
			return err
		}
	}

	if outfmt.IsJSON(ctx) {
		type item struct {
			Email string `json:"email"`
			Role  string `json:"role"`
			Type  string `json:"type"`
		}
		items := make([]item, 0, len(members))
		for _, member := range members {
			if member == nil {
				continue
			}
			items = append(items, item{
				Email: member.Email,
				Role:  member.Role,
				Type:  member.Type,
			})
		}
		if err := outfmt.WriteJSON(ctx, os.Stdout, map[string]any{
			"members":       items,
			"nextPageToken": nextPageToken,
		}); err != nil {
			return err
		}
		if len(items) == 0 {
			return failEmptyExit(c.FailEmpty)
		}
		return nil
	}

	if len(members) == 0 {
		u.Err().Println("No members found")
		return failEmptyExit(c.FailEmpty)
	}

	w, flush := tableWriter(ctx)
	defer flush()
	fmt.Fprintln(w, "EMAIL\tROLE\tTYPE")
	for _, member := range members {
		if member == nil {
			continue
		}
		fmt.Fprintf(w, "%s\t%s\t%s\n",
			sanitizeTab(member.Email),
			sanitizeTab(member.Role),
			sanitizeTab(member.Type),
		)
	}
	printNextPageHint(u, nextPageToken)
	return nil
}

type AdminGroupsMembersAddCmd struct {
	GroupEmail  string `arg:"" name:"groupEmail" help:"Group email"`
	MemberEmail string `arg:"" name:"memberEmail" help:"Member email to add"`
	Role        string `name:"role" help:"Member role (MEMBER, MANAGER, OWNER)" default:"MEMBER"`
}

func (c *AdminGroupsMembersAddCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	account, err := requireAdminAccount(flags)
	if err != nil {
		return err
	}

	groupEmail := strings.TrimSpace(c.GroupEmail)
	memberEmail := strings.TrimSpace(c.MemberEmail)
	if groupEmail == "" || memberEmail == "" {
		return usage("group email and member email required")
	}

	role := strings.ToUpper(c.Role)
	if role != adminRoleMember && role != adminRoleManager && role != adminRoleOwner {
		return usage("role must be MEMBER, MANAGER, or OWNER")
	}

	member := &admin.Member{
		Email: memberEmail,
		Role:  role,
	}

	if dryRunErr := dryRunExit(ctx, flags, fmt.Sprintf("add %s to %s as %s", memberEmail, groupEmail, role), member); dryRunErr != nil {
		return dryRunErr
	}

	svc, err := newAdminDirectoryService(ctx, account)
	if err != nil {
		return wrapAdminDirectoryError(err, account)
	}

	created, err := svc.Members.Insert(groupEmail, member).Context(ctx).Do()
	if err != nil {
		return wrapAdminDirectoryError(err, account)
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(ctx, os.Stdout, map[string]any{
			"email": created.Email,
			"role":  created.Role,
		})
	}

	u.Out().Printf("Added %s to %s as %s", created.Email, groupEmail, created.Role)
	return nil
}

type AdminGroupsMembersRemoveCmd struct {
	GroupEmail  string `arg:"" name:"groupEmail" help:"Group email"`
	MemberEmail string `arg:"" name:"memberEmail" help:"Member email to remove"`
}

func (c *AdminGroupsMembersRemoveCmd) Run(ctx context.Context, flags *RootFlags) error {
	account, err := requireAdminAccount(flags)
	if err != nil {
		return err
	}

	groupEmail := strings.TrimSpace(c.GroupEmail)
	memberEmail := strings.TrimSpace(c.MemberEmail)
	if groupEmail == "" || memberEmail == "" {
		return usage("group email and member email required")
	}

	if confirmErr := confirmDestructive(ctx, flags, fmt.Sprintf("remove %s from %s", memberEmail, groupEmail)); confirmErr != nil {
		return confirmErr
	}

	svc, err := newAdminDirectoryService(ctx, account)
	if err != nil {
		return wrapAdminDirectoryError(err, account)
	}

	if err := svc.Members.Delete(groupEmail, memberEmail).Context(ctx).Do(); err != nil {
		return wrapAdminDirectoryError(err, account)
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(ctx, os.Stdout, map[string]any{
			"removed": true,
			"email":   memberEmail,
			"group":   groupEmail,
		})
	}

	u := ui.FromContext(ctx)
	u.Out().Printf("Removed %s from %s", memberEmail, groupEmail)
	return nil
}
