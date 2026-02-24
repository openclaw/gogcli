package cmd

import (
	"context"
	"fmt"
	"os"
	"sort"
	"strings"

	"google.golang.org/api/cloudidentity/v1"

	"github.com/steipete/gogcli/internal/errfmt"
	"github.com/steipete/gogcli/internal/googleapi"
	"github.com/steipete/gogcli/internal/outfmt"
	"github.com/steipete/gogcli/internal/ui"
)

var newCloudIdentityService = googleapi.NewCloudIdentityGroups

const (
	groupRoleOwner   = "OWNER"
	groupRoleManager = "MANAGER"
	groupRoleMember  = "MEMBER"
)

type GroupsCmd struct {
	List    GroupsListCmd    `cmd:"" name:"list" aliases:"ls" help:"List groups you belong to"`
	Members GroupsMembersCmd `cmd:"" name:"members" help:"List members of a group"`
}

type GroupsListCmd struct {
	Max       int64  `name:"max" aliases:"limit" help:"Max results" default:"100"`
	Page      string `name:"page" aliases:"cursor" help:"Page token"`
	All       bool   `name:"all" aliases:"all-pages,allpages" help:"Fetch all pages"`
	FailEmpty bool   `name:"fail-empty" aliases:"non-empty,require-results" help:"Exit with code 3 if no results"`
}

func (c *GroupsListCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}

	svc, err := newCloudIdentityService(ctx, account)
	if err != nil {
		return wrapCloudIdentityError(err, account)
	}

	// Search for all groups the user belongs to
	// Using "groups/-" as parent searches across all groups
	fetch := func(pageToken string) ([]*cloudidentity.GroupRelation, string, error) {
		call := svc.Groups.Memberships.SearchTransitiveGroups("groups/-").
			Query("member_key_id == '" + account + "'").
			PageSize(c.Max).
			Context(ctx)
		if strings.TrimSpace(pageToken) != "" {
			call = call.PageToken(pageToken)
		}
		resp, err := call.Do()
		if err != nil {
			return nil, "", wrapCloudIdentityError(err, account)
		}
		return resp.Memberships, resp.NextPageToken, nil
	}

	var memberships []*cloudidentity.GroupRelation
	nextPageToken := ""
	if c.All {
		all, err := collectAllPages(c.Page, fetch)
		if err != nil {
			return err
		}
		memberships = all
	} else {
		var err error
		memberships, nextPageToken, err = fetch(c.Page)
		if err != nil {
			return err
		}
	}

	if outfmt.IsJSON(ctx) {
		type item struct {
			GroupName   string `json:"groupName"`
			DisplayName string `json:"displayName,omitempty"`
			Role        string `json:"role,omitempty"`
		}
		items := make([]item, 0, len(memberships))
		for _, m := range memberships {
			if m == nil {
				continue
			}
			items = append(items, item{
				GroupName:   m.GroupKey.Id,
				DisplayName: m.DisplayName,
				Role:        getRelationType(m.RelationType),
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

	if len(memberships) == 0 {
		u.Err().Println("No groups found")
		return failEmptyExit(c.FailEmpty)
	}

	w, flush := tableWriter(ctx)
	defer flush()
	fmt.Fprintln(w, "GROUP\tNAME\tRELATION")
	for _, m := range memberships {
		if m == nil {
			continue
		}
		fmt.Fprintf(w, "%s\t%s\t%s\n",
			sanitizeTab(m.GroupKey.Id),
			sanitizeTab(m.DisplayName),
			sanitizeTab(getRelationType(m.RelationType)),
		)
	}
	printNextPageHint(u, nextPageToken)
	return nil
}

// wrapCloudIdentityError provides helpful error messages for common Cloud Identity API issues.
func wrapCloudIdentityError(err error, account string) error {
	errStr := err.Error()
	if strings.Contains(errStr, "accessNotConfigured") ||
		strings.Contains(errStr, "Cloud Identity API has not been used") {
		return errfmt.NewUserFacingError("Cloud Identity API is not enabled; enable it at: https://console.developers.google.com/apis/api/cloudidentity.googleapis.com/overview", err)
	}
	if strings.Contains(errStr, "insufficientPermissions") ||
		strings.Contains(errStr, "insufficient authentication scopes") {
		return errfmt.NewUserFacingError("Insufficient permissions for Cloud Identity API; re-authenticate with the cloud-identity.groups.readonly scope: gog auth add <account> --services groups", err)
	}
	if isConsumerAccount(account) && (strings.Contains(errStr, "invalid argument") || strings.Contains(errStr, "badRequest")) {
		return errfmt.NewUserFacingError("Cloud Identity groups require a Google Workspace/Cloud Identity account; consumer accounts (gmail.com/googlemail.com) are not supported.", err)
	}
	return err
}

// getRelationType returns a human-readable relation type.
func getRelationType(relationType string) string {
	switch relationType {
	case "DIRECT":
		return "direct"
	case "INDIRECT":
		return "indirect"
	default:
		return relationType
	}
}

type GroupsMembersCmd struct {
	GroupEmail string `arg:"" name:"groupEmail" help:"Group email (e.g., engineering@company.com)"`
	Max        int64  `name:"max" aliases:"limit" help:"Max results" default:"100"`
	Page       string `name:"page" aliases:"cursor" help:"Page token"`
	All        bool   `name:"all" aliases:"all-pages,allpages" help:"Fetch all pages"`
	FailEmpty  bool   `name:"fail-empty" aliases:"non-empty,require-results" help:"Exit with code 3 if no results"`
}

func (c *GroupsMembersCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}

	groupEmail := strings.TrimSpace(c.GroupEmail)
	if groupEmail == "" {
		return usage("group email required")
	}

	svc, err := newCloudIdentityService(ctx, account)
	if err != nil {
		return wrapCloudIdentityError(err, account)
	}

	// First, look up the group by email to get its resource name
	groupName, err := lookupGroupByEmail(ctx, svc, groupEmail)
	if err != nil {
		return fmt.Errorf("failed to find group %q: %w", groupEmail, err)
	}

	// List members of the group
	fetch := func(pageToken string) ([]*cloudidentity.Membership, string, error) {
		call := svc.Groups.Memberships.List(groupName).
			PageSize(c.Max).
			Context(ctx)
		if strings.TrimSpace(pageToken) != "" {
			call = call.PageToken(pageToken)
		}
		resp, err := call.Do()
		if err != nil {
			return nil, "", fmt.Errorf("failed to list members: %w", err)
		}
		return resp.Memberships, resp.NextPageToken, nil
	}

	var memberships []*cloudidentity.Membership
	nextPageToken := ""
	if c.All {
		all, err := collectAllPages(c.Page, fetch)
		if err != nil {
			return err
		}
		memberships = all
	} else {
		var err error
		memberships, nextPageToken, err = fetch(c.Page)
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
		items := make([]item, 0, len(memberships))
		for _, m := range memberships {
			if m == nil || m.PreferredMemberKey == nil {
				continue
			}
			items = append(items, item{
				Email: m.PreferredMemberKey.Id,
				Role:  getMemberRole(m.Roles),
				Type:  m.Type,
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

	if len(memberships) == 0 {
		u.Err().Printf("No members in group %s", groupEmail)
		return failEmptyExit(c.FailEmpty)
	}

	w, flush := tableWriter(ctx)
	defer flush()
	fmt.Fprintln(w, "EMAIL\tROLE\tTYPE")
	for _, m := range memberships {
		if m == nil || m.PreferredMemberKey == nil {
			continue
		}
		fmt.Fprintf(w, "%s\t%s\t%s\n",
			sanitizeTab(m.PreferredMemberKey.Id),
			sanitizeTab(getMemberRole(m.Roles)),
			sanitizeTab(m.Type),
		)
	}
	printNextPageHint(u, nextPageToken)
	return nil
}

// lookupGroupByEmail finds a group by its email address and returns its resource name.
func lookupGroupByEmail(ctx context.Context, svc *cloudidentity.Service, email string) (string, error) {
	resp, err := svc.Groups.Lookup().
		GroupKeyId(email).
		Context(ctx).
		Do()
	if err != nil {
		return "", err
	}
	return resp.Name, nil
}

// getMemberRole extracts the role from membership roles.
func getMemberRole(roles []*cloudidentity.MembershipRole) string {
	if len(roles) == 0 {
		return groupRoleMember
	}
	// Return the highest role (OWNER > MANAGER > MEMBER)
	for _, r := range roles {
		if r.Name == groupRoleOwner {
			return groupRoleOwner
		}
	}
	for _, r := range roles {
		if r.Name == groupRoleManager {
			return groupRoleManager
		}
	}
	return groupRoleMember
}

// truncate shortens a string to maxLen, adding "..." if truncated.
func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	if maxLen <= 3 {
		return s[:maxLen]
	}
	return s[:maxLen-3] + "..."
}

func collectGroupMemberEmails(ctx context.Context, svc *cloudidentity.Service, groupEmail string) ([]string, error) {
	seenGroups := make(map[string]bool)
	emails := make(map[string]bool)
	if err := collectGroupMemberEmailsRecursive(ctx, svc, groupEmail, seenGroups, emails); err != nil {
		return nil, err
	}

	results := make([]string, 0, len(emails))
	for email := range emails {
		results = append(results, email)
	}
	sort.Strings(results)
	return results, nil
}

func collectGroupMemberEmailsRecursive(ctx context.Context, svc *cloudidentity.Service, groupEmail string, seenGroups map[string]bool, emails map[string]bool) error {
	groupEmail = strings.TrimSpace(groupEmail)
	if groupEmail == "" {
		return nil
	}
	if seenGroups[groupEmail] {
		return nil
	}
	seenGroups[groupEmail] = true

	groupName, err := lookupGroupByEmail(ctx, svc, groupEmail)
	if err != nil {
		return fmt.Errorf("lookup group %q: %w", groupEmail, err)
	}

	memberships, err := listGroupMemberships(ctx, svc, groupName, 200)
	if err != nil {
		return fmt.Errorf("list members for %q: %w", groupEmail, err)
	}

	for _, m := range memberships {
		if m == nil || m.PreferredMemberKey == nil {
			continue
		}
		email := strings.TrimSpace(m.PreferredMemberKey.Id)
		if email == "" || !strings.Contains(email, "@") {
			continue
		}
		switch m.Type {
		case "GROUP":
			if err := collectGroupMemberEmailsRecursive(ctx, svc, email, seenGroups, emails); err != nil {
				return err
			}
		case "USER", "":
			emails[email] = true
		}
	}

	return nil
}

func listGroupMemberships(ctx context.Context, svc *cloudidentity.Service, groupName string, pageSize int64) ([]*cloudidentity.Membership, error) {
	fetch := func(pageToken string) ([]*cloudidentity.Membership, string, error) {
		call := svc.Groups.Memberships.List(groupName).PageSize(pageSize).Context(ctx)
		if strings.TrimSpace(pageToken) != "" {
			call = call.PageToken(pageToken)
		}
		resp, err := call.Do()
		if err != nil {
			return nil, "", err
		}
		return resp.Memberships, resp.NextPageToken, nil
	}
	return collectAllPages("", fetch)
}
