package cmd

// GmailTrackCmd groups tracking-related subcommands
type GmailTrackCmd struct {
	Setup  GmailTrackSetupCmd  `cmd:"" help:"Set up email tracking (deploy Cloudflare Worker)"`
	Opens  GmailTrackOpensCmd  `cmd:"" help:"Query email opens"`
	Status GmailTrackStatusCmd `cmd:"" help:"Show tracking configuration status"`
	Key    GmailTrackKeyCmd    `cmd:"" help:"Manage tracking encryption keys"`
}
