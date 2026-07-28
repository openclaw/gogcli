package cmd

import (
	"testing"

	"google.golang.org/api/gmail/v1"
)

func verifiedSendAs(email string) *gmail.SendAs {
	return &gmail.SendAs{SendAsEmail: email, VerificationStatus: gmailVerificationAccepted}
}

func TestPickSendAsFromRecipients(t *testing.T) {
	sendAs := []*gmail.SendAs{
		verifiedSendAs("mail@kamuko.de"),
		verifiedSendAs("sales@kamuko.de"),
		{SendAsEmail: "pending@kamuko.de", VerificationStatus: "pending"},
	}

	cases := []struct {
		name string
		to   []string
		cc   []string
		want string
	}{
		{"to match wins", []string{"sales@kamuko.de"}, nil, "sales@kamuko.de"},
		{"cc used when to has no alias", []string{"someone@else.com"}, []string{"mail@kamuko.de"}, "mail@kamuko.de"},
		{"to beats cc", []string{"sales@kamuko.de"}, []string{"mail@kamuko.de"}, "sales@kamuko.de"},
		{"first to match wins", []string{"someone@else.com", "mail@kamuko.de", "sales@kamuko.de"}, nil, "mail@kamuko.de"},
		{"case-insensitive", []string{"Sales@Kamuko.de"}, nil, "sales@kamuko.de"},
		{"no match falls back to empty", []string{"someone@else.com"}, []string{"other@else.com"}, ""},
		{"unverified alias is skipped", []string{"pending@kamuko.de"}, nil, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := pickSendAsFromRecipients(tc.to, tc.cc, sendAs); got != tc.want {
				t.Fatalf("got %q, want %q", got, tc.want)
			}
		})
	}
}
