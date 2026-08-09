package cmd

import (
	"strconv"

	admin "google.golang.org/api/admin/directory/v1"

	"github.com/openclaw/gogcli/internal/outfmt"
)

func adminUserColumns() []outfmt.Column[*admin.User] {
	return []outfmt.Column[*admin.User]{
		{Header: "EMAIL", Value: func(user *admin.User) string {
			return sanitizeTab(user.PrimaryEmail)
		}},
		{Header: "NAME", Value: func(user *admin.User) string {
			if user.Name == nil {
				return ""
			}
			return sanitizeTab(user.Name.FullName)
		}},
		{Header: "SUSPENDED", Value: func(user *admin.User) string {
			return adminYesNo(user.Suspended)
		}},
		{Header: "ADMIN", Value: func(user *admin.User) string {
			return adminYesNo(user.IsAdmin)
		}},
	}
}

func adminGroupColumns() []outfmt.Column[*admin.Group] {
	return []outfmt.Column[*admin.Group]{
		{Header: "EMAIL", Value: func(group *admin.Group) string {
			return sanitizeTab(group.Email)
		}},
		{Header: "NAME", Value: func(group *admin.Group) string {
			return sanitizeTab(group.Name)
		}},
		{Header: "MEMBERS", Value: func(group *admin.Group) string {
			return strconv.FormatInt(group.DirectMembersCount, 10)
		}},
		{Header: "DESCRIPTION", Value: func(group *admin.Group) string {
			return sanitizeTab(truncateAdminDescription(group.Description))
		}},
	}
}

func adminMemberColumns() []outfmt.Column[*admin.Member] {
	return []outfmt.Column[*admin.Member]{
		{Header: "EMAIL", Value: func(member *admin.Member) string {
			return sanitizeTab(member.Email)
		}},
		{Header: "ROLE", Value: func(member *admin.Member) string {
			return sanitizeTab(member.Role)
		}},
		{Header: "TYPE", Value: func(member *admin.Member) string {
			return sanitizeTab(member.Type)
		}},
	}
}

func adminOrgUnitColumns() []outfmt.Column[*admin.OrgUnit] {
	return []outfmt.Column[*admin.OrgUnit]{
		sanitizedColumn("PATH", func(unit *admin.OrgUnit) string { return unit.OrgUnitPath }),
		sanitizedColumn("NAME", func(unit *admin.OrgUnit) string { return unit.Name }),
		sanitizedColumn("ID", func(unit *admin.OrgUnit) string { return unit.OrgUnitId }),
		sanitizedColumn("PARENT", func(unit *admin.OrgUnit) string { return unit.ParentOrgUnitPath }),
		sanitizedColumn("DESCRIPTION", func(unit *admin.OrgUnit) string { return unit.Description }),
	}
}

func nonNilAdminRows[T any](rows []*T) []*T {
	filtered := make([]*T, 0, len(rows))
	for _, row := range rows {
		if row != nil {
			filtered = append(filtered, row)
		}
	}
	return filtered
}

func truncateAdminDescription(description string) string {
	if len(description) <= 50 {
		return description
	}
	return description[:47] + "..."
}

func adminYesNo(value bool) string {
	if value {
		return "yes"
	}
	return "no"
}
