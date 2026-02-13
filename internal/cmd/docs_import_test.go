package cmd

import (
	"testing"

	"google.golang.org/api/docs/v1"
)

// ---------------------------------------------------------------------------
// extractMarkdownImages
// ---------------------------------------------------------------------------

func TestExtractMarkdownImages_NoImages(t *testing.T) {
	content := "Hello world, no images here."
	cleaned, images := extractMarkdownImages(content)
	if cleaned != content {
		t.Fatalf("expected content unchanged, got %q", cleaned)
	}
	if len(images) != 0 {
		t.Fatalf("expected 0 images, got %d", len(images))
	}
}

func TestExtractMarkdownImages_SingleImage(t *testing.T) {
	content := "![alt text](image.png)"
	cleaned, images := extractMarkdownImages(content)
	if cleaned != "<<IMG_0>>" {
		t.Fatalf("expected <<IMG_0>>, got %q", cleaned)
	}
	if len(images) != 1 {
		t.Fatalf("expected 1 image, got %d", len(images))
	}
	if images[0].alt != "alt text" {
		t.Fatalf("expected alt 'alt text', got %q", images[0].alt)
	}
	if images[0].originalRef != "image.png" {
		t.Fatalf("expected ref 'image.png', got %q", images[0].originalRef)
	}
	if images[0].index != 0 {
		t.Fatalf("expected index 0, got %d", images[0].index)
	}
}

func TestExtractMarkdownImages_MultipleImages(t *testing.T) {
	content := "![a](one.png) text ![b](two.jpg) more ![c](three.gif)"
	cleaned, images := extractMarkdownImages(content)
	if len(images) != 3 {
		t.Fatalf("expected 3 images, got %d", len(images))
	}
	want := "<<IMG_0>> text <<IMG_1>> more <<IMG_2>>"
	if cleaned != want {
		t.Fatalf("expected %q, got %q", want, cleaned)
	}
	for i, img := range images {
		if img.index != i {
			t.Fatalf("image %d: expected index %d, got %d", i, i, img.index)
		}
	}
	if images[0].alt != "a" || images[1].alt != "b" || images[2].alt != "c" {
		t.Fatalf("unexpected alt texts: %q %q %q", images[0].alt, images[1].alt, images[2].alt)
	}
	if images[0].originalRef != "one.png" || images[1].originalRef != "two.jpg" || images[2].originalRef != "three.gif" {
		t.Fatalf("unexpected refs: %q %q %q", images[0].originalRef, images[1].originalRef, images[2].originalRef)
	}
}

func TestExtractMarkdownImages_RemoteURL(t *testing.T) {
	tests := []struct {
		name string
		url  string
	}{
		{"https", "https://example.com/photo.png"},
		{"http", "http://cdn.example.com/img.jpg"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			content := "![photo](" + tc.url + ")"
			cleaned, images := extractMarkdownImages(content)
			if cleaned != "<<IMG_0>>" {
				t.Fatalf("expected <<IMG_0>>, got %q", cleaned)
			}
			if len(images) != 1 {
				t.Fatalf("expected 1 image, got %d", len(images))
			}
			if images[0].originalRef != tc.url {
				t.Fatalf("expected ref %q, got %q", tc.url, images[0].originalRef)
			}
		})
	}
}

func TestExtractMarkdownImages_LocalFilePath(t *testing.T) {
	tests := []struct {
		name string
		path string
	}{
		{"relative", "images/photo.png"},
		{"relative_dot", "./images/photo.png"},
		{"absolute", "/home/user/photo.png"},
		{"just_filename", "photo.png"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			content := "![img](" + tc.path + ")"
			_, images := extractMarkdownImages(content)
			if len(images) != 1 {
				t.Fatalf("expected 1 image, got %d", len(images))
			}
			if images[0].originalRef != tc.path {
				t.Fatalf("expected ref %q, got %q", tc.path, images[0].originalRef)
			}
		})
	}
}

func TestExtractMarkdownImages_WithTitleText(t *testing.T) {
	content := `![alt](image.png "My Title")`
	cleaned, images := extractMarkdownImages(content)
	if cleaned != "<<IMG_0>>" {
		t.Fatalf("expected <<IMG_0>>, got %q", cleaned)
	}
	if len(images) != 1 {
		t.Fatalf("expected 1 image, got %d", len(images))
	}
	if images[0].originalRef != "image.png" {
		t.Fatalf("expected ref 'image.png', got %q", images[0].originalRef)
	}
	if images[0].alt != "alt" {
		t.Fatalf("expected alt 'alt', got %q", images[0].alt)
	}
}

func TestExtractMarkdownImages_MixedContent(t *testing.T) {
	content := "# Heading\n\nSome text before.\n\n![first](a.png)\n\nMiddle paragraph.\n\n![second](b.jpg)\n\nText after images."
	cleaned, images := extractMarkdownImages(content)
	if len(images) != 2 {
		t.Fatalf("expected 2 images, got %d", len(images))
	}
	want := "# Heading\n\nSome text before.\n\n<<IMG_0>>\n\nMiddle paragraph.\n\n<<IMG_1>>\n\nText after images."
	if cleaned != want {
		t.Fatalf("expected:\n%s\ngot:\n%s", want, cleaned)
	}
}

func TestExtractMarkdownImages_EmptyAltText(t *testing.T) {
	content := "![](image.png)"
	cleaned, images := extractMarkdownImages(content)
	if cleaned != "<<IMG_0>>" {
		t.Fatalf("expected <<IMG_0>>, got %q", cleaned)
	}
	if len(images) != 1 {
		t.Fatalf("expected 1 image, got %d", len(images))
	}
	if images[0].alt != "" {
		t.Fatalf("expected empty alt, got %q", images[0].alt)
	}
}

func TestExtractMarkdownImages_SpecialCharsInURL(t *testing.T) {
	content := "![pic](https://example.com/path/to/image%20name.png?v=2&size=large)"
	cleaned, images := extractMarkdownImages(content)
	if cleaned != "<<IMG_0>>" {
		t.Fatalf("expected <<IMG_0>>, got %q", cleaned)
	}
	if len(images) != 1 {
		t.Fatalf("expected 1 image, got %d", len(images))
	}
	if images[0].originalRef != "https://example.com/path/to/image%20name.png?v=2&size=large" {
		t.Fatalf("unexpected ref %q", images[0].originalRef)
	}
}

func TestExtractMarkdownImages_PlaceholderFormat(t *testing.T) {
	content := "![a](one.png) ![b](two.png) ![c](three.png)"
	cleaned, images := extractMarkdownImages(content)
	_ = images
	if cleaned != "<<IMG_0>> <<IMG_1>> <<IMG_2>>" {
		t.Fatalf("unexpected placeholder format: %q", cleaned)
	}
}

func TestExtractMarkdownImages_EmptyContent(t *testing.T) {
	cleaned, images := extractMarkdownImages("")
	if cleaned != "" {
		t.Fatalf("expected empty string, got %q", cleaned)
	}
	if len(images) != 0 {
		t.Fatalf("expected 0 images, got %d", len(images))
	}
}

// ---------------------------------------------------------------------------
// markdownImage methods
// ---------------------------------------------------------------------------

func TestMarkdownImage_Placeholder(t *testing.T) {
	tests := []struct {
		index int
		want  string
	}{
		{0, "<<IMG_0>>"},
		{1, "<<IMG_1>>"},
		{5, "<<IMG_5>>"},
		{42, "<<IMG_42>>"},
	}
	for _, tc := range tests {
		img := markdownImage{index: tc.index}
		got := img.placeholder()
		if got != tc.want {
			t.Errorf("index %d: placeholder() = %q, want %q", tc.index, got, tc.want)
		}
	}
}

func TestMarkdownImage_IsRemote(t *testing.T) {
	tests := []struct {
		name string
		ref  string
		want bool
	}{
		{"https URL", "https://example.com/img.png", true},
		{"http URL", "http://example.com/img.png", true},
		{"local relative", "images/photo.png", false},
		{"local absolute", "/home/user/photo.png", false},
		{"relative dot", "./photo.png", false},
		{"ftp not remote", "ftp://server/img.png", false},
		{"empty string", "", false},
		{"https with path", "https://cdn.example.com/a/b/c.jpg?q=1", true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			img := markdownImage{originalRef: tc.ref}
			got := img.isRemote()
			if got != tc.want {
				t.Errorf("isRemote(%q) = %v, want %v", tc.ref, got, tc.want)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// findPlaceholderIndices
// ---------------------------------------------------------------------------

func TestFindPlaceholderIndices_NilDocument(t *testing.T) {
	result := findPlaceholderIndices(nil, 1)
	if len(result) != 0 {
		t.Fatalf("expected empty map for nil doc, got %d entries", len(result))
	}
}

func TestFindPlaceholderIndices_NilBody(t *testing.T) {
	doc := &docs.Document{}
	result := findPlaceholderIndices(doc, 1)
	if len(result) != 0 {
		t.Fatalf("expected empty map for nil body, got %d entries", len(result))
	}
}

func TestFindPlaceholderIndices_EmptyDocument(t *testing.T) {
	doc := &docs.Document{
		Body: &docs.Body{
			Content: []*docs.StructuralElement{},
		},
	}
	result := findPlaceholderIndices(doc, 1)
	if len(result) != 0 {
		t.Fatalf("expected empty map for empty doc, got %d entries", len(result))
	}
}

func TestFindPlaceholderIndices_CountZero(t *testing.T) {
	doc := &docs.Document{
		Body: &docs.Body{
			Content: []*docs.StructuralElement{
				{
					Paragraph: &docs.Paragraph{
						Elements: []*docs.ParagraphElement{
							{
								StartIndex: 0,
								TextRun:    &docs.TextRun{Content: "<<IMG_0>>"},
							},
						},
					},
				},
			},
		},
	}
	result := findPlaceholderIndices(doc, 0)
	if len(result) != 0 {
		t.Fatalf("expected empty map for count=0, got %d entries", len(result))
	}
}

func TestFindPlaceholderIndices_NoPlaceholders(t *testing.T) {
	doc := &docs.Document{
		Body: &docs.Body{
			Content: []*docs.StructuralElement{
				{
					Paragraph: &docs.Paragraph{
						Elements: []*docs.ParagraphElement{
							{
								StartIndex: 0,
								TextRun:    &docs.TextRun{Content: "Just some regular text."},
							},
						},
					},
				},
			},
		},
	}
	result := findPlaceholderIndices(doc, 2)
	if len(result) != 0 {
		t.Fatalf("expected empty map for doc with no placeholders, got %d entries", len(result))
	}
}

func TestFindPlaceholderIndices_SinglePlaceholder(t *testing.T) {
	doc := &docs.Document{
		Body: &docs.Body{
			Content: []*docs.StructuralElement{
				{
					Paragraph: &docs.Paragraph{
						Elements: []*docs.ParagraphElement{
							{
								StartIndex: 1,
								TextRun:    &docs.TextRun{Content: "<<IMG_0>>"},
							},
						},
					},
				},
			},
		},
	}
	result := findPlaceholderIndices(doc, 1)
	if len(result) != 1 {
		t.Fatalf("expected 1 placeholder, got %d", len(result))
	}
	dr, ok := result["<<IMG_0>>"]
	if !ok {
		t.Fatalf("<<IMG_0>> not found in result")
	}
	if dr.startIndex != 1 {
		t.Fatalf("expected startIndex 1, got %d", dr.startIndex)
	}
	if dr.endIndex != 1+int64(len("<<IMG_0>>")) {
		t.Fatalf("expected endIndex %d, got %d", 1+len("<<IMG_0>>"), dr.endIndex)
	}
}

func TestFindPlaceholderIndices_MultiplePlaceholders(t *testing.T) {
	doc := &docs.Document{
		Body: &docs.Body{
			Content: []*docs.StructuralElement{
				{
					Paragraph: &docs.Paragraph{
						Elements: []*docs.ParagraphElement{
							{
								StartIndex: 1,
								TextRun:    &docs.TextRun{Content: "Hello <<IMG_0>> world"},
							},
						},
					},
				},
				{
					Paragraph: &docs.Paragraph{
						Elements: []*docs.ParagraphElement{
							{
								StartIndex: 50,
								TextRun:    &docs.TextRun{Content: "More text <<IMG_1>> end"},
							},
						},
					},
				},
			},
		},
	}
	result := findPlaceholderIndices(doc, 2)
	if len(result) != 2 {
		t.Fatalf("expected 2 placeholders, got %d", len(result))
	}

	dr0 := result["<<IMG_0>>"]
	// "Hello " is 6 chars, so placeholder starts at startIndex + 6
	if dr0.startIndex != 1+6 {
		t.Fatalf("<<IMG_0>>: expected startIndex %d, got %d", 1+6, dr0.startIndex)
	}
	phLen := int64(len("<<IMG_0>>"))
	if dr0.endIndex != 1+6+phLen {
		t.Fatalf("<<IMG_0>>: expected endIndex %d, got %d", 1+6+phLen, dr0.endIndex)
	}

	dr1 := result["<<IMG_1>>"]
	// "More text " is 10 chars
	if dr1.startIndex != 50+10 {
		t.Fatalf("<<IMG_1>>: expected startIndex %d, got %d", 50+10, dr1.startIndex)
	}
	if dr1.endIndex != 50+10+phLen {
		t.Fatalf("<<IMG_1>>: expected endIndex %d, got %d", 50+10+phLen, dr1.endIndex)
	}
}

func TestFindPlaceholderIndices_PlaceholderWithSurroundingText(t *testing.T) {
	// Placeholder embedded within text: "prefix<<IMG_0>>suffix"
	doc := &docs.Document{
		Body: &docs.Body{
			Content: []*docs.StructuralElement{
				{
					Paragraph: &docs.Paragraph{
						Elements: []*docs.ParagraphElement{
							{
								StartIndex: 10,
								TextRun:    &docs.TextRun{Content: "prefix<<IMG_0>>suffix"},
							},
						},
					},
				},
			},
		},
	}
	result := findPlaceholderIndices(doc, 1)
	if len(result) != 1 {
		t.Fatalf("expected 1 placeholder, got %d", len(result))
	}
	dr := result["<<IMG_0>>"]
	// "prefix" is 6 chars
	wantStart := int64(10 + 6)
	wantEnd := wantStart + int64(len("<<IMG_0>>"))
	if dr.startIndex != wantStart {
		t.Fatalf("expected startIndex %d, got %d", wantStart, dr.startIndex)
	}
	if dr.endIndex != wantEnd {
		t.Fatalf("expected endIndex %d, got %d", wantEnd, dr.endIndex)
	}
}

func TestFindPlaceholderIndices_SkipsNonParagraphElements(t *testing.T) {
	doc := &docs.Document{
		Body: &docs.Body{
			Content: []*docs.StructuralElement{
				{
					// No paragraph — e.g. a section break
					SectionBreak: &docs.SectionBreak{},
				},
				{
					Paragraph: &docs.Paragraph{
						Elements: []*docs.ParagraphElement{
							{
								StartIndex: 5,
								TextRun:    &docs.TextRun{Content: "<<IMG_0>>"},
							},
						},
					},
				},
			},
		},
	}
	result := findPlaceholderIndices(doc, 1)
	if len(result) != 1 {
		t.Fatalf("expected 1 placeholder, got %d", len(result))
	}
}

func TestFindPlaceholderIndices_SkipsNilTextRun(t *testing.T) {
	doc := &docs.Document{
		Body: &docs.Body{
			Content: []*docs.StructuralElement{
				{
					Paragraph: &docs.Paragraph{
						Elements: []*docs.ParagraphElement{
							{
								StartIndex: 0,
								// TextRun is nil (e.g. an InlineObjectElement)
							},
							{
								StartIndex: 10,
								TextRun:    &docs.TextRun{Content: "<<IMG_0>>"},
							},
						},
					},
				},
			},
		},
	}
	result := findPlaceholderIndices(doc, 1)
	if len(result) != 1 {
		t.Fatalf("expected 1 placeholder, got %d", len(result))
	}
}

// ---------------------------------------------------------------------------
// buildImageInsertRequests
// ---------------------------------------------------------------------------

func TestBuildImageInsertRequests_EmptyInputs(t *testing.T) {
	// All empty
	reqs := buildImageInsertRequests(nil, nil, nil)
	if len(reqs) != 0 {
		t.Fatalf("expected 0 requests for nil inputs, got %d", len(reqs))
	}

	// Empty maps and slices
	reqs = buildImageInsertRequests(
		make(map[string]docRange),
		[]markdownImage{},
		make(map[int]string),
	)
	if len(reqs) != 0 {
		t.Fatalf("expected 0 requests for empty inputs, got %d", len(reqs))
	}
}

func TestBuildImageInsertRequests_SingleImage(t *testing.T) {
	img := markdownImage{index: 0, alt: "photo", originalRef: "https://example.com/img.png"}
	placeholders := map[string]docRange{
		"<<IMG_0>>": {startIndex: 10, endIndex: 19},
	}
	imageURLs := map[int]string{
		0: "https://example.com/img.png",
	}

	reqs := buildImageInsertRequests(placeholders, []markdownImage{img}, imageURLs)
	if len(reqs) != 2 {
		t.Fatalf("expected 2 requests (delete + insert), got %d", len(reqs))
	}

	// First request: delete the placeholder text
	del := reqs[0]
	if del.DeleteContentRange == nil {
		t.Fatalf("expected DeleteContentRange, got nil")
	}
	if del.DeleteContentRange.Range.StartIndex != 10 {
		t.Fatalf("delete startIndex = %d, want 10", del.DeleteContentRange.Range.StartIndex)
	}
	if del.DeleteContentRange.Range.EndIndex != 19 {
		t.Fatalf("delete endIndex = %d, want 19", del.DeleteContentRange.Range.EndIndex)
	}

	// Second request: insert inline image
	ins := reqs[1]
	if ins.InsertInlineImage == nil {
		t.Fatalf("expected InsertInlineImage, got nil")
	}
	if ins.InsertInlineImage.Uri != "https://example.com/img.png" {
		t.Fatalf("insert URI = %q, want %q", ins.InsertInlineImage.Uri, "https://example.com/img.png")
	}
	if ins.InsertInlineImage.Location.Index != 10 {
		t.Fatalf("insert location index = %d, want 10", ins.InsertInlineImage.Location.Index)
	}
}

func TestBuildImageInsertRequests_MultipleImages_ReverseOrder(t *testing.T) {
	images := []markdownImage{
		{index: 0, alt: "first", originalRef: "a.png"},
		{index: 1, alt: "second", originalRef: "b.png"},
		{index: 2, alt: "third", originalRef: "c.png"},
	}
	placeholders := map[string]docRange{
		"<<IMG_0>>": {startIndex: 10, endIndex: 19},
		"<<IMG_1>>": {startIndex: 50, endIndex: 59},
		"<<IMG_2>>": {startIndex: 100, endIndex: 109},
	}
	imageURLs := map[int]string{
		0: "https://example.com/a.png",
		1: "https://example.com/b.png",
		2: "https://example.com/c.png",
	}

	reqs := buildImageInsertRequests(placeholders, images, imageURLs)
	// 3 images * 2 requests each = 6
	if len(reqs) != 6 {
		t.Fatalf("expected 6 requests, got %d", len(reqs))
	}

	// Verify reverse ordering: highest start index first
	// Request pair 0,1 should be for IMG_2 (startIndex 100)
	// Request pair 2,3 should be for IMG_1 (startIndex 50)
	// Request pair 4,5 should be for IMG_0 (startIndex 10)
	expectedStarts := []int64{100, 50, 10}
	for i, wantStart := range expectedStarts {
		delReq := reqs[i*2]
		if delReq.DeleteContentRange == nil {
			t.Fatalf("request %d: expected DeleteContentRange", i*2)
		}
		if delReq.DeleteContentRange.Range.StartIndex != wantStart {
			t.Fatalf("request pair %d: delete startIndex = %d, want %d", i, delReq.DeleteContentRange.Range.StartIndex, wantStart)
		}

		insReq := reqs[i*2+1]
		if insReq.InsertInlineImage == nil {
			t.Fatalf("request %d: expected InsertInlineImage", i*2+1)
		}
		if insReq.InsertInlineImage.Location.Index != wantStart {
			t.Fatalf("request pair %d: insert location = %d, want %d", i, insReq.InsertInlineImage.Location.Index, wantStart)
		}
	}
}

func TestBuildImageInsertRequests_MissingPlaceholder(t *testing.T) {
	// Image exists but its placeholder was not found in the document
	img := markdownImage{index: 0, alt: "photo", originalRef: "https://example.com/img.png"}
	placeholders := map[string]docRange{} // empty — placeholder not found
	imageURLs := map[int]string{
		0: "https://example.com/img.png",
	}

	reqs := buildImageInsertRequests(placeholders, []markdownImage{img}, imageURLs)
	if len(reqs) != 0 {
		t.Fatalf("expected 0 requests when placeholder missing, got %d", len(reqs))
	}
}

func TestBuildImageInsertRequests_MissingURL(t *testing.T) {
	// Placeholder found but image URL was not resolved
	img := markdownImage{index: 0, alt: "photo", originalRef: "local.png"}
	placeholders := map[string]docRange{
		"<<IMG_0>>": {startIndex: 10, endIndex: 19},
	}
	imageURLs := map[int]string{} // empty — URL not resolved

	reqs := buildImageInsertRequests(placeholders, []markdownImage{img}, imageURLs)
	if len(reqs) != 0 {
		t.Fatalf("expected 0 requests when URL missing, got %d", len(reqs))
	}
}

func TestBuildImageInsertRequests_PartialMissing(t *testing.T) {
	// Two images: one has both placeholder and URL, other is missing URL
	images := []markdownImage{
		{index: 0, alt: "good", originalRef: "https://example.com/ok.png"},
		{index: 1, alt: "missing", originalRef: "missing.png"},
	}
	placeholders := map[string]docRange{
		"<<IMG_0>>": {startIndex: 10, endIndex: 19},
		"<<IMG_1>>": {startIndex: 50, endIndex: 59},
	}
	imageURLs := map[int]string{
		0: "https://example.com/ok.png",
		// 1 is intentionally missing
	}

	reqs := buildImageInsertRequests(placeholders, images, imageURLs)
	// Only 1 image produces requests (2 = delete + insert)
	if len(reqs) != 2 {
		t.Fatalf("expected 2 requests, got %d", len(reqs))
	}
	if reqs[0].DeleteContentRange.Range.StartIndex != 10 {
		t.Fatalf("expected delete at index 10, got %d", reqs[0].DeleteContentRange.Range.StartIndex)
	}
}

func TestBuildImageInsertRequests_DeleteRangeMatchesPlaceholder(t *testing.T) {
	img := markdownImage{index: 0, originalRef: "https://x.com/a.png"}
	phStart := int64(25)
	phEnd := int64(34)
	placeholders := map[string]docRange{
		"<<IMG_0>>": {startIndex: phStart, endIndex: phEnd},
	}
	imageURLs := map[int]string{0: "https://x.com/a.png"}

	reqs := buildImageInsertRequests(placeholders, []markdownImage{img}, imageURLs)
	if len(reqs) != 2 {
		t.Fatalf("expected 2 requests, got %d", len(reqs))
	}
	delRange := reqs[0].DeleteContentRange.Range
	if delRange.StartIndex != phStart || delRange.EndIndex != phEnd {
		t.Fatalf("delete range = [%d, %d), want [%d, %d)", delRange.StartIndex, delRange.EndIndex, phStart, phEnd)
	}
}

func TestBuildImageInsertRequests_InsertLocationMatchesStart(t *testing.T) {
	img := markdownImage{index: 0, originalRef: "https://x.com/a.png"}
	phStart := int64(42)
	placeholders := map[string]docRange{
		"<<IMG_0>>": {startIndex: phStart, endIndex: phStart + 9},
	}
	imageURLs := map[int]string{0: "https://x.com/a.png"}

	reqs := buildImageInsertRequests(placeholders, []markdownImage{img}, imageURLs)
	if len(reqs) != 2 {
		t.Fatalf("expected 2 requests, got %d", len(reqs))
	}
	insLoc := reqs[1].InsertInlineImage.Location.Index
	if insLoc != phStart {
		t.Fatalf("insert location = %d, want %d", insLoc, phStart)
	}
}

// ---------------------------------------------------------------------------
// Round-trip: extract then find placeholders
// ---------------------------------------------------------------------------

func TestExtractAndFindPlaceholders_RoundTrip(t *testing.T) {
	content := "Before ![a](a.png) middle ![b](b.jpg) after"
	cleaned, images := extractMarkdownImages(content)

	// Build a fake Google Doc from the cleaned content
	doc := &docs.Document{
		Body: &docs.Body{
			Content: []*docs.StructuralElement{
				{
					Paragraph: &docs.Paragraph{
						Elements: []*docs.ParagraphElement{
							{
								StartIndex: 1, // Google Docs body starts at index 1
								TextRun:    &docs.TextRun{Content: cleaned},
							},
						},
					},
				},
			},
		},
	}

	placeholders := findPlaceholderIndices(doc, len(images))
	if len(placeholders) != 2 {
		t.Fatalf("expected 2 placeholders, got %d", len(placeholders))
	}

	// Both images should have resolved placeholders
	for _, img := range images {
		ph := img.placeholder()
		dr, ok := placeholders[ph]
		if !ok {
			t.Fatalf("placeholder %q not found", ph)
		}
		// Verify the range length matches the placeholder text length
		phLen := int64(len(ph))
		if dr.endIndex-dr.startIndex != phLen {
			t.Fatalf("placeholder %q: range length = %d, want %d", ph, dr.endIndex-dr.startIndex, phLen)
		}
	}

	// Now build requests and verify they are in reverse order
	imageURLs := map[int]string{
		0: "https://example.com/a.png",
		1: "https://example.com/b.jpg",
	}
	reqs := buildImageInsertRequests(placeholders, images, imageURLs)
	if len(reqs) != 4 {
		t.Fatalf("expected 4 requests, got %d", len(reqs))
	}

	// First delete should be for the later placeholder (higher index)
	firstDelStart := reqs[0].DeleteContentRange.Range.StartIndex
	secondDelStart := reqs[2].DeleteContentRange.Range.StartIndex
	if firstDelStart <= secondDelStart {
		t.Fatalf("expected reverse order: first delete start (%d) should be > second (%d)", firstDelStart, secondDelStart)
	}
}
