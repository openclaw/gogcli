package cmd

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/mark3labs/mcp-go/mcp"
)

func mcpGmailMutationTools() []mcpToolSpec {
	return []mcpToolSpec{
		mcpGmailListDraftsTool(),
		{
			Name: "gmail_get_draft", Service: "gmail", Risk: mcpRiskRead,
			Description: "Read one Gmail draft, including its MIME payload, without downloading attachments.",
			Options:     []mcp.ToolOption{mcp.WithString("draft_id", mcp.Required(), mcp.Description("Gmail draft ID"))},
			BuildArgs:   mcpGmailIDArgs([]string{"gmail", "drafts", "get"}, "draft_id"),
		},
		{
			Name: "gmail_list_labels", Service: "gmail", Risk: mcpRiskRead,
			Description: "List Gmail labels and their case-sensitive IDs.",
			BuildArgs:   func(mcp.CallToolRequest) ([]string, error) { return []string{"gmail", "labels", "list"}, nil },
		},
		mcpGmailComposeTool("create"),
		mcpGmailComposeTool("update"),
		{
			Name: "gmail_create_label", Service: "gmail", Risk: mcpRiskWrite,
			Description: "Create a Gmail label.",
			Options:     []mcp.ToolOption{mcp.WithString("name", mcp.Required(), mcp.Description("New label name"))},
			BuildArgs:   mcpGmailIDArgs([]string{"gmail", "labels", "create"}, "name"),
		},
		mcpGmailModifyTool(false),
		mcpGmailModifyTool(true),
		mcpGmailMessagesTool("gmail_mark_read", "Mark explicit Gmail messages as read.", []string{"gmail", "mark-read"}, ""),
		mcpGmailMessagesTool("gmail_mark_unread", "Mark explicit Gmail messages as unread.", []string{"gmail", "unread"}, ""),
		mcpGmailMessagesTool("gmail_archive_messages", "Archive explicit Gmail messages by removing INBOX.", []string{"gmail", "archive"}, ""),
		mcpGmailMessagesTool("gmail_trash_messages", "Move explicit Gmail messages to Trash, removing INBOX.", []string{"gmail", "trash"}, ""),
		mcpGmailMessagesTool("gmail_restore_messages", "Remove TRASH from explicit Gmail messages. Does not add INBOX.", []string{"gmail", "batch", "modify", "--remove=TRASH"}, ""),
		mcpGmailComposeTool("send"),
		{
			Name: "gmail_send_draft", Service: "gmail", Risk: mcpRiskWrite, Capability: mcpCapabilityGmailSend,
			Description: "Send an existing Gmail draft to its stored recipients. Requires explicit Gmail send authorization.",
			Options:     []mcp.ToolOption{mcp.WithString("draft_id", mcp.Required(), mcp.Description("Gmail draft ID to send"))},
			BuildArgs:   mcpGmailIDArgs([]string{"gmail", "drafts", "send"}, "draft_id"),
		},
		{
			Name: "gmail_delete_draft", Service: "gmail", Risk: mcpRiskWrite, Capability: mcpCapabilityGmailDelete,
			Description: "Permanently delete a Gmail draft; it cannot be restored. Requires Gmail delete authorization and server-startup --force.",
			Options:     []mcp.ToolOption{mcp.WithString("draft_id", mcp.Required(), mcp.Description("Gmail draft ID to permanently delete"))},
			BuildArgs:   mcpGmailIDArgs([]string{"gmail", "drafts", "delete"}, "draft_id"),
		},
		mcpGmailMessagesTool("gmail_delete_messages", "Permanently delete explicit Gmail messages. Requires Gmail delete authorization, server-startup --force, and the https://mail.google.com/ OAuth scope.", []string{"gmail", "batch", "delete"}, mcpCapabilityGmailDelete),
	}
}

func mcpGmailIDArgs(command []string, key string) func(mcp.CallToolRequest) ([]string, error) {
	return func(req mcp.CallToolRequest) ([]string, error) {
		id, err := requireMCPString(req, key)
		if err != nil {
			return nil, err
		}
		return append(append([]string{}, command...), "--", id), nil
	}
}

func mcpGmailListDraftsTool() mcpToolSpec {
	return mcpToolSpec{
		Name: "gmail_list_drafts", Service: "gmail", Risk: mcpRiskRead,
		Description: "List Gmail draft IDs and their message/thread IDs, one bounded page at a time.",
		Options: []mcp.ToolOption{
			mcp.WithInteger("max", mcp.Description("Maximum results"), mcp.DefaultNumber(20), mcp.Min(1), mcp.Max(100)),
			mcp.WithString("page", mcp.Description("Page token returned by a previous call")),
		},
		BuildArgs: func(req mcp.CallToolRequest) ([]string, error) {
			args := []string{"gmail", "drafts", "list", "--max=" + strconv.Itoa(clampMCPInt(req.GetInt("max", 20), 1, 100))}
			if page := strings.TrimSpace(req.GetString("page", "")); page != "" {
				args = append(args, "--page="+page)
			}
			return args, nil
		},
	}
}

func mcpGmailComposeTool(action string) mcpToolSpec {
	tool := mcpToolSpec{
		Name: "gmail_" + action + "_draft", Service: "gmail", Risk: mcpRiskWrite,
		Description: "Create a Gmail draft from literal text or HTML; no local files are read.",
		Options: []mcp.ToolOption{
			mcp.WithString("to", mcp.Description("Comma-separated recipients; optional for a draft. On update, omit to keep To or pass an empty string to clear it.")),
			mcp.WithString("cc", mcp.Description("Comma-separated Cc recipients; omitted Cc is cleared on update")),
			mcp.WithString("bcc", mcp.Description("Comma-separated Bcc recipients; omitted Bcc is cleared on update")),
			mcp.WithString("subject", mcp.Required(), mcp.Description("Message subject")),
			mcp.WithString("body", mcp.Description("Literal plain text body; body or body_html is required")),
			mcp.WithString("body_html", mcp.Description("Literal HTML body; body or body_html is required")),
			mcp.WithString("from", mcp.Description("Optional verified send-as alias")),
			mcp.WithString("reply_to", mcp.Description("Reply-To header address")),
			mcp.WithString("reply_to_message_id", mcp.Description("Reply target Gmail message ID; mutually exclusive with thread_id")),
			mcp.WithString("thread_id", mcp.Description("Reply target Gmail thread ID; mutually exclusive with reply_to_message_id")),
			mcp.WithBoolean("reply_all", mcp.Description("Resolve recipients from the reply target")),
			mcp.WithBoolean("quote", mcp.Description("Include the original message in the reply body")),
		},
	}
	command := []string{"gmail", "drafts", action}
	switch action {
	case "update":
		tool.Description = "Replace a Gmail draft's subject/body. Omitted To, attachments, and reply lineage are preserved; omitted Cc/Bcc are cleared. Supply body_html to retain HTML."
		tool.Options = append(tool.Options,
			mcp.WithString("draft_id", mcp.Required(), mcp.Description("Gmail draft ID")),
			mcp.WithBoolean("clear_attachments", mcp.Description("Remove existing draft attachments")),
			mcp.WithBoolean("clear_reply_context", mcp.Description("Remove existing reply headers; incompatible with reply targets and quote")),
		)
	case "send":
		tool.Name = "gmail_send_message"
		tool.Description = "Send a Gmail message from literal text or HTML. Requires explicit Gmail send authorization."
		tool.Capability = mcpCapabilityGmailSend
		command = []string{"gmail", "send"}
	}
	tool.BuildArgs = func(req mcp.CallToolRequest) ([]string, error) {
		if _, err := requireMCPString(req, "subject"); err != nil {
			return nil, err
		}
		if strings.TrimSpace(req.GetString("body", "")) == "" && strings.TrimSpace(req.GetString("body_html", "")) == "" {
			return nil, fmt.Errorf("body or body_html is required")
		}
		args := append([]string{}, command...)
		for _, key := range []string{"to", "cc", "bcc", "subject", "body", "body_html", "from", "reply_to", "reply_to_message_id", "thread_id"} {
			if _, present := req.GetArguments()[key]; present {
				value, err := req.RequireString(key)
				if err != nil {
					return nil, err
				}
				args = append(args, "--"+strings.ReplaceAll(key, "_", "-")+"="+value)
			}
		}
		for _, key := range []string{"reply_all", "quote"} {
			if req.GetBool(key, false) {
				args = append(args, "--"+strings.ReplaceAll(key, "_", "-"))
			}
		}
		if action == "update" {
			for _, key := range []string{"clear_attachments", "clear_reply_context"} {
				if req.GetBool(key, false) {
					args = append(args, "--"+strings.ReplaceAll(key, "_", "-"))
				}
			}
			id, err := requireMCPString(req, "draft_id")
			if err != nil {
				return nil, err
			}
			args = append(args, "--", id)
		}
		return args, nil
	}
	return tool
}

func mcpGmailMessagesTool(name, description string, command []string, capability mcpToolCapability) mcpToolSpec {
	return mcpToolSpec{
		Name: name, Service: "gmail", Risk: mcpRiskWrite, Capability: capability, Description: description,
		Options: []mcp.ToolOption{mcp.WithArray("message_ids", mcp.Required(), mcp.Description("Explicit Gmail message IDs (1–1000)"), mcp.WithStringItems(), mcp.MinItems(1), mcp.MaxItems(1000))},
		BuildArgs: func(req mcp.CallToolRequest) ([]string, error) {
			ids, err := mcpGmailStrings(req, "message_ids", 1, 1000)
			if err != nil {
				return nil, err
			}
			args := append(append([]string{}, command...), "--")
			return append(args, ids...), nil
		},
	}
}

func mcpGmailModifyTool(thread bool) mcpToolSpec {
	tool := mcpGmailMessagesTool("gmail_modify_messages", "Add or remove labels on explicit Gmail messages. Label IDs are case-sensitive; names use CLI label lookup.", []string{"gmail", "batch", "modify"}, "")
	if thread {
		tool.Name = "gmail_modify_thread"
		tool.Description = "Add or remove labels on every message in one Gmail thread. Label IDs are case-sensitive."
		tool.Options = []mcp.ToolOption{mcp.WithString("thread_id", mcp.Required(), mcp.Description("Gmail thread ID"))}
		tool.BuildArgs = mcpGmailIDArgs([]string{"gmail", "thread", "modify"}, "thread_id")
	}
	for _, key := range []string{"add_labels", "remove_labels"} {
		tool.Options = append(tool.Options, mcp.WithArray(key, mcp.Description("Literal label names or case-sensitive IDs; commas and backslashes in each entry are preserved"), mcp.WithStringItems(), mcp.MaxItems(100)))
	}
	buildIDs := tool.BuildArgs
	tool.BuildArgs = func(req mcp.CallToolRequest) ([]string, error) {
		args, err := buildIDs(req)
		if err != nil {
			return nil, err
		}
		var labelArgs []string
		for _, key := range []string{"add_labels", "remove_labels"} {
			if _, present := req.GetArguments()[key]; !present {
				continue
			}
			labels, err := mcpGmailStrings(req, key, 0, 100)
			if err != nil {
				return nil, err
			}
			flag := "--" + strings.TrimSuffix(key, "_labels") + "-label="
			for _, label := range labels {
				labelArgs = append(labelArgs, flag+label)
			}
		}
		if len(labelArgs) == 0 {
			return nil, fmt.Errorf("add_labels or remove_labels must contain at least one label")
		}
		// Insert only fixed label flags before the positional-argument delimiter.
		return append(append(args[:3:3], labelArgs...), args[3:]...), nil
	}
	return tool
}

func mcpGmailStrings(req mcp.CallToolRequest, key string, minItems, maxItems int) ([]string, error) {
	values, err := req.RequireStringSlice(key)
	if err != nil {
		return nil, err
	}
	if len(values) < minItems || len(values) > maxItems {
		return nil, fmt.Errorf("%s must contain %d–%d values", key, minItems, maxItems)
	}
	for i, value := range values {
		values[i] = strings.TrimSpace(value)
		if values[i] == "" {
			return nil, fmt.Errorf("%s[%d] must not be empty", key, i)
		}
	}
	return values, nil
}
