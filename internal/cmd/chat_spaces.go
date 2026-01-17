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
	List   ChatSpacesListCmd   `cmd:"" name:"list" help:"List spaces"`
	Find   ChatSpacesFindCmd   `cmd:"" name:"find" help:"Find spaces by display name"`
	Create ChatSpacesCreateCmd `cmd:"" name:"create" help:"Create a space"`
}

type ChatSpacesListCmd struct {
	Max  int64  `name:"max" aliases:"limit" help:"Max results" default:"100"`
	Page string `name:"page" help:"Page token"`
}

func (c *ChatSpacesListCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}

	svc, err := newChatService(ctx, account)
	if err != nil {
		return err
	}

	resp, err := svc.Spaces.List().
		PageSize(c.Max).
		PageToken(c.Page).
		Do()
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
		items := make([]item, 0, len(resp.Spaces))
		for _, space := range resp.Spaces {
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
		return outfmt.WriteJSON(os.Stdout, map[string]any{
			"spaces":        items,
			"nextPageToken": resp.NextPageToken,
		})
	}

	if len(resp.Spaces) == 0 {
		u.Err().Println("No spaces")
		return nil
	}

	w, flush := tableWriter(ctx)
	defer flush()
	fmt.Fprintln(w, "RESOURCE\tNAME\tTYPE")
	for _, space := range resp.Spaces {
		if space == nil {
			continue
		}
		fmt.Fprintf(w, "%s\t%s\t%s\n",
			space.Name,
			sanitizeTab(space.DisplayName),
			sanitizeTab(chatSpaceType(space)),
		)
	}
	printNextPageHint(u, resp.NextPageToken)
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

	displayName := strings.TrimSpace(c.DisplayName)
	if displayName == "" {
		return usage("required: displayName")
	}

	svc, err := newChatService(ctx, account)
	if err != nil {
		return err
	}

	var matches []*chat.Space
	pageToken := ""
	for {
		call := svc.Spaces.List().PageSize(c.Max)
		if pageToken != "" {
			call = call.PageToken(pageToken)
		}
		resp, err := call.Do()
		if err != nil {
			return err
		}
		for _, space := range resp.Spaces {
			if space == nil {
				continue
			}
			if strings.EqualFold(space.DisplayName, displayName) {
				matches = append(matches, space)
			}
		}
		if resp.NextPageToken == "" {
			break
		}
		pageToken = resp.NextPageToken
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
		return outfmt.WriteJSON(os.Stdout, map[string]any{"spaces": items})
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
	account, err := requireAccount(flags)
	if err != nil {
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

	members := parseCommaArgs(c.Members)
	memberships := make([]*chat.Membership, 0, len(members))
	for _, member := range members {
		user := normalizeUser(member)
		if user == "" {
			continue
		}
		memberships = append(memberships, &chat.Membership{
			Member: &chat.User{
				Name: user,
				Type: "HUMAN",
			},
		})
	}

	resp, err := svc.Spaces.Setup(&chat.SetUpSpaceRequest{
		Space: &chat.Space{
			SpaceType:   "SPACE",
			DisplayName: displayName,
		},
		Memberships: memberships,
	}).Do()
	if err != nil {
		return err
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(os.Stdout, map[string]any{"space": resp})
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
