package cmd

import (
	"context"
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
	if c.Max <= 0 {
		return usage("max must be > 0")
	}
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}
	if err = requireWorkspaceAccount(account); err != nil {
		return err
	}

	svc, err := chatService(ctx, account)
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

	spaces, nextPageToken, err := loadPagedItems(c.Page, c.All, fetch)
	if err != nil {
		return err
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
		if err := outfmt.WriteJSON(ctx, stdoutWriter(ctx), map[string]any{
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

	if err := outfmt.WriteTable(
		ctx,
		stdoutWriter(ctx),
		compactChatRows(spaces),
		chatSpaceColumns(),
	); err != nil {
		return err
	}
	printNextPageHintWithAll(u, nextPageToken, "--all/--all-pages")
	return nil
}

type ChatSpacesFindCmd struct {
	DisplayName string `arg:"" name:"displayName" help:"Space display name (substring match, case-insensitive)"`
	Max         int64  `name:"max" aliases:"limit" help:"Max results per page" default:"100"`
	Exact       bool   `name:"exact" help:"Require an exact, case-insensitive match on displayName instead of substring match"`
}

func (c *ChatSpacesFindCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	displayName := strings.TrimSpace(c.DisplayName)
	if displayName == "" {
		return usage("required: displayName")
	}
	if c.Max <= 0 {
		return usage("max must be > 0")
	}

	account, err := requireAccount(flags)
	if err != nil {
		return err
	}
	if err = requireWorkspaceAccount(account); err != nil {
		return err
	}

	svc, err := chatService(ctx, account)
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
			if chatSpaceDisplayNameMatches(space.DisplayName, displayName, c.Exact) {
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
		return outfmt.WriteJSON(ctx, stdoutWriter(ctx), map[string]any{"spaces": items})
	}

	if len(matches) == 0 {
		u.Err().Println("No results")
		return nil
	}

	return outfmt.WriteTable(ctx, stdoutWriter(ctx), compactChatRows(matches), chatSpaceColumns())
}

func chatSpaceDisplayNameMatches(displayName, query string, exact bool) bool {
	if exact {
		return strings.EqualFold(displayName, query)
	}
	return strings.Contains(strings.ToLower(displayName), strings.ToLower(query))
}

type ChatSpacesCreateCmd struct {
	DisplayName string   `arg:"" name:"displayName" help:"Space display name"`
	Members     []string `name:"member" help:"Space members (email or users/...; repeatable or comma-separated)"`
}

func (c *ChatSpacesCreateCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	plan, err := newChatSpaceCreatePlan(chatSpaceCreateInput{
		DisplayName: c.DisplayName,
		Members:     c.Members,
	})
	if err != nil {
		return err
	}

	if dryRunErr := dryRunExit(ctx, flags, "chat.spaces.create", plan.dryRunPayload()); dryRunErr != nil {
		return dryRunErr
	}

	account, err := requireAccount(flags)
	if err != nil {
		return err
	}
	if err = requireWorkspaceAccount(account); err != nil {
		return err
	}

	svc, err := chatService(ctx, account)
	if err != nil {
		return err
	}

	resp, err := svc.Spaces.Setup(plan.Request).Do()
	if err != nil {
		return err
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(ctx, stdoutWriter(ctx), map[string]any{"space": resp})
	}

	if resp == nil {
		u.Out().Linef("space\t%s", plan.DisplayName)
		return nil
	}
	if resp.Name != "" {
		u.Out().Linef("resource\t%s", resp.Name)
	}
	if resp.DisplayName != "" {
		u.Out().Linef("name\t%s", resp.DisplayName)
	}
	return nil
}
