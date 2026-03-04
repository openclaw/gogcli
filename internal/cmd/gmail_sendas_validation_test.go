package cmd

import (
	"testing"

	"google.golang.org/api/gmail/v1"
)

func TestSendAsAllowedForFrom(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		sa   *gmail.SendAs
		want bool
	}{
		{
			name: "accepted status",
			sa: &gmail.SendAs{
				VerificationStatus: "accepted",
			},
			want: true,
		},
		{
			name: "accepted status case-insensitive",
			sa: &gmail.SendAs{
				VerificationStatus: "ACCEPTED",
			},
			want: true,
		},
		{
			name: "workspace alias with empty status and gmail-managed delivery",
			sa: &gmail.SendAs{
				VerificationStatus: "",
			},
			want: true,
		},
		{
			name: "empty status with smtp relay is not allowed",
			sa: &gmail.SendAs{
				VerificationStatus: "",
				SmtpMsa: &gmail.SmtpMsa{
					Host: "smtp.example.com",
				},
			},
			want: false,
		},
		{
			name: "pending status is not allowed",
			sa: &gmail.SendAs{
				VerificationStatus: "pending",
			},
			want: false,
		},
		{
			name: "nil send-as",
			sa:   nil,
			want: false,
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got := sendAsAllowedForFrom(tc.sa)
			if got != tc.want {
				t.Fatalf("sendAsAllowedForFrom() = %t, want %t", got, tc.want)
			}
		})
	}
}
