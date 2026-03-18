package cmd

import "github.com/steipete/gogcli/internal/googleapi"

var newGmailService = googleapi.NewGmail

type GmailCmd struct {
	Search     GmailSearchCmd     `cmd:"" name:"search" aliases:"find,query,ls,list" group:"Read" help:"Search threads using Gmail query syntax"`
	Messages   GmailMessagesCmd   `cmd:"" name:"messages" aliases:"message,msg,msgs" group:"Read" help:"Message operations"`
	Thread     GmailThreadCmd     `cmd:"" name:"thread" aliases:"threads,read" group:"Organize" help:"Thread operations (get, modify)"`
	Get        GmailGetCmd        `cmd:"" name:"get" aliases:"info,show" group:"Read" help:"Get a message (full|metadata|raw)"`
	Attachment GmailAttachmentCmd `cmd:"" name:"attachment" group:"Read" help:"Download a single attachment"`
	URL        GmailURLCmd        `cmd:"" name:"url" group:"Read" help:"Print Gmail web URLs for threads"`
	History    GmailHistoryCmd    `cmd:"" name:"history" group:"Read" help:"Gmail history"`

	Labels  GmailLabelsCmd   `cmd:"" name:"labels" aliases:"label" group:"Organize" help:"Label operations"`
	Batch   GmailBatchCmd    `cmd:"" name:"batch" group:"Organize" help:"Batch operations"`
	Archive GmailArchiveCmd  `cmd:"" name:"archive" group:"Organize" help:"Archive messages (remove from inbox)"`
	Read    GmailReadCmd     `cmd:"" name:"mark-read" aliases:"read-messages" group:"Organize" help:"Mark messages as read"`
	Unread  GmailUnreadCmd   `cmd:"" name:"unread" aliases:"mark-unread" group:"Organize" help:"Mark messages as unread"`
	Trash   GmailTrashMsgCmd `cmd:"" name:"trash" group:"Organize" help:"Move messages to trash"`

	Send      GmailSendCmd      `cmd:"" name:"send" group:"Write" help:"Send an email"`
	AutoReply GmailAutoReplyCmd `cmd:"" name:"autoreply" group:"Write" help:"Reply once to matching messages"`
	Track     GmailTrackCmd     `cmd:"" name:"track" group:"Write" help:"Email open tracking"`
	Drafts    GmailDraftsCmd    `cmd:"" name:"drafts" aliases:"draft" group:"Write" help:"Draft operations"`

	Settings GmailSettingsCmd `cmd:"" name:"settings" group:"Admin" help:"Settings and admin"`

	Watch       GmailWatchCmd       `cmd:"" name:"watch" hidden:"" help:"Manage Gmail watch"`
	AutoForward GmailAutoForwardCmd `cmd:"" name:"autoforward" hidden:"" help:"Auto-forwarding settings"`
	Delegates   GmailDelegatesCmd   `cmd:"" name:"delegates" hidden:"" help:"Delegate operations"`
	Filters     GmailFiltersCmd     `cmd:"" name:"filters" hidden:"" help:"Filter operations"`
	Forwarding  GmailForwardingCmd  `cmd:"" name:"forwarding" hidden:"" help:"Forwarding addresses"`
	SendAs      GmailSendAsCmd      `cmd:"" name:"sendas" hidden:"" help:"Send-as settings"`
	Vacation    GmailVacationCmd    `cmd:"" name:"vacation" hidden:"" help:"Vacation responder"`
}

type GmailSettingsCmd struct {
	Filters     GmailFiltersCmd     `cmd:"" name:"filters" group:"Organize" help:"Filter operations"`
	Delegates   GmailDelegatesCmd   `cmd:"" name:"delegates" group:"Admin" help:"Delegate operations"`
	Forwarding  GmailForwardingCmd  `cmd:"" name:"forwarding" group:"Admin" help:"Forwarding addresses"`
	AutoForward GmailAutoForwardCmd `cmd:"" name:"autoforward" group:"Admin" help:"Auto-forwarding settings"`
	SendAs      GmailSendAsCmd      `cmd:"" name:"sendas" group:"Admin" help:"Send-as settings"`
	Vacation    GmailVacationCmd    `cmd:"" name:"vacation" group:"Admin" help:"Vacation responder"`
	Watch       GmailWatchCmd       `cmd:"" name:"watch" group:"Admin" help:"Manage Gmail watch"`
}
