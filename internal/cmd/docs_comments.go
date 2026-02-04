package cmd

// DocsCommentsCmd embeds drive comments commands for Google Docs.
// Since Google Docs comments use the same Drive API endpoints,
// we reuse the drive comments implementation directly.
type DocsCommentsCmd struct {
	List    DriveCommentsListCmd    `cmd:"" name:"list" help:"List comments on a Google Doc"`
	Read    DriveCommentsGetCmd     `cmd:"" name:"read" help:"Read a comment with replies"`
	Create  DriveCommentsCreateCmd  `cmd:"" name:"create" help:"Create a comment on a Google Doc"`
	Reply   DriveCommentReplyCmd    `cmd:"" name:"reply" help:"Reply to a comment"`
	Resolve DriveCommentsResolveCmd `cmd:"" name:"resolve" help:"Mark a comment as resolved"`
	Delete  DriveCommentsDeleteCmd  `cmd:"" name:"delete" help:"Delete a comment"`
}
