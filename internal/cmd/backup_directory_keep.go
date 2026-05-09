package cmd

import (
	"context"
	"fmt"
	"sort"
	"strings"

	admin "google.golang.org/api/admin/directory/v1"
	"google.golang.org/api/cloudidentity/v1"
	keepapi "google.golang.org/api/keep/v1"

	"github.com/steipete/gogcli/internal/backup"
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
	account, err := requireAccount(flags)
	if err != nil {
		return backup.Snapshot{}, err
	}
	svc, err := newCloudIdentityService(ctx, account)
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
	svc, err := newAdminDirectoryService(ctx, account)
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
	for {
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
			out = append(out, groupsBackupMember{GroupEmail: groupEmail, Error: err.Error()})
			continue
		}
		pageToken := ""
		for {
			call := svc.Groups.Memberships.List(groupName).PageSize(1000).Context(ctx)
			if pageToken != "" {
				call = call.PageToken(pageToken)
			}
			resp, err := call.Do()
			if err != nil {
				out = append(out, groupsBackupMember{GroupEmail: groupEmail, Error: err.Error()})
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

func fetchBackupAdminUsers(ctx context.Context, svc *admin.Service, domain string) ([]*admin.User, error) {
	var out []*admin.User
	pageToken := ""
	for {
		call := svc.Users.List().Domain(domain).MaxResults(500).Context(ctx)
		if pageToken != "" {
			call = call.PageToken(pageToken)
		}
		resp, err := call.Do()
		if err != nil {
			return nil, err
		}
		out = append(out, resp.Users...)
		if resp.NextPageToken == "" {
			break
		}
		pageToken = resp.NextPageToken
	}
	sort.Slice(out, func(i, j int) bool { return out[i].PrimaryEmail < out[j].PrimaryEmail })
	return out, nil
}

func fetchBackupAdminGroups(ctx context.Context, svc *admin.Service, domain string) ([]*admin.Group, error) {
	var out []*admin.Group
	pageToken := ""
	for {
		call := svc.Groups.List().Domain(domain).MaxResults(200).Context(ctx)
		if pageToken != "" {
			call = call.PageToken(pageToken)
		}
		resp, err := call.Do()
		if err != nil {
			return nil, err
		}
		out = append(out, resp.Groups...)
		if resp.NextPageToken == "" {
			break
		}
		pageToken = resp.NextPageToken
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Email < out[j].Email })
	return out, nil
}

func fetchBackupAdminGroupMembers(ctx context.Context, svc *admin.Service, groups []*admin.Group) []adminBackupMember {
	var out []adminBackupMember
	for _, group := range groups {
		if group == nil || strings.TrimSpace(group.Email) == "" {
			continue
		}
		pageToken := ""
		for {
			call := svc.Members.List(group.Email).MaxResults(200).Context(ctx)
			if pageToken != "" {
				call = call.PageToken(pageToken)
			}
			resp, err := call.Do()
			if err != nil {
				out = append(out, adminBackupMember{GroupEmail: group.Email, Error: err.Error()})
				break
			}
			for _, member := range resp.Members {
				out = append(out, adminBackupMember{GroupEmail: group.Email, Member: member})
			}
			if resp.NextPageToken == "" {
				break
			}
			pageToken = resp.NextPageToken
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

func fetchBackupKeepNotes(ctx context.Context, svc *keepapi.Service) ([]*keepapi.Note, error) {
	var out []*keepapi.Note
	pageToken := ""
	for {
		call := svc.Notes.List().PageSize(1000).Context(ctx)
		if pageToken != "" {
			call = call.PageToken(pageToken)
		}
		resp, err := call.Do()
		if err != nil {
			return nil, err
		}
		out = append(out, resp.Notes...)
		if resp.NextPageToken == "" {
			break
		}
		pageToken = resp.NextPageToken
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, nil
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
