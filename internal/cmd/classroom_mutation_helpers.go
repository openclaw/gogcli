package cmd

import (
	"context"
	"strings"

	"google.golang.org/api/classroom/v1"

	"github.com/openclaw/gogcli/internal/outfmt"
	"github.com/openclaw/gogcli/internal/ui"
)

type classroomDeleteOperation struct {
	op               string
	parentName       string
	parentPayloadKey string
	parentResultKey  string
	childName        string
	childPayloadKey  string
	childResultKey   string
	successResultKey string
	action           func(string, string) string
	delete           func(*classroom.Service, string, string) error
}

func runClassroomDelete(ctx context.Context, flags *RootFlags, rawParentID, rawChildID string, operation classroomDeleteOperation) error {
	parentID := strings.TrimSpace(rawParentID)
	childID := strings.TrimSpace(rawChildID)
	if parentID == "" {
		return usage("empty " + operation.parentName)
	}
	if childID == "" {
		return usage("empty " + operation.childName)
	}

	if err := dryRunAndConfirmDestructive(ctx, flags, operation.op, map[string]any{
		operation.parentPayloadKey: parentID,
		operation.childPayloadKey:  childID,
	}, operation.action(parentID, childID)); err != nil {
		return err
	}

	_, svc, err := requireClassroomService(ctx, flags)
	if err != nil {
		return wrapClassroomError(err)
	}
	if err := operation.delete(svc, parentID, childID); err != nil {
		return wrapClassroomError(err)
	}

	return writeResult(ctx, ui.FromContext(ctx),
		kv(operation.successResultKey, true),
		kv(operation.parentResultKey, parentID),
		kv(operation.childResultKey, childID),
	)
}

type classroomAssigneeOperation[Request, Result any] struct {
	op                 string
	itemName           string
	itemPayloadKey     string
	jsonKey            string
	buildRequest       func(string, *classroom.ModifyIndividualStudentsOptions) (Request, error)
	mutate             func(*classroom.Service, string, string, Request) (*Result, error)
	resultID           func(*Result) string
	resultAssigneeMode func(*Result) string
}

func validateClassroomAssigneeChanges(mode string, options *classroom.ModifyIndividualStudentsOptions) error {
	if mode == "" && options == nil {
		return usage("no assignee changes specified")
	}
	return nil
}

func runClassroomAssigneeMutation[Request, Result any](
	ctx context.Context,
	flags *RootFlags,
	rawCourseID string,
	rawItemID string,
	mode string,
	addStudents []string,
	removeStudents []string,
	operation classroomAssigneeOperation[Request, Result],
) error {
	courseID := strings.TrimSpace(rawCourseID)
	itemID := strings.TrimSpace(rawItemID)
	if courseID == "" {
		return usage("empty courseId")
	}
	if itemID == "" {
		return usage("empty " + operation.itemName)
	}

	assigneeMode, individual, err := normalizeAssigneeMode(mode, addStudents, removeStudents)
	if err != nil {
		return usage(err.Error())
	}
	request, err := operation.buildRequest(assigneeMode, individual)
	if err != nil {
		return err
	}
	if dryErr := dryRunExit(ctx, flags, operation.op, map[string]any{
		"course_id":              courseID,
		operation.itemPayloadKey: itemID,
		"request":                request,
	}); dryErr != nil {
		return dryErr
	}

	_, svc, err := requireClassroomService(ctx, flags)
	if err != nil {
		return wrapClassroomError(err)
	}
	updated, err := operation.mutate(svc, courseID, itemID, request)
	if err != nil {
		return wrapClassroomError(err)
	}
	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(ctx, stdoutWriter(ctx), map[string]any{operation.jsonKey: updated})
	}
	u := ui.FromContext(ctx)
	u.Out().Linef("id\t%s", operation.resultID(updated))
	u.Out().Linef("assignee_mode\t%s", operation.resultAssigneeMode(updated))
	return nil
}
