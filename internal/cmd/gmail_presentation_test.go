package cmd

import (
	"strings"
	"testing"

	"google.golang.org/api/gmail/v1"
)

func TestGmailPresentationSchemas(t *testing.T) {
	t.Parallel()

	t.Run("drafts", func(t *testing.T) {
		t.Parallel()
		got := renderPlainTable(t, []*gmail.Draft{
			{Id: "d1", Message: &gmail.Message{Id: "m1"}},
			{Id: "d2"},
		}, gmailDraftColumns())
		assertTableOutput(t, got, "ID\tMESSAGE_ID\nd1\tm1\nd2\t\n")
	})

	t.Run("labels", func(t *testing.T) {
		t.Parallel()
		got := renderPlainTable(t, []*gmail.Label{{
			Id:   "Label_1",
			Name: "Custom",
			Type: "user",
		}}, gmailLabelColumns())
		assertTableOutput(t, got, "ID\tNAME\tTYPE\nLabel_1\tCustom\tuser\n")
	})

	t.Run("messages", func(t *testing.T) {
		t.Parallel()
		item := messageItem{
			ID:       "m1",
			ThreadID: "t1",
			Date:     "2026-06-12",
			From:     "sender@example.com",
			Subject:  "Receipt",
			Labels:   []string{"INBOX", "Work"},
		}
		got := renderPlainTable(t, []messageItem{item}, gmailMessageColumns(false, false))
		assertTableOutput(
			t,
			got,
			"ID\tTHREAD\tDATE\tFROM\tSUBJECT\tLABELS\n"+
				"m1\tt1\t2026-06-12\tsender@example.com\tReceipt\tINBOX,Work\n",
		)
	})

	t.Run("messages with body", func(t *testing.T) {
		t.Parallel()
		body := strings.Repeat("x", 201) + "\nmore"
		got := renderPlainTable(t, []messageItem{{
			ID:   "m1",
			Body: body,
		}}, gmailMessageColumns(true, false))
		assertTableOutput(
			t,
			got,
			"ID\tTHREAD\tDATE\tFROM\tSUBJECT\tLABELS\tBODY\n"+
				"m1\t\t\t\t\t\t"+strings.Repeat("x", 200)+gmailTextTruncationMarker+"\n",
		)
	})

	t.Run("threads", func(t *testing.T) {
		t.Parallel()
		got := renderPlainTable(t, []threadItem{
			{ID: "t1", Date: "2026-06-12", From: "sender@example.com", Subject: "One", Labels: []string{"INBOX"}, MessageCount: 1},
			{ID: "t2", Date: "2026-06-13", From: "sender@example.com", Subject: "Many", MessageCount: 3},
		}, gmailThreadColumns())
		assertTableOutput(
			t,
			got,
			"ID\tDATE\tFROM\tSUBJECT\tLABELS\tTHREAD\n"+
				"t1\t2026-06-12\tsender@example.com\tOne\tINBOX\t-\n"+
				"t2\t2026-06-13\tsender@example.com\tMany\t\t[3 msgs]\n",
		)
	})

	t.Run("email status", func(t *testing.T) {
		t.Parallel()
		got := renderPlainTable(t, []gmailEmailStatusRow{{
			Email:  "user\texample.com",
			Status: "pending\tverification",
		}}, gmailEmailStatusColumns())
		assertTableOutput(t, got, "EMAIL\tSTATUS\nuser example.com\tpending verification\n")
	})
}

func TestCompactGmailRows(t *testing.T) {
	t.Parallel()

	label := &gmail.Label{Id: "Label_1"}
	rows := compactGmailRows([]*gmail.Label{nil, label, nil})
	if len(rows) != 1 || rows[0] != label {
		t.Fatalf("rows = %#v, want only Label_1", rows)
	}
}
