package docssed

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"google.golang.org/api/docs/v1"
)

var (
	errDeferredBulletGet    = errors.New("deferred bullet get failed")
	errDeferredBulletUpdate = errors.New("deferred bullet update failed")
)

type deferredBulletBackend struct {
	documents   []*docs.Document
	getCalls    int
	updateCalls int
	updates     [][]*docs.Request
	getErr      error
	updateErr   error
}

func (b *deferredBulletBackend) Get(_ context.Context, _ string) (*docs.Document, error) {
	if b.getErr != nil {
		return nil, b.getErr
	}

	if b.getCalls >= len(b.documents) {
		return b.documents[len(b.documents)-1], nil
	}

	document := b.documents[b.getCalls]
	b.getCalls++

	return document, nil
}

func (b *deferredBulletBackend) BatchUpdate(
	_ context.Context,
	_ string,
	requests []*docs.Request,
) (*docs.BatchUpdateDocumentResponse, error) {
	b.updateCalls++
	b.updates = append(b.updates, requests)

	if b.updateErr != nil {
		return nil, b.updateErr
	}

	return &docs.BatchUpdateDocumentResponse{}, nil
}

func TestPlanDeferredBulletsNoPendingTabs(t *testing.T) {
	t.Parallel()

	document := deferredBulletDocument(
		deferredBulletParagraphInput{start: 1, end: 8, text: "plain\n"},
	)

	plan := PlanDeferredBullets(document)
	if len(plan.Requests) != 0 || plan.More {
		t.Fatalf("plan = %#v, want no-op", plan)
	}
}

func TestPlanDeferredBulletsIncludesExistingNumberedParent(t *testing.T) {
	t.Parallel()

	document := deferredBulletDocument(
		deferredBulletParagraphInput{
			start:  1,
			end:    10,
			text:   "Parent\n",
			listID: "numbered",
		},
		deferredBulletParagraphInput{start: 10, end: 22, text: "\tChild\n"},
	)
	document.Lists = map[string]docs.List{
		"numbered": {
			ListProperties: &docs.ListProperties{
				NestingLevels: []*docs.NestingLevel{{GlyphType: "DECIMAL"}},
			},
		},
	}

	plan := PlanDeferredBullets(document)
	assertDeferredBulletPlan(
		t,
		plan,
		1,
		21,
		deferredBulletPresetNumbered,
		false,
	)
}

func TestPlanDeferredBulletsSplitsGroups(t *testing.T) {
	t.Parallel()

	document := deferredBulletDocument(
		deferredBulletParagraphInput{start: 1, end: 10, text: "\tFirst\n"},
		deferredBulletParagraphInput{start: 10, end: 20, text: "Boundary\n"},
		deferredBulletParagraphInput{start: 20, end: 30, text: "\tSecond\n"},
	)

	plan := PlanDeferredBullets(document)
	assertDeferredBulletPlan(t, plan, 1, 9, deferredBulletPresetDisc, true)
}

func TestPlanDeferredBulletsSplitsDifferingPresets(t *testing.T) {
	t.Parallel()

	document := deferredBulletDocument(
		deferredBulletParagraphInput{
			start:  1,
			end:    10,
			text:   "Numbered parent\n",
			listID: "numbered",
		},
		deferredBulletParagraphInput{start: 10, end: 20, text: "\tNumbered child\n"},
		deferredBulletParagraphInput{
			start:  20,
			end:    30,
			text:   "Bullet parent\n",
			listID: "bullet",
		},
		deferredBulletParagraphInput{start: 30, end: 40, text: "\tBullet child\n"},
	)
	document.Lists = map[string]docs.List{
		"numbered": {
			ListProperties: &docs.ListProperties{
				NestingLevels: []*docs.NestingLevel{{GlyphType: "ROMAN"}},
			},
		},
		"bullet": {
			ListProperties: &docs.ListProperties{
				NestingLevels: []*docs.NestingLevel{{GlyphType: "GLYPH_TYPE_UNSPECIFIED"}},
			},
		},
	}

	plan := PlanDeferredBullets(document)
	assertDeferredBulletPlan(
		t,
		plan,
		1,
		19,
		deferredBulletPresetNumbered,
		true,
	)
}

func TestPlanDeferredBulletsSkipsInvalidFirstGroup(t *testing.T) {
	t.Parallel()

	document := deferredBulletDocument(
		deferredBulletParagraphInput{start: 1, end: 2, text: "\tInvalid\n"},
		deferredBulletParagraphInput{start: 2, end: 10, text: "Boundary\n"},
		deferredBulletParagraphInput{start: 10, end: 20, text: "\tValid\n"},
	)

	plan := PlanDeferredBullets(document)
	assertDeferredBulletPlan(t, plan, 10, 19, deferredBulletPresetDisc, false)
}

func TestDeferredBulletRequestsClampsBodyEnd(t *testing.T) {
	t.Parallel()

	requests := deferredBulletRequests(deferredBulletGroup{
		startIndex: 1,
		endIndex:   30,
		preset:     deferredBulletPresetDisc,
	}, 20)
	assertDeferredBulletPlan(t, DeferredBulletPlan{Requests: requests}, 1, 19, deferredBulletPresetDisc, false)
}

func TestInferDeferredBulletPreset(t *testing.T) {
	t.Parallel()

	document := &docs.Document{}
	if got := inferDeferredBulletPreset(document, "missing"); got != deferredBulletPresetDisc {
		t.Fatalf("missing list preset = %q, want %q", got, deferredBulletPresetDisc)
	}

	document.Lists = map[string]docs.List{
		"alpha": {
			ListProperties: &docs.ListProperties{
				NestingLevels: []*docs.NestingLevel{{GlyphType: "UPPER_ALPHA"}},
			},
		},
	}
	if got := inferDeferredBulletPreset(document, "alpha"); got != deferredBulletPresetNumbered {
		t.Fatalf("alpha preset = %q, want %q", got, deferredBulletPresetNumbered)
	}
}

func TestExecutorApplyDeferredBulletsRefetchesBetweenGroups(t *testing.T) {
	t.Parallel()

	first := deferredBulletDocument(
		deferredBulletParagraphInput{start: 1, end: 10, text: "\tFirst\n"},
		deferredBulletParagraphInput{start: 10, end: 20, text: "Boundary\n"},
		deferredBulletParagraphInput{start: 20, end: 30, text: "\tSecond\n"},
	)
	second := deferredBulletDocument(
		deferredBulletParagraphInput{start: 1, end: 10, text: "Boundary\n"},
		deferredBulletParagraphInput{start: 10, end: 20, text: "\tSecond\n"},
	)
	backend := &deferredBulletBackend{documents: []*docs.Document{first, second}}

	err := NewExecutor(backend).ApplyDeferredBullets(context.Background(), "doc")
	if err != nil {
		t.Fatalf("ApplyDeferredBullets: %v", err)
	}

	if backend.getCalls != 2 || backend.updateCalls != 2 {
		t.Fatalf("get/update calls = %d/%d, want 2/2", backend.getCalls, backend.updateCalls)
	}

	wantRanges := []*docs.Range{
		{StartIndex: 1, EndIndex: 9},
		{StartIndex: 10, EndIndex: 19},
	}

	for index, requests := range backend.updates {
		if len(requests) != 2 {
			t.Fatalf("update %d requests = %d, want 2", index, len(requests))
		}

		if !reflect.DeepEqual(requests[0].DeleteParagraphBullets.Range, wantRanges[index]) {
			t.Fatalf(
				"update %d delete range = %#v, want %#v",
				index,
				requests[0].DeleteParagraphBullets.Range,
				wantRanges[index],
			)
		}

		if !reflect.DeepEqual(requests[1].CreateParagraphBullets.Range, wantRanges[index]) {
			t.Fatalf(
				"update %d create range = %#v, want %#v",
				index,
				requests[1].CreateParagraphBullets.Range,
				wantRanges[index],
			)
		}
	}
}

func TestExecutorApplyDeferredBulletsNoOpSkipsUpdate(t *testing.T) {
	t.Parallel()

	backend := &deferredBulletBackend{documents: []*docs.Document{
		deferredBulletDocument(
			deferredBulletParagraphInput{start: 1, end: 10, text: "Plain\n"},
		),
	}}

	err := NewExecutor(backend).ApplyDeferredBullets(context.Background(), "doc")
	if err != nil {
		t.Fatalf("ApplyDeferredBullets: %v", err)
	}

	if backend.getCalls != 1 || backend.updateCalls != 0 {
		t.Fatalf("get/update calls = %d/%d, want 1/0", backend.getCalls, backend.updateCalls)
	}
}

func TestExecutorApplyDeferredBulletsReturnsGetCause(t *testing.T) {
	t.Parallel()

	backend := &deferredBulletBackend{getErr: errDeferredBulletGet}

	err := NewExecutor(backend).ApplyDeferredBullets(context.Background(), "doc")
	if !errors.Is(err, errDeferredBulletGet) {
		t.Fatalf("error = %v, want get cause", err)
	}
}

func TestExecutorApplyDeferredBulletsReturnsUpdateCause(t *testing.T) {
	t.Parallel()

	backend := &deferredBulletBackend{
		documents: []*docs.Document{
			deferredBulletDocument(
				deferredBulletParagraphInput{start: 1, end: 10, text: "\tFirst\n"},
			),
		},
		updateErr: errDeferredBulletUpdate,
	}

	err := NewExecutor(backend).ApplyDeferredBullets(context.Background(), "doc")
	if !errors.Is(err, errDeferredBulletUpdate) {
		t.Fatalf("error = %v, want update cause", err)
	}
}

type deferredBulletParagraphInput struct {
	start  int64
	end    int64
	text   string
	listID string
}

func deferredBulletDocument(inputs ...deferredBulletParagraphInput) *docs.Document {
	content := make([]*docs.StructuralElement, 0, len(inputs))
	for _, input := range inputs {
		paragraph := &docs.Paragraph{
			Elements: []*docs.ParagraphElement{{
				StartIndex: input.start,
				EndIndex:   input.end,
				TextRun:    &docs.TextRun{Content: input.text},
			}},
		}
		if input.listID != "" {
			paragraph.Bullet = &docs.Bullet{ListId: input.listID}
		}
		content = append(content, &docs.StructuralElement{
			StartIndex: input.start,
			EndIndex:   input.end,
			Paragraph:  paragraph,
		})
	}

	return &docs.Document{
		DocumentId: "doc",
		Body:       &docs.Body{Content: content},
	}
}

func assertDeferredBulletPlan(
	t *testing.T,
	plan DeferredBulletPlan,
	startIndex int64,
	endIndex int64,
	preset string,
	more bool,
) {
	t.Helper()

	if len(plan.Requests) != 2 {
		t.Fatalf("requests = %d, want 2", len(plan.Requests))
	}

	deleteRequest := plan.Requests[0].DeleteParagraphBullets
	createRequest := plan.Requests[1].CreateParagraphBullets

	if deleteRequest == nil || createRequest == nil {
		t.Fatalf("requests = %#v, want delete then create bullets", plan.Requests)
	}

	wantRange := &docs.Range{StartIndex: startIndex, EndIndex: endIndex}
	if !reflect.DeepEqual(deleteRequest.Range, wantRange) {
		t.Fatalf("delete range = %#v, want %#v", deleteRequest.Range, wantRange)
	}

	if !reflect.DeepEqual(createRequest.Range, wantRange) {
		t.Fatalf("create range = %#v, want %#v", createRequest.Range, wantRange)
	}

	if createRequest.BulletPreset != preset || plan.More != more {
		t.Fatalf(
			"preset/more = %q/%t, want %q/%t",
			createRequest.BulletPreset,
			plan.More,
			preset,
			more,
		)
	}
}
