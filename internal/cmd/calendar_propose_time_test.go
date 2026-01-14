package cmd

import (
	"encoding/base64"
	"testing"
)

func TestProposeTimeURLGeneration(t *testing.T) {
	tests := []struct {
		name       string
		eventID    string
		calendarID string
		wantURL    string
	}{
		{
			name:       "basic event",
			eventID:    "rp2rg301pirvlufurh62sfkh74",
			calendarID: "vladimir.novosselov@gmail.com",
			wantURL:    "https://calendar.google.com/calendar/u/0/r/proposetime/cnAycmczMDFwaXJ2bHVmdXJoNjJzZmtoNzQgdmxhZGltaXIubm92b3NzZWxvdkBnbWFpbC5jb20=",
		},
		{
			name:       "simple ids",
			eventID:    "evt123",
			calendarID: "test@example.com",
			wantURL:    "https://calendar.google.com/calendar/u/0/r/proposetime/" + base64.StdEncoding.EncodeToString([]byte("evt123 test@example.com")),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			payload := tt.eventID + " " + tt.calendarID
			encoded := base64.StdEncoding.EncodeToString([]byte(payload))
			got := "https://calendar.google.com/calendar/u/0/r/proposetime/" + encoded

			if got != tt.wantURL {
				t.Errorf("URL mismatch:\ngot:  %s\nwant: %s", got, tt.wantURL)
			}
		})
	}
}
