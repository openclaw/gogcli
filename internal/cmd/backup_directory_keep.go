package cmd

import (
	"context"
	"fmt"
	"sort"
	"strings"

	admin "google.golang.org/api/admin/directory/v1"
	"google.golang.org/api/cloudidentity/v1"
	keepapi "google.golang.org/api/keep/v1"

	"github.com/openclaw/gogcli/internal/backup"
	"github.com/openclaw/gogcli/internal/errfmt"
)

type groupsBackupMember struct {
	GroupEmail string                    `json:"groupEmail"`
	Member     *cloudidentity.Membership `json:"member,omitempty"`
	Error      string                    `json:"error,omitempty"`
}

type adminBackupMember struct {
	GroupEmail string        `json:"groupEmail"`
	Member     *admin.Member `json:"member,omitempty"`
	Error      string        `json:"error,omitempty"`
}

func buildGroupsBackupSnapshot(ctx context.Context, flags *RootFlags, shardMaxRows int) (backup.Snapshot, error) {
	account, err := requireGroupsAccount(flags)
	if err != nil {
		return backup.Snapshot{}, err
	}
	svc, err := cloudIdentityService(ctx, account)
	if err != nil {
		return backup.Snapshot{}, wrapCloudIdentityError(err, account)
	}
	accountHash := backupAccountHash(account)
	groups, err := fetchBackupCloudIdentityGroups(ctx, svc, account)
	if err != nil {
		return backup.Snapshot{}, err
	}
	members := fetchBackupCloudIdentityGroupMembers(ctx, svc, groups)
	groupShards, err := buildBackupShards(backupServiceGroups, "groups", accountHash, fmt.Sprintf("data/groups/%s/groups", accountHash), groups, shardMaxRows)
	if err != nil {
		return backup.Snapshot{}, err
	}
	memberShards, err := buildBackupShards(backupServiceGroups, "members", accountHash, fmt.Sprintf("data/groups/%s/members", accountHash), members, shardMaxRows)
	if err != nil {
		return backup.Snapshot{}, err
	}
	return backup.Snapshot{
		Services: []string{backupServiceGroups},
		Accounts: []string{accountHash},
		Counts: map[string]int{
			"groups.groups":  len(groups),
			"groups.members": len(members),
		},
		Shards: append(groupShards, memberShards...),
	}, nil
}

func buildAdminBackupSnapshot(ctx context.Context, flags *RootFlags, shardMaxRows int) (backup.Snapshot, error) {
	account, err := requireAdminAccount(flags)
	if err != nil {
		return backup.Snapshot{}, err
	}
	svc, err := adminDirectoryService(ctx, account)
	if err != nil {
		return backup.Snapshot{}, wrapAdminDirectoryError(err, account)
	}
	domain := domainFromAccount(account)
	accountHash := backupAccountHash(account)
	users, err := fetchBackupAdminUsers(ctx, svc, domain)
	if err != nil {
		return backup.Snapshot{}, err
	}
	groups, err := fetchBackupAdminGroups(ctx, svc, domain)
	if err != nil {
		return backup.Snapshot{}, err
	}
	members := fetchBackupAdminGroupMembers(ctx, svc, groups)
	userShards, err := buildBackupShards(backupServiceAdmin, "users", accountHash, fmt.Sprintf("data/admin/%s/users", accountHash), users, shardMaxRows)
	if err != nil {
		return backup.Snapshot{}, err
	}
	groupShards, err := buildBackupShards(backupServiceAdmin, "groups", accountHash, fmt.Sprintf("data/admin/%s/groups", accountHash), groups, shardMaxRows)
	if err != nil {
		return backup.Snapshot{}, err
	}
	memberShards, err := buildBackupShards(backupServiceAdmin, "members", accountHash, fmt.Sprintf("data/admin/%s/members", accountHash), members, shardMaxRows)
	if err != nil {
		return backup.Snapshot{}, err
	}
	shards := make([]backup.PlainShard, 0, len(userShards)+len(groupShards)+len(memberShards))
	shards = append(shards, userShards...)
	shards = append(shards, groupShards...)
	shards = append(shards, memberShards...)
	return backup.Snapshot{
		Services: []string{backupServiceAdmin},
		Accounts: []string{accountHash},
		Counts: map[string]int{
			"admin.users":   len(users),
			"admin.groups":  len(groups),
			"admin.members": len(members),
		},
		Shards: shards,
	}, nil
}

func buildKeepBackupSnapshot(ctx context.Context, flags *RootFlags, shardMaxRows int) (backup.Snapshot, error) {
	account, err := requireAccount(flags)
	if err != nil {
		return backup.Snapshot{}, err
	}
	svc, err := getKeepService(ctx, flags, &KeepCmd{})
	if err != nil {
		return backup.Snapshot{}, err
	}
	accountHash := backupAccountHash(account)
	notes, err := fetchBackupKeepNotes(ctx, svc)
	if err != nil {
		return backup.Snapshot{}, err
	}
	shards, err := buildBackupShards(backupServiceKeep, "notes", accountHash, fmt.Sprintf("data/keep/%s/notes", accountHash), notes, shardMaxRows)
	if err != nil {
		return backup.Snapshot{}, err
	}
	return backup.Snapshot{
		Services: []string{backupServiceKeep},
		Accounts: []string{accountHash},
		Counts:   map[string]int{"keep.notes": len(notes)},
		Shards:   shards,
	}, nil
}

func fetchBackupCloudIdentityGroups(ctx context.Context, svc *cloudidentity.Service, account string) ([]*cloudidentity.GroupRelation, error) {
	var out []*cloudidentity.GroupRelation
	pageToken := ""
	var guard pageTokenGuard
	for {
		if err := guard.check(pageToken); err != nil {
			return nil, err
		}
		call := svc.Groups.Memberships.SearchTransitiveGroups("groups/-").
			Query(searchTransitiveGroupsQuery(account)).
			PageSize(1000).
			Context(ctx)
		if pageToken != "" {
			call = call.PageToken(pageToken)
		}
		resp, err := call.Do()
		if err != nil {
			return nil, wrapCloudIdentityError(err, account)
		}
		out = append(out, resp.Memberships...)
		if resp.NextPageToken == "" {
			break
		}
		pageToken = resp.NextPageToken
	}
	sort.Slice(out, func(i, j int) bool { return groupRelationEmail(out[i]) < groupRelationEmail(out[j]) })
	return out, nil
}

func fetchBackupCloudIdentityGroupMembers(ctx context.Context, svc *cloudidentity.Service, groups []*cloudidentity.GroupRelation) []groupsBackupMember {
	var out []groupsBackupMember
	for _, group := range groups {
		groupEmail := groupRelationEmail(group)
		if groupEmail == "" {
			continue
		}
		groupName, err := lookupGroupByEmail(ctx, svc, groupEmail)
		if err != nil {
			out = append(out, groupsBackupMember{GroupEmail: groupEmail, Error: errfmt.Format(err)})
			continue
		}
		pageToken := ""
		var guard pageTokenGuard
		groupStart := len(out)
		for {
			if err := guard.check(pageToken); err != nil {
				out = append(out[:groupStart], groupsBackupMember{GroupEmail: groupEmail, Error: err.Error()})
				break
			}
			call := svc.Groups.Memberships.List(groupName).PageSize(1000).Context(ctx)
			if pageToken != "" {
				call = call.PageToken(pageToken)
			}
			resp, err := call.Do()
			if err != nil {
				out = append(out, groupsBackupMember{GroupEmail: groupEmail, Error: errfmt.Format(err)})
				break
			}
			for _, member := range resp.Memberships {
				out = append(out, groupsBackupMember{GroupEmail: groupEmail, Member: member})
			}
			if resp.NextPageToken == "" {
				break
			}
			pageToken = resp.NextPageToken
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].GroupEmail == out[j].GroupEmail {
			return cloudIdentityMemberSortKey(out[i].Member) < cloudIdentityMemberSortKey(out[j].Member)
		}
		return out[i].GroupEmail < out[j].GroupEmail
	})
	return out
}

func fetchSortedBackupPages[T any](fetch func(string) ([]T, string, error), less func(T, T) bool) ([]T, error) {
	rows, err := collectUnboundedPages("", fetch)
	if err != nil {
		return nil, err
	}
	sort.Slice(rows, func(i, j int) bool { return less(rows[i], rows[j]) })
	return rows, nil
}

func fetchBackupAdminUsers(ctx context.Context, svc *admin.Service, domain string) ([]*admin.User, error) {
	return fetchSortedBackupPages(
		func(pageToken string) ([]*admin.User, string, error) {
			call := svc.Users.List().Domain(domain).MaxResults(500).Context(ctx)
			if pageToken != "" {
				call = call.PageToken(pageToken)
			}
			resp, err := call.Do()
			if err != nil {
				return nil, "", err
			}
			return resp.Users, resp.NextPageToken, nil
		},
		func(a, b *admin.User) bool { return a.PrimaryEmail < b.PrimaryEmail },
	)
}

func fetchBackupAdminGroups(ctx context.Context, svc *admin.Service, domain string) ([]*admin.Group, error) {
	return fetchSortedBackupPages(
		func(pageToken string) ([]*admin.Group, string, error) {
			call := svc.Groups.List().Domain(domain).MaxResults(200).Context(ctx)
			if pageToken != "" {
				call = call.PageToken(pageToken)
			}
			resp, err := call.Do()
			if err != nil {
				return nil, "", err
			}
			return resp.Groups, resp.NextPageToken, nil
		},
		func(a, b *admin.Group) bool { return a.Email < b.Email },
	)
}

func fetchBackupAdminGroupMembers(ctx context.Context, svc *admin.Service, groups []*admin.Group) []adminBackupMember {
	var out []adminBackupMember
	for _, group := range groups {
		if group == nil || strings.TrimSpace(group.Email) == "" {
			continue
		}
		members, err := fetchAllBackupAdminGroupMembers(ctx, svc, group.Email)
		if err != nil {
			out = append(out, adminBackupMember{GroupEmail: group.Email, Error: errfmt.Format(err)})
			continue
		}
		for _, member := range members {
			out = append(out, adminBackupMember{GroupEmail: group.Email, Member: member})
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].GroupEmail == out[j].GroupEmail {
			return adminMemberSortKey(out[i].Member) < adminMemberSortKey(out[j].Member)
		}
		return out[i].GroupEmail < out[j].GroupEmail
	})
	return out
}

func fetchAllBackupAdminGroupMembers(ctx context.Context, svc *admin.Service, groupEmail string) ([]*admin.Member, error) {
	items, _, err := loadPagedItems("", true, func(pageToken string) ([]*admin.Member, string, error) {
		call := svc.Members.List(groupEmail).MaxResults(200).Context(ctx)
		if pageToken != "" {
			call = call.PageToken(pageToken)
		}
		resp, err := call.Do()
		if err != nil {
			return nil, "", err
		}
		return resp.Members, resp.NextPageToken, nil
	})
	if err != nil {
		return nil, err
	}
	return items, nil
}

func fetchBackupKeepNotes(ctx context.Context, svc *keepapi.Service) ([]*keepapi.Note, error) {
	return fetchSortedBackupPages(
		func(pageToken string) ([]*keepapi.Note, string, error) {
			call := svc.Notes.List().PageSize(1000).Context(ctx)
			if pageToken != "" {
				call = call.PageToken(pageToken)
			}
			resp, err := call.Do()
			if err != nil {
				return nil, "", err
			}
			return resp.Notes, resp.NextPageToken, nil
		},
		func(a, b *keepapi.Note) bool { return a.Name < b.Name },
	)
}

func domainFromAccount(account string) string {
	_, domain, ok := strings.Cut(strings.TrimSpace(account), "@")
	if !ok {
		return strings.TrimSpace(account)
	}
	return domain
}

func groupRelationEmail(group *cloudidentity.GroupRelation) string {
	if group == nil || group.GroupKey == nil {
		return ""
	}
	return group.GroupKey.Id
}

func cloudIdentityMemberSortKey(member *cloudidentity.Membership) string {
	if member == nil || member.PreferredMemberKey == nil {
		return ""
	}
	return member.PreferredMemberKey.Id
}

func adminMemberSortKey(member *admin.Member) string {
	if member == nil {
		return ""
	}
	return member.Email
}
