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

// AdminUsersCmd manages Workspace users.
type AdminUsersCmd struct {
	List    AdminUsersListCmd    `cmd:"" name:"list" aliases:"ls" help:"List users in a domain"`
	Get     AdminUsersGetCmd     `cmd:"" name:"get" aliases:"info,show" help:"Get user details"`
	Create  AdminUsersCreateCmd  `cmd:"" name:"create" aliases:"add,new" help:"Create a new user"`
	Suspend AdminUsersSuspendCmd `cmd:"" name:"suspend" help:"Suspend a user account"`
}

type AdminUsersListCmd struct {
	Domain    string `name:"domain" help:"Domain to list users from (e.g., example.com)"`
	Max       int64  `name:"max" aliases:"limit" help:"Max results" default:"100"`
	Page      string `name:"page" aliases:"cursor" help:"Page token"`
	All       bool   `name:"all" aliases:"all-pages,allpages" help:"Fetch all pages"`
	FailEmpty bool   `name:"fail-empty" aliases:"non-empty,require-results" help:"Exit with code 3 if no results"`
}

func (c *AdminUsersListCmd) Run(ctx context.Context, flags *RootFlags) error {
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

	fetch := func(pageToken string) ([]*admin.User, string, error) {
		call := svc.Users.List().
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
		return resp.Users, resp.NextPageToken, nil
	}

	users, nextPageToken, err := loadPagedItems(c.Page, c.All, fetch)
	if err != nil {
		return err
	}

	if outfmt.IsJSON(ctx) {
		type item struct {
			Email     string `json:"email"`
			Name      string `json:"name,omitempty"`
			Suspended bool   `json:"suspended"`
			Admin     bool   `json:"admin"`
		}
		items := make([]item, 0, len(users))
		for _, user := range users {
			if user == nil {
				continue
			}
			name := ""
			if user.Name != nil {
				name = user.Name.FullName
			}
			items = append(items, item{
				Email:     user.PrimaryEmail,
				Name:      name,
				Suspended: user.Suspended,
				Admin:     user.IsAdmin,
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

	if len(users) == 0 {
		u.Err().Println("No users found")
		return failEmptyExit(c.FailEmpty)
	}

	w, flush := tableWriter(ctx)
	defer flush()
	fmt.Fprintln(w, "EMAIL\tNAME\tSUSPENDED\tADMIN")
	for _, user := range users {
		if user == nil {
			continue
		}
		suspended := "no"
		if user.Suspended {
			suspended = "yes"
		}
		isAdmin := "no"
		if user.IsAdmin {
			isAdmin = "yes"
		}
		name := ""
		if user.Name != nil {
			name = user.Name.FullName
		}
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\n",
			sanitizeTab(user.PrimaryEmail),
			sanitizeTab(name),
			suspended,
			isAdmin,
		)
	}
	printNextPageHint(u, nextPageToken)
	return nil
}

type AdminUsersGetCmd struct {
	UserEmail string `arg:"" name:"userEmail" help:"User email (e.g., user@example.com)"`
}

func (c *AdminUsersGetCmd) Run(ctx context.Context, flags *RootFlags) error {
	account, err := requireAdminAccount(flags)
	if err != nil {
		return err
	}

	userEmail := strings.TrimSpace(c.UserEmail)
	if userEmail == "" {
		return usage("user email required")
	}

	svc, err := newAdminDirectoryService(ctx, account)
	if err != nil {
		return wrapAdminDirectoryError(err, account)
	}

	user, err := svc.Users.Get(userEmail).Context(ctx).Do()
	if err != nil {
		return wrapAdminDirectoryError(err, account)
	}

	if outfmt.IsJSON(ctx) {
		type item struct {
			Email       string   `json:"email"`
			Name        string   `json:"name,omitempty"`
			GivenName   string   `json:"givenName,omitempty"`
			FamilyName  string   `json:"familyName,omitempty"`
			Suspended   bool     `json:"suspended"`
			Admin       bool     `json:"admin"`
			Aliases     []string `json:"aliases,omitempty"`
			OrgUnitPath string   `json:"orgUnitPath,omitempty"`
			Creation    string   `json:"creationTime,omitempty"`
			LastLogin   string   `json:"lastLoginTime,omitempty"`
		}
		var aliases []string
		if user.Aliases != nil {
			aliases = user.Aliases
		}
		name := ""
		givenName := ""
		familyName := ""
		if user.Name != nil {
			name = user.Name.FullName
			givenName = user.Name.GivenName
			familyName = user.Name.FamilyName
		}
		return outfmt.WriteJSON(ctx, os.Stdout, item{
			Email:       user.PrimaryEmail,
			Name:        name,
			GivenName:   givenName,
			FamilyName:  familyName,
			Suspended:   user.Suspended,
			Admin:       user.IsAdmin,
			Aliases:     aliases,
			OrgUnitPath: user.OrgUnitPath,
			Creation:    user.CreationTime,
			LastLogin:   user.LastLoginTime,
		})
	}

	w, flush := tableWriter(ctx)
	defer flush()
	fmt.Fprintf(w, "Email:\t%s\n", user.PrimaryEmail)
	if user.Name != nil {
		fmt.Fprintf(w, "Name:\t%s\n", user.Name.FullName)
		fmt.Fprintf(w, "Given Name:\t%s\n", user.Name.GivenName)
		fmt.Fprintf(w, "Family Name:\t%s\n", user.Name.FamilyName)
	}
	fmt.Fprintf(w, "Suspended:\t%t\n", user.Suspended)
	fmt.Fprintf(w, "Admin:\t%t\n", user.IsAdmin)
	fmt.Fprintf(w, "Org Unit:\t%s\n", user.OrgUnitPath)
	fmt.Fprintf(w, "Created:\t%s\n", user.CreationTime)
	fmt.Fprintf(w, "Last Login:\t%s\n", user.LastLoginTime)
	if len(user.Aliases) > 0 {
		fmt.Fprintf(w, "Aliases:\t%s\n", strings.Join(user.Aliases, ", "))
	}
	return nil
}

type AdminUsersCreateCmd struct {
	Email      string `arg:"" name:"email" help:"User email (e.g., user@example.com)"`
	GivenName  string `name:"given" help:"Given (first) name"`
	FamilyName string `name:"family" help:"Family (last) name"`
	Password   string `name:"password" help:"Initial password"`
	ChangePwd  bool   `name:"change-password" help:"Require password change on first login"`
	OrgUnit    string `name:"org-unit" help:"Organization unit path"`
	Admin      bool   `name:"admin" help:"Not supported; assign admin roles separately after user creation"`
}

func (c *AdminUsersCreateCmd) Run(ctx context.Context, flags *RootFlags) error {
	account, err := requireAdminAccount(flags)
	if err != nil {
		return err
	}

	email := strings.TrimSpace(c.Email)
	givenName := strings.TrimSpace(c.GivenName)
	familyName := strings.TrimSpace(c.FamilyName)
	password := strings.TrimSpace(c.Password)
	if email == "" {
		return usage("email required")
	}
	if givenName == "" {
		return usage("--given required")
	}
	if familyName == "" {
		return usage("--family required")
	}
	if password == "" {
		return usage("--password required")
	}
	if c.Admin {
		return usage("--admin is not supported; assign admin roles separately after user creation")
	}

	user := &admin.User{
		PrimaryEmail: email,
		Name: &admin.UserName{
			GivenName:  givenName,
			FamilyName: familyName,
		},
		Password:                  password,
		ChangePasswordAtNextLogin: c.ChangePwd,
	}
	if c.OrgUnit != "" {
		user.OrgUnitPath = c.OrgUnit
	}

	if dryRunErr := dryRunExit(ctx, flags, "create user", user); dryRunErr != nil {
		return dryRunErr
	}

	svc, err := newAdminDirectoryService(ctx, account)
	if err != nil {
		return wrapAdminDirectoryError(err, account)
	}

	created, err := svc.Users.Insert(user).Context(ctx).Do()
	if err != nil {
		return wrapAdminDirectoryError(err, account)
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(ctx, os.Stdout, map[string]any{
			"email": created.PrimaryEmail,
			"id":    created.Id,
		})
	}

	u := ui.FromContext(ctx)
	u.Out().Printf("Created user: %s (ID: %s)", created.PrimaryEmail, created.Id)
	return nil
}

type AdminUsersSuspendCmd struct {
	UserEmail string `arg:"" name:"userEmail" help:"User email to suspend"`
}

func (c *AdminUsersSuspendCmd) Run(ctx context.Context, flags *RootFlags) error {
	account, err := requireAdminAccount(flags)
	if err != nil {
		return err
	}

	userEmail := strings.TrimSpace(c.UserEmail)
	if userEmail == "" {
		return usage("user email required")
	}

	if confirmErr := confirmDestructive(ctx, flags, fmt.Sprintf("suspend user %s", userEmail)); confirmErr != nil {
		return confirmErr
	}

	svc, err := newAdminDirectoryService(ctx, account)
	if err != nil {
		return wrapAdminDirectoryError(err, account)
	}

	updated, err := svc.Users.Update(userEmail, &admin.User{Suspended: true}).Context(ctx).Do()
	if err != nil {
		return wrapAdminDirectoryError(err, account)
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(ctx, os.Stdout, map[string]any{
			"email":     updated.PrimaryEmail,
			"suspended": updated.Suspended,
		})
	}

	u := ui.FromContext(ctx)
	u.Out().Printf("Suspended user: %s", updated.PrimaryEmail)
	return nil
}
