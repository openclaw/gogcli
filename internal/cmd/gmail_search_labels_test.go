package cmd

import "testing"

func TestGmailQuerySystemLabelIDs(t *testing.T) {
	tests := []struct {
		name  string
		query string
		want  []string
	}{
		{name: "positive conjunction", query: "in:spam is:unread", want: []string{"SPAM", "UNREAD"}},
		{name: "positive group", query: "(in:inbox is:starred) from:a@b.com", want: []string{"INBOX", "STARRED"}},
		{name: "duplicate terms", query: "in:inbox label:inbox", want: []string{"INBOX"}},
		{name: "dash negation", query: "in:sent -in:inbox", want: []string{"SENT"}},
		{name: "dash negation in brackets", query: "in:sent (-in:inbox)", want: []string{"SENT"}},
		{name: "NOT negation", query: "in:sent NOT in:inbox", want: []string{"SENT"}},
		{name: "NOT negation lowercase", query: "in:sent not in:inbox", want: []string{"SENT"}},
		{name: "NOT against a bracket", query: "in:sent NOT(in:inbox)", want: []string{"SENT"}},
		{name: "NOT group", query: "NOT (zzqx in:inbox) in:sent", want: []string{"SENT"}},
		{name: "NOT group against a bracket", query: "NOT(zzqx in:inbox is:unread) in:sent", want: []string{"SENT"}},
		{name: "NOT with a complete inline group then a positive term", query: "in:spam NOT(in:inbox) is:unread", want: []string{"SPAM", "UNREAD"}},
		{name: "NOT with a complete inline group then a positive term, spaced", query: "in:spam NOT (in:inbox) is:unread", want: []string{"SPAM", "UNREAD"}},
		{name: "quoted phrase containing not", query: `subject:"not for sale" in:inbox`, want: []string{"INBOX"}},
		{name: "quoted word not", query: `in:spam "not" is:unread`, want: []string{"SPAM", "UNREAD"}},
		{name: "quoted word OR", query: `in:spam is:unread "OR"`, want: []string{"SPAM", "UNREAD"}},
		{name: "quoted phrase containing OR and a pipe", query: `subject:"a OR b | c" in:inbox`, want: []string{"INBOX"}},
		{name: "quoted phrase with brackets", query: `subject:"(draft" in:inbox`, want: []string{"INBOX"}},
		{name: "quoted negation phrase", query: `"not in:inbox" is:unread`, want: []string{"UNREAD"}},
		{name: "word starting with not", query: "nothing in:inbox", want: []string{"INBOX"}},
		{name: "unterminated quote", query: `subject:"oops in:inbox`, want: nil},
		{name: "negated group", query: "-(zzqx in:inbox) in:sent", want: []string{"SENT"}},
		{name: "negated square group", query: "-[zzqx in:inbox] in:sent", want: []string{"SENT"}},
		{name: "negated nested group", query: "-(zzqx (in:inbox is:unread) in:spam) in:sent", want: []string{"SENT"}},
		{name: "negated field group", query: "-subject:(zzqx in:inbox) in:sent", want: []string{"SENT"}},
		{name: "term after a negated group closes", query: "-(zzqx) in:inbox", want: []string{"INBOX"}},
		{name: "OR", query: "in:sent OR in:inbox", want: nil},
		{name: "OR against a bracket", query: "in:sent OR(in:inbox)", want: nil},
		{name: "OR inside a group", query: "(in:sent OR in:inbox) is:unread", want: nil},
		{name: "pipe", query: "in:sent | in:inbox", want: nil},
		{name: "pipe without spaces", query: "in:sent|in:inbox", want: nil},
		{name: "braces", query: "{in:sent in:inbox}", want: nil},
		{name: "no labels", query: "from:a@b.com newer_than:7d", want: nil},
		{name: "empty", query: "", want: nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assertSameStrings(t, gmailQuerySystemLabelIDs(tt.query), tt.want)
		})
	}
}
