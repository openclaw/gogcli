package cmd

import (
	"context"
	"fmt"
	"os"
	"strings"

	"google.golang.org/api/chat/v1"

	"github.com/steipete/gogcli/internal/outfmt"
	"github.com/steipete/gogcli/internal/ui"
)

type ChatSpacesCmd struct {
	List   ChatSpacesListCmd   `cmd:"" name:"list" aliases:"ls" help:"List spaces"`
	Find   ChatSpacesFindCmd   `cmd:"" name:"find" aliases:"search,query" help:"Find spaces by display name"`
	Create ChatSpacesCreateCmd `cmd:"" name:"create" aliases:"add,new" help:"Create a space"`
}

type ChatSpacesListCmd struct {
	Max       int64  `name:"max" aliases:"limit" help:"Max results" default:"100"`
	Page      string `name:"page" aliases:"cursor" help:"Page token"`
	All       bool   `name:"all" aliases:"all-pages,allpages" help:"Fetch all pages"`
	FailEmpty bool   `name:"fail-empty" aliases:"non-empty,require-results" help:"Exit with code 3 if no results"`
}

func (c *ChatSpacesListCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}
	if err = requireWorkspaceAccount(account); err != nil {
		return err
	}

	svc, err := newChatService(ctx, account)
	if err != nil {
		return err
	}

	fetch := func(pageToken string) ([]*chat.Space, string, error) {
		call := svc.Spaces.List().PageSize(c.Max).Context(ctx)
		if strings.TrimSpace(pageToken) != "" {
			call = call.PageToken(pageToken)
		}
		resp, callErr := call.Do()
		if callErr != nil {
			return nil, "", callErr
		}
		return resp.Spaces, resp.NextPageToken, nil
	}

	var spaces []*chat.Space
	nextPageToken := ""
	if c.All {
		all, err := collectAllPages(c.Page, fetch)
		if err != nil {
			return err
		}
		spaces = all
	} else {
		var err error
		spaces, nextPageToken, err = fetch(c.Page)
		if err != nil {
			return err
		}
	}

	if outfmt.IsJSON(ctx) {
		type item struct {
			Resource    string `json:"resource"`
			Name        string `json:"name,omitempty"`
			SpaceType   string `json:"type,omitempty"`
			SpaceURI    string `json:"uri,omitempty"`
			ThreadState string `json:"threading,omitempty"`
		}
		items := make([]item, 0, len(spaces))
		for _, space := range spaces {
			if space == nil {
				continue
			}
			items = append(items, item{
				Resource:    space.Name,
				Name:        space.DisplayName,
				SpaceType:   chatSpaceType(space),
				SpaceURI:    space.SpaceUri,
				ThreadState: space.SpaceThreadingState,
			})
		}
		if err := outfmt.WriteJSON(ctx, os.Stdout, map[string]any{
			"spaces":        items,
			"nextPageToken": nextPageToken,
		}); err != nil {
			return err
		}
		if len(items) == 0 {
			return failEmptyExit(c.FailEmpty)
		}
		return nil
	}

	if len(spaces) == 0 {
		u.Err().Println("No spaces")
		return failEmptyExit(c.FailEmpty)
	}

	w, flush := tableWriter(ctx)
	defer flush()
	fmt.Fprintln(w, "RESOURCE\tNAME\tTYPE")
	for _, space := range spaces {
		if space == nil {
			continue
		}
		fmt.Fprintf(w, "%s\t%s\t%s\n",
			space.Name,
			sanitizeTab(space.DisplayName),
			sanitizeTab(chatSpaceType(space)),
		)
	}
	printNextPageHint(u, nextPageToken)
	return nil
}

type ChatSpacesFindCmd struct {
	DisplayName string `arg:"" name:"displayName" help:"Space display name"`
	Max         int64  `name:"max" aliases:"limit" help:"Max results per page" default:"100"`
}

func (c *ChatSpacesFindCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}
	if err = requireWorkspaceAccount(account); err != nil {
		return err
	}

	displayName := strings.TrimSpace(c.DisplayName)
	if displayName == "" {
		return usage("required: displayName")
	}

	svc, err := newChatService(ctx, account)
	if err != nil {
		return err
	}

	fetch := func(pageToken string) ([]*chat.Space, string, error) {
		call := svc.Spaces.List().PageSize(c.Max).Context(ctx)
		if strings.TrimSpace(pageToken) != "" {
			call = call.PageToken(pageToken)
		}
		resp, callErr := call.Do()
		if callErr != nil {
			return nil, "", callErr
		}
		matches := make([]*chat.Space, 0, len(resp.Spaces))
		for _, space := range resp.Spaces {
			if space == nil {
				continue
			}
			if strings.EqualFold(space.DisplayName, displayName) {
				matches = append(matches, space)
			}
		}
		return matches, resp.NextPageToken, nil
	}

	matches, err := collectAllPages("", fetch)
	if err != nil {
		return err
	}

	if outfmt.IsJSON(ctx) {
		type item struct {
			Resource  string `json:"resource"`
			Name      string `json:"name,omitempty"`
			SpaceType string `json:"type,omitempty"`
			SpaceURI  string `json:"uri,omitempty"`
		}
		items := make([]item, 0, len(matches))
		for _, space := range matches {
			if space == nil {
				continue
			}
			items = append(items, item{
				Resource:  space.Name,
				Name:      space.DisplayName,
				SpaceType: chatSpaceType(space),
				SpaceURI:  space.SpaceUri,
			})
		}
		return outfmt.WriteJSON(ctx, os.Stdout, map[string]any{"spaces": items})
	}

	if len(matches) == 0 {
		u.Err().Println("No results")
		return nil
	}

	w, flush := tableWriter(ctx)
	defer flush()
	fmt.Fprintln(w, "RESOURCE\tNAME\tTYPE")
	for _, space := range matches {
		if space == nil {
			continue
		}
		fmt.Fprintf(w, "%s\t%s\t%s\n",
			space.Name,
			sanitizeTab(space.DisplayName),
			sanitizeTab(chatSpaceType(space)),
		)
	}
	return nil
}

type ChatSpacesCreateCmd struct {
	DisplayName string   `arg:"" name:"displayName" help:"Space display name"`
	Members     []string `name:"member" help:"Space members (email or users/...; repeatable or comma-separated)"`
}

func (c *ChatSpacesCreateCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	displayName := strings.TrimSpace(c.DisplayName)
	if displayName == "" {
		return usage("required: displayName")
	}

	members := parseCommaArgs(c.Members)
	memberUsers := make([]string, 0, len(members))
	memberships := make([]*chat.Membership, 0, len(members))
	for _, member := range members {
		user := normalizeUser(member)
		if user == "" {
			continue
		}
		memberUsers = append(memberUsers, user)
		memberships = append(memberships, &chat.Membership{
			Member: &chat.User{
				Name: user,
				Type: "HUMAN",
			},
		})
	}

	if err := dryRunExit(ctx, flags, "chat.spaces.create", map[string]any{
		"display_name": displayName,
		"members":      members,
		"member_users": memberUsers,
	}); err != nil {
		return err
	}

	account, err := requireAccount(flags)
	if err != nil {
		return err
	}
	if err = requireWorkspaceAccount(account); err != nil {
		return err
	}

	svc, err := newChatService(ctx, account)
	if err != nil {
		return err
	}

	req := &chat.SetUpSpaceRequest{
		Space: &chat.Space{
			SpaceType:   "SPACE",
			DisplayName: displayName,
		},
	}
	if len(memberships) > 0 {
		req.Memberships = memberships
	}
	resp, err := svc.Spaces.Setup(req).Do()
	if err != nil {
		return err
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(ctx, os.Stdout, map[string]any{"space": resp})
	}

	if resp == nil {
		u.Out().Printf("space\t%s", displayName)
		return nil
	}
	if resp.Name != "" {
		u.Out().Printf("resource\t%s", resp.Name)
	}
	if resp.DisplayName != "" {
		u.Out().Printf("name\t%s", resp.DisplayName)
	}
	return nil
}
