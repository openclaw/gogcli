package cmd

import (
	"context"
	"strings"

	"google.golang.org/api/gmail/v1"
)

const snoozeLabelName = "Snoozed"

// ensureSnoozedLabel resolves or creates the "Snoozed" label and returns its ID.
func ensureSnoozedLabel(ctx context.Context, svc *gmail.Service) (string, error) {
	idMap, err := fetchLabelNameToID(svc)
	if err != nil {
		return "", err
	}
	if id, ok := idMap[strings.ToLower(snoozeLabelName)]; ok {
		return id, nil
	}
	label, err := createLabel(ctx, svc, snoozeLabelName)
	if err != nil {
		return "", err
	}
	return label.Id, nil
}

// snoozeApply removes the INBOX label and adds the Snoozed label on a thread.
func snoozeApply(ctx context.Context, svc *gmail.Service, threadID, snoozedLabelID string) error {
	_, err := svc.Users.Threads.Modify("me", threadID, &gmail.ModifyThreadRequest{
		RemoveLabelIds: []string{"INBOX"},
		AddLabelIds:    []string{snoozedLabelID},
	}).Context(ctx).Do()
	return err
}

// snoozeRestore adds the INBOX label and removes the Snoozed label on a thread.
func snoozeRestore(ctx context.Context, svc *gmail.Service, threadID, snoozedLabelID string) error {
	_, err := svc.Users.Threads.Modify("me", threadID, &gmail.ModifyThreadRequest{
		AddLabelIds:    []string{"INBOX"},
		RemoveLabelIds: []string{snoozedLabelID},
	}).Context(ctx).Do()
	return err
}

// fetchThreadSubject retrieves the Subject header from the first message of a thread.
// Returns an empty string (not an error) when the thread has no messages or no Subject header.
func fetchThreadSubject(ctx context.Context, svc *gmail.Service, threadID string) (string, error) {
	thread, err := svc.Users.Threads.Get("me", threadID).
		Format("metadata").
		MetadataHeaders("Subject").
		Context(ctx).
		Do()
	if err != nil {
		return "", err
	}
	if thread == nil || len(thread.Messages) == 0 {
		return "", nil
	}
	first := thread.Messages[0]
	if first == nil || first.Payload == nil {
		return "", nil
	}
	return headerValue(first.Payload, "Subject"), nil
}
