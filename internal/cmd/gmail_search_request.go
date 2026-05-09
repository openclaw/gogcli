package cmd

import (
	"strings"

	"google.golang.org/api/gmail/v1"
)

type gmailSearchRequestOptions struct {
	query      string
	maxResults int64
	pageToken  string
	labelIDs   []string
}

func newGmailSearchRequestOptions(query string, maxResults int64, pageToken string) gmailSearchRequestOptions {
	query = strings.TrimSpace(query)
	return gmailSearchRequestOptions{
		query:      query,
		maxResults: maxResults,
		pageToken:  strings.TrimSpace(pageToken),
		labelIDs:   gmailQuerySystemLabelIDs(query),
	}
}

func applyGmailThreadListOptions(call *gmail.UsersThreadsListCall, opts gmailSearchRequestOptions) *gmail.UsersThreadsListCall {
	call = call.Q(opts.query).MaxResults(opts.maxResults)
	if len(opts.labelIDs) > 0 {
		call = call.LabelIds(opts.labelIDs...)
	}
	if opts.pageToken != "" {
		call = call.PageToken(opts.pageToken)
	}
	return call
}

func applyGmailMessageListOptions(call *gmail.UsersMessagesListCall, opts gmailSearchRequestOptions) *gmail.UsersMessagesListCall {
	call = call.Q(opts.query).MaxResults(opts.maxResults)
	if len(opts.labelIDs) > 0 {
		call = call.LabelIds(opts.labelIDs...)
	}
	if opts.pageToken != "" {
		call = call.PageToken(opts.pageToken)
	}
	return call
}
