package cmd

import (
	"testing"

	"google.golang.org/api/docs/v1"
)

// testImages creates []markdownImage with the fixed "test" token for use in tests.
func testImages(count int) []markdownImage {
	imgs := make([]markdownImage, count)
	for i := range imgs {
		imgs[i] = markdownImage{index: i, token: "test"}
	}
	return imgs
}

// ---------------------------------------------------------------------------
// extractMarkdownImages
// ---------------------------------------------------------------------------

func TestExtractMarkdownImages_NoImages(t *testing.T) {
	origToken := imgPlaceholderToken
	t.Cleanup(func() { imgPlaceholderToken = origToken })
	imgPlaceholderToken = func() string { return "test" }

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
	origToken := imgPlaceholderToken
	t.Cleanup(func() { imgPlaceholderToken = origToken })
	imgPlaceholderToken = func() string { return "test" }

	content := "![alt text](image.png)"
	cleaned, images := extractMarkdownImages(content)
	if cleaned != "<<IMG_test_0>>" {
		t.Fatalf("expected <<IMG_test_0>>, got %q", cleaned)
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
	origToken := imgPlaceholderToken
	t.Cleanup(func() { imgPlaceholderToken = origToken })
	imgPlaceholderToken = func() string { return "test" }

	content := "![a](one.png) text ![b](two.jpg) more ![c](three.gif)"
	cleaned, images := extractMarkdownImages(content)
	if len(images) != 3 {
		t.Fatalf("expected 3 images, got %d", len(images))
	}
	want := "<<IMG_test_0>> text <<IMG_test_1>> more <<IMG_test_2>>"
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
			origToken := imgPlaceholderToken
			t.Cleanup(func() { imgPlaceholderToken = origToken })
			imgPlaceholderToken = func() string { return "test" }

			content := "![photo](" + tc.url + ")"
			cleaned, images := extractMarkdownImages(content)
			if cleaned != "<<IMG_test_0>>" {
				t.Fatalf("expected <<IMG_test_0>>, got %q", cleaned)
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
			origToken := imgPlaceholderToken
			t.Cleanup(func() { imgPlaceholderToken = origToken })
			imgPlaceholderToken = func() string { return "test" }

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
	origToken := imgPlaceholderToken
	t.Cleanup(func() { imgPlaceholderToken = origToken })
	imgPlaceholderToken = func() string { return "test" }

	content := `![alt](image.png "My Title")`
	cleaned, images := extractMarkdownImages(content)
	if cleaned != "<<IMG_test_0>>" {
		t.Fatalf("expected <<IMG_test_0>>, got %q", cleaned)
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
	origToken := imgPlaceholderToken
	t.Cleanup(func() { imgPlaceholderToken = origToken })
	imgPlaceholderToken = func() string { return "test" }

	content := "# Heading\n\nSome text before.\n\n![first](a.png)\n\nMiddle paragraph.\n\n![second](b.jpg)\n\nText after images."
	cleaned, images := extractMarkdownImages(content)
	if len(images) != 2 {
		t.Fatalf("expected 2 images, got %d", len(images))
	}
	want := "# Heading\n\nSome text before.\n\n<<IMG_test_0>>\n\nMiddle paragraph.\n\n<<IMG_test_1>>\n\nText after images."
	if cleaned != want {
		t.Fatalf("expected:\n%s\ngot:\n%s", want, cleaned)
	}
}

func TestExtractMarkdownImages_EmptyAltText(t *testing.T) {
	origToken := imgPlaceholderToken
	t.Cleanup(func() { imgPlaceholderToken = origToken })
	imgPlaceholderToken = func() string { return "test" }

	content := "![](image.png)"
	cleaned, images := extractMarkdownImages(content)
	if cleaned != "<<IMG_test_0>>" {
		t.Fatalf("expected <<IMG_test_0>>, got %q", cleaned)
	}
	if len(images) != 1 {
		t.Fatalf("expected 1 image, got %d", len(images))
	}
	if images[0].alt != "" {
		t.Fatalf("expected empty alt, got %q", images[0].alt)
	}
}

func TestExtractMarkdownImages_SpecialCharsInURL(t *testing.T) {
	origToken := imgPlaceholderToken
	t.Cleanup(func() { imgPlaceholderToken = origToken })
	imgPlaceholderToken = func() string { return "test" }

	content := "![pic](https://example.com/path/to/image%20name.png?v=2&size=large)"
	cleaned, images := extractMarkdownImages(content)
	if cleaned != "<<IMG_test_0>>" {
		t.Fatalf("expected <<IMG_test_0>>, got %q", cleaned)
	}
	if len(images) != 1 {
		t.Fatalf("expected 1 image, got %d", len(images))
	}
	if images[0].originalRef != "https://example.com/path/to/image%20name.png?v=2&size=large" {
		t.Fatalf("unexpected ref %q", images[0].originalRef)
	}
}

func TestExtractMarkdownImages_PlaceholderFormat(t *testing.T) {
	origToken := imgPlaceholderToken
	t.Cleanup(func() { imgPlaceholderToken = origToken })
	imgPlaceholderToken = func() string { return "test" }

	content := "![a](one.png) ![b](two.png) ![c](three.png)"
	cleaned, images := extractMarkdownImages(content)
	_ = images
	if cleaned != "<<IMG_test_0>> <<IMG_test_1>> <<IMG_test_2>>" {
		t.Fatalf("unexpected placeholder format: %q", cleaned)
	}
}

func TestExtractMarkdownImages_EmptyContent(t *testing.T) {
	origToken := imgPlaceholderToken
	t.Cleanup(func() { imgPlaceholderToken = origToken })
	imgPlaceholderToken = func() string { return "test" }

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
		{0, "<<IMG_test_0>>"},
		{1, "<<IMG_test_1>>"},
		{5, "<<IMG_test_5>>"},
		{42, "<<IMG_test_42>>"},
	}
	for _, tc := range tests {
		img := markdownImage{index: tc.index, token: "test"}
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
	result := findPlaceholderIndices(nil, testImages(1))
	if len(result) != 0 {
		t.Fatalf("expected empty map for nil doc, got %d entries", len(result))
	}
}

func TestFindPlaceholderIndices_NilBody(t *testing.T) {
	doc := &docs.Document{}
	result := findPlaceholderIndices(doc, testImages(1))
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
	result := findPlaceholderIndices(doc, testImages(1))
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
								TextRun:    &docs.TextRun{Content: "<<IMG_test_0>>"},
							},
						},
					},
				},
			},
		},
	}
	result := findPlaceholderIndices(doc, testImages(0))
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
	result := findPlaceholderIndices(doc, testImages(2))
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
								TextRun:    &docs.TextRun{Content: "<<IMG_test_0>>"},
							},
						},
					},
				},
			},
		},
	}
	result := findPlaceholderIndices(doc, testImages(1))
	if len(result) != 1 {
		t.Fatalf("expected 1 placeholder, got %d", len(result))
	}
	dr, ok := result["<<IMG_test_0>>"]
	if !ok {
		t.Fatalf("<<IMG_test_0>> not found in result")
	}
	if dr.startIndex != 1 {
		t.Fatalf("expected startIndex 1, got %d", dr.startIndex)
	}
	if dr.endIndex != 1+int64(len("<<IMG_test_0>>")) {
		t.Fatalf("expected endIndex %d, got %d", 1+len("<<IMG_test_0>>"), dr.endIndex)
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
								TextRun:    &docs.TextRun{Content: "Hello <<IMG_test_0>> world"},
							},
						},
					},
				},
				{
					Paragraph: &docs.Paragraph{
						Elements: []*docs.ParagraphElement{
							{
								StartIndex: 50,
								TextRun:    &docs.TextRun{Content: "More text <<IMG_test_1>> end"},
							},
						},
					},
				},
			},
		},
	}
	result := findPlaceholderIndices(doc, testImages(2))
	if len(result) != 2 {
		t.Fatalf("expected 2 placeholders, got %d", len(result))
	}

	dr0 := result["<<IMG_test_0>>"]
	// "Hello " is 6 chars, so placeholder starts at startIndex + 6
	if dr0.startIndex != 1+6 {
		t.Fatalf("<<IMG_test_0>>: expected startIndex %d, got %d", 1+6, dr0.startIndex)
	}
	phLen := int64(len("<<IMG_test_0>>"))
	if dr0.endIndex != 1+6+phLen {
		t.Fatalf("<<IMG_test_0>>: expected endIndex %d, got %d", 1+6+phLen, dr0.endIndex)
	}

	dr1 := result["<<IMG_test_1>>"]
	// "More text " is 10 chars
	if dr1.startIndex != 50+10 {
		t.Fatalf("<<IMG_test_1>>: expected startIndex %d, got %d", 50+10, dr1.startIndex)
	}
	if dr1.endIndex != 50+10+phLen {
		t.Fatalf("<<IMG_test_1>>: expected endIndex %d, got %d", 50+10+phLen, dr1.endIndex)
	}
}

func TestFindPlaceholderIndices_PlaceholderWithSurroundingText(t *testing.T) {
	// Placeholder embedded within text: "prefix<<IMG_test_0>>suffix"
	doc := &docs.Document{
		Body: &docs.Body{
			Content: []*docs.StructuralElement{
				{
					Paragraph: &docs.Paragraph{
						Elements: []*docs.ParagraphElement{
							{
								StartIndex: 10,
								TextRun:    &docs.TextRun{Content: "prefix<<IMG_test_0>>suffix"},
							},
						},
					},
				},
			},
		},
	}
	result := findPlaceholderIndices(doc, testImages(1))
	if len(result) != 1 {
		t.Fatalf("expected 1 placeholder, got %d", len(result))
	}
	dr := result["<<IMG_test_0>>"]
	// "prefix" is 6 chars
	wantStart := int64(10 + 6)
	wantEnd := wantStart + int64(len("<<IMG_test_0>>"))
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
								TextRun:    &docs.TextRun{Content: "<<IMG_test_0>>"},
							},
						},
					},
				},
			},
		},
	}
	result := findPlaceholderIndices(doc, testImages(1))
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
								TextRun:    &docs.TextRun{Content: "<<IMG_test_0>>"},
							},
						},
					},
				},
			},
		},
	}
	result := findPlaceholderIndices(doc, testImages(1))
	if len(result) != 1 {
		t.Fatalf("expected 1 placeholder, got %d", len(result))
	}
}

// ---------------------------------------------------------------------------
// buildImageInsertRequests
// ---------------------------------------------------------------------------

func TestBuildImageInsertRequests_EmptyInputs(t *testing.T) {
	// All empty
	reqs := buildImageInsertRequests(nil, nil, nil, "")
	if len(reqs) != 0 {
		t.Fatalf("expected 0 requests for nil inputs, got %d", len(reqs))
	}

	// Empty maps and slices
	reqs = buildImageInsertRequests(
		make(map[string]docRange),
		[]markdownImage{},
		make(map[int]string),
		"",
	)
	if len(reqs) != 0 {
		t.Fatalf("expected 0 requests for empty inputs, got %d", len(reqs))
	}
}

func TestBuildImageInsertRequests_SingleImage(t *testing.T) {
	img := markdownImage{index: 0, alt: "photo", originalRef: "https://example.com/img.png", token: "test"}
	placeholders := map[string]docRange{
		"<<IMG_test_0>>": {startIndex: 10, endIndex: 19},
	}
	imageURLs := map[int]string{
		0: "https://example.com/img.png",
	}

	reqs := buildImageInsertRequests(placeholders, []markdownImage{img}, imageURLs, "")
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
		{index: 0, alt: "first", originalRef: "a.png", token: "test"},
		{index: 1, alt: "second", originalRef: "b.png", token: "test"},
		{index: 2, alt: "third", originalRef: "c.png", token: "test"},
	}
	placeholders := map[string]docRange{
		"<<IMG_test_0>>": {startIndex: 10, endIndex: 19},
		"<<IMG_test_1>>": {startIndex: 50, endIndex: 59},
		"<<IMG_test_2>>": {startIndex: 100, endIndex: 109},
	}
	imageURLs := map[int]string{
		0: "https://example.com/a.png",
		1: "https://example.com/b.png",
		2: "https://example.com/c.png",
	}

	reqs := buildImageInsertRequests(placeholders, images, imageURLs, "")
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
	img := markdownImage{index: 0, alt: "photo", originalRef: "https://example.com/img.png", token: "test"}
	placeholders := map[string]docRange{} // empty — placeholder not found
	imageURLs := map[int]string{
		0: "https://example.com/img.png",
	}

	reqs := buildImageInsertRequests(placeholders, []markdownImage{img}, imageURLs, "")
	if len(reqs) != 0 {
		t.Fatalf("expected 0 requests when placeholder missing, got %d", len(reqs))
	}
}

func TestBuildImageInsertRequests_MissingURL(t *testing.T) {
	// Placeholder found but image URL was not resolved
	img := markdownImage{index: 0, alt: "photo", originalRef: "local.png", token: "test"}
	placeholders := map[string]docRange{
		"<<IMG_test_0>>": {startIndex: 10, endIndex: 19},
	}
	imageURLs := map[int]string{} // empty — URL not resolved

	reqs := buildImageInsertRequests(placeholders, []markdownImage{img}, imageURLs, "")
	if len(reqs) != 0 {
		t.Fatalf("expected 0 requests when URL missing, got %d", len(reqs))
	}
}

func TestBuildImageInsertRequests_PartialMissing(t *testing.T) {
	// Two images: one has both placeholder and URL, other is missing URL
	images := []markdownImage{
		{index: 0, alt: "good", originalRef: "https://example.com/ok.png", token: "test"},
		{index: 1, alt: "missing", originalRef: "missing.png", token: "test"},
	}
	placeholders := map[string]docRange{
		"<<IMG_test_0>>": {startIndex: 10, endIndex: 19},
		"<<IMG_test_1>>": {startIndex: 50, endIndex: 59},
	}
	imageURLs := map[int]string{
		0: "https://example.com/ok.png",
		// 1 is intentionally missing
	}

	reqs := buildImageInsertRequests(placeholders, images, imageURLs, "")
	// Only 1 image produces requests (2 = delete + insert)
	if len(reqs) != 2 {
		t.Fatalf("expected 2 requests, got %d", len(reqs))
	}
	if reqs[0].DeleteContentRange.Range.StartIndex != 10 {
		t.Fatalf("expected delete at index 10, got %d", reqs[0].DeleteContentRange.Range.StartIndex)
	}
}

func TestBuildImageInsertRequests_DeleteRangeMatchesPlaceholder(t *testing.T) {
	img := markdownImage{index: 0, originalRef: "https://x.com/a.png", token: "test"}
	phStart := int64(25)
	phEnd := int64(34)
	placeholders := map[string]docRange{
		"<<IMG_test_0>>": {startIndex: phStart, endIndex: phEnd},
	}
	imageURLs := map[int]string{0: "https://x.com/a.png"}

	reqs := buildImageInsertRequests(placeholders, []markdownImage{img}, imageURLs, "")
	if len(reqs) != 2 {
		t.Fatalf("expected 2 requests, got %d", len(reqs))
	}
	delRange := reqs[0].DeleteContentRange.Range
	if delRange.StartIndex != phStart || delRange.EndIndex != phEnd {
		t.Fatalf("delete range = [%d, %d), want [%d, %d)", delRange.StartIndex, delRange.EndIndex, phStart, phEnd)
	}
}

func TestBuildImageInsertRequests_InsertLocationMatchesStart(t *testing.T) {
	img := markdownImage{index: 0, originalRef: "https://x.com/a.png", token: "test"}
	phStart := int64(42)
	placeholders := map[string]docRange{
		"<<IMG_test_0>>": {startIndex: phStart, endIndex: phStart + 9},
	}
	imageURLs := map[int]string{0: "https://x.com/a.png"}

	reqs := buildImageInsertRequests(placeholders, []markdownImage{img}, imageURLs, "")
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
	origToken := imgPlaceholderToken
	t.Cleanup(func() { imgPlaceholderToken = origToken })
	imgPlaceholderToken = func() string { return "test" }

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

	placeholders := findPlaceholderIndices(doc, images)
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
	reqs := buildImageInsertRequests(placeholders, images, imageURLs, "")
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

// ---------------------------------------------------------------------------
// findPlaceholderIndices: UTF-16 correctness
// ---------------------------------------------------------------------------

func TestFindPlaceholderIndices_NonASCIIPrefix(t *testing.T) {
	// The bullet character "•" is 3 bytes in UTF-8 but 1 UTF-16 code unit.
	// This test verifies that findPlaceholderIndices uses UTF-16 offsets
	// (matching the Google Docs API) rather than byte offsets.
	//
	// Text: "• <<IMG_test_0>> after"
	//   UTF-16: • (1) + space (1) + <<IMG_test_0>> (14) + ...
	//   Bytes:  • (3) + space (1) + <<IMG_test_0>> (14) + ...
	//
	// With startIndex=10, the placeholder should be at UTF-16 position 10+2=12,
	// NOT at byte position 10+4=14.
	doc := &docs.Document{
		Body: &docs.Body{
			Content: []*docs.StructuralElement{
				{
					Paragraph: &docs.Paragraph{
						Elements: []*docs.ParagraphElement{
							{
								StartIndex: 10,
								TextRun:    &docs.TextRun{Content: "• <<IMG_test_0>> after"},
							},
						},
					},
				},
			},
		},
	}
	result := findPlaceholderIndices(doc, testImages(1))
	if len(result) != 1 {
		t.Fatalf("expected 1 placeholder, got %d", len(result))
	}
	dr := result["<<IMG_test_0>>"]
	// "• " is 2 UTF-16 code units (not 4 bytes), so placeholder starts at 10+2=12.
	wantStart := int64(10 + 2)
	if dr.startIndex != wantStart {
		t.Fatalf("startIndex = %d, want %d (UTF-16 offset, not byte offset)", dr.startIndex, wantStart)
	}
	wantEnd := wantStart + utf16Len("<<IMG_test_0>>")
	if dr.endIndex != wantEnd {
		t.Fatalf("endIndex = %d, want %d", dr.endIndex, wantEnd)
	}
}

func TestFindPlaceholderIndices_EmojiPrefix(t *testing.T) {
	// Emoji "🎉" is 4 bytes in UTF-8 and 2 UTF-16 code units (surrogate pair).
	// Text: "🎉 <<IMG_test_0>>"
	//   UTF-16: 🎉 (2) + space (1) = 3 units before placeholder
	//   Bytes:  🎉 (4) + space (1) = 5 bytes before placeholder
	doc := &docs.Document{
		Body: &docs.Body{
			Content: []*docs.StructuralElement{
				{
					Paragraph: &docs.Paragraph{
						Elements: []*docs.ParagraphElement{
							{
								StartIndex: 1,
								TextRun:    &docs.TextRun{Content: "🎉 <<IMG_test_0>>"},
							},
						},
					},
				},
			},
		},
	}
	result := findPlaceholderIndices(doc, testImages(1))
	if len(result) != 1 {
		t.Fatalf("expected 1 placeholder, got %d", len(result))
	}
	dr := result["<<IMG_test_0>>"]
	// "🎉 " = 2+1 = 3 UTF-16 code units, so placeholder starts at 1+3=4.
	wantStart := int64(1 + 3)
	if dr.startIndex != wantStart {
		t.Fatalf("startIndex = %d, want %d (emoji is 2 UTF-16 units, not 4 bytes)", dr.startIndex, wantStart)
	}
}

func TestFindPlaceholderIndices_InsideTableCell(t *testing.T) {
	// Drive's markdown converter puts table cell content inside Table > TableRow
	// > TableCell > Content > Paragraph structures. Placeholders in table cells
	// must be found by recursing into table elements.
	doc := &docs.Document{
		Body: &docs.Body{
			Content: []*docs.StructuralElement{
				{
					Table: &docs.Table{
						TableRows: []*docs.TableRow{
							{
								TableCells: []*docs.TableCell{
									{
										Content: []*docs.StructuralElement{
											{
												Paragraph: &docs.Paragraph{
													Elements: []*docs.ParagraphElement{
														{
															StartIndex: 50,
															TextRun:    &docs.TextRun{Content: "<<IMG_test_0>>\n"},
														},
													},
												},
											},
										},
									},
									{
										Content: []*docs.StructuralElement{
											{
												Paragraph: &docs.Paragraph{
													Elements: []*docs.ParagraphElement{
														{
															StartIndex: 100,
															TextRun:    &docs.TextRun{Content: "<<IMG_test_1>>\n"},
														},
													},
												},
											},
										},
									},
								},
							},
						},
					},
				},
			},
		},
	}
	result := findPlaceholderIndices(doc, testImages(2))
	if len(result) != 2 {
		t.Fatalf("expected 2 placeholders inside table cells, got %d", len(result))
	}
	dr0 := result["<<IMG_test_0>>"]
	if dr0.startIndex != 50 {
		t.Fatalf("IMG_0 startIndex = %d, want 50", dr0.startIndex)
	}
	dr1 := result["<<IMG_test_1>>"]
	if dr1.startIndex != 100 {
		t.Fatalf("IMG_1 startIndex = %d, want 100", dr1.startIndex)
	}
}

func TestFindPlaceholderIndices_SplitAcrossTextRuns(t *testing.T) {
	// Simulates the real-world scenario where Drive's markdown converter splits
	// a placeholder across two text runs within the same paragraph.
	// <<IMG_test_0>> is split as "text <<IMG_" in run1 and "test_0>> more" in run2.
	doc := &docs.Document{
		Body: &docs.Body{
			Content: []*docs.StructuralElement{
				{
					Paragraph: &docs.Paragraph{
						Elements: []*docs.ParagraphElement{
							{
								StartIndex: 1,
								TextRun:    &docs.TextRun{Content: "text <<IMG_"},
							},
							{
								StartIndex: 12,
								TextRun:    &docs.TextRun{Content: "test_0>> more"},
							},
						},
					},
				},
			},
		},
	}
	result := findPlaceholderIndices(doc, testImages(1))
	if len(result) != 1 {
		t.Fatalf("expected 1 placeholder across split runs, got %d", len(result))
	}
	dr := result["<<IMG_test_0>>"]
	// "text " is 5 chars, so placeholder starts at 1+5=6
	wantStart := int64(6)
	if dr.startIndex != wantStart {
		t.Fatalf("startIndex = %d, want %d", dr.startIndex, wantStart)
	}
	wantEnd := wantStart + int64(len("<<IMG_test_0>>"))
	if dr.endIndex != wantEnd {
		t.Fatalf("endIndex = %d, want %d", dr.endIndex, wantEnd)
	}
}

func TestFindPlaceholderIndices_SplitAtAngleBrackets(t *testing.T) {
	// Split right at the << boundary — most likely point Drive would split.
	doc := &docs.Document{
		Body: &docs.Body{
			Content: []*docs.StructuralElement{
				{
					Paragraph: &docs.Paragraph{
						Elements: []*docs.ParagraphElement{
							{
								StartIndex: 1,
								TextRun:    &docs.TextRun{Content: "before <<"},
							},
							{
								StartIndex: 10,
								TextRun:    &docs.TextRun{Content: "IMG_test_0>> after"},
							},
						},
					},
				},
			},
		},
	}
	result := findPlaceholderIndices(doc, testImages(1))
	if len(result) != 1 {
		t.Fatalf("expected 1 placeholder across angle-bracket split, got %d", len(result))
	}
	dr := result["<<IMG_test_0>>"]
	// "before " is 7 chars
	wantStart := int64(8)
	if dr.startIndex != wantStart {
		t.Fatalf("startIndex = %d, want %d", dr.startIndex, wantStart)
	}
}

// ---------------------------------------------------------------------------
// extractMarkdownImages: dimension parsing
// ---------------------------------------------------------------------------

func TestExtractMarkdownImages_WithDimensions(t *testing.T) {
	origToken := imgPlaceholderToken
	t.Cleanup(func() { imgPlaceholderToken = origToken })
	imgPlaceholderToken = func() string { return "test" }

	content := "![alt](url){width=200}"
	cleaned, images := extractMarkdownImages(content)
	if cleaned != "<<IMG_test_0>>" {
		t.Fatalf("expected <<IMG_test_0>>, got %q", cleaned)
	}
	if len(images) != 1 {
		t.Fatalf("expected 1 image, got %d", len(images))
	}
	if images[0].widthPt != 200 {
		t.Fatalf("expected widthPt=200, got %v", images[0].widthPt)
	}
	if images[0].heightPt != 0 {
		t.Fatalf("expected heightPt=0, got %v", images[0].heightPt)
	}
}

func TestExtractMarkdownImages_WithBothDimensions(t *testing.T) {
	origToken := imgPlaceholderToken
	t.Cleanup(func() { imgPlaceholderToken = origToken })
	imgPlaceholderToken = func() string { return "test" }

	content := "![alt](url){width=200 height=150}"
	cleaned, images := extractMarkdownImages(content)
	if cleaned != "<<IMG_test_0>>" {
		t.Fatalf("expected <<IMG_test_0>>, got %q", cleaned)
	}
	if len(images) != 1 {
		t.Fatalf("expected 1 image, got %d", len(images))
	}
	if images[0].widthPt != 200 {
		t.Fatalf("expected widthPt=200, got %v", images[0].widthPt)
	}
	if images[0].heightPt != 150 {
		t.Fatalf("expected heightPt=150, got %v", images[0].heightPt)
	}
}

func TestExtractMarkdownImages_WithShorthandDimensions(t *testing.T) {
	origToken := imgPlaceholderToken
	t.Cleanup(func() { imgPlaceholderToken = origToken })
	imgPlaceholderToken = func() string { return "test" }

	content := "![alt](url){w=200 h=150}"
	cleaned, images := extractMarkdownImages(content)
	if cleaned != "<<IMG_test_0>>" {
		t.Fatalf("expected <<IMG_test_0>>, got %q", cleaned)
	}
	if len(images) != 1 {
		t.Fatalf("expected 1 image, got %d", len(images))
	}
	if images[0].widthPt != 200 {
		t.Fatalf("expected widthPt=200, got %v", images[0].widthPt)
	}
	if images[0].heightPt != 150 {
		t.Fatalf("expected heightPt=150, got %v", images[0].heightPt)
	}
}

func TestExtractMarkdownImages_NoDimensionsFallback(t *testing.T) {
	origToken := imgPlaceholderToken
	t.Cleanup(func() { imgPlaceholderToken = origToken })
	imgPlaceholderToken = func() string { return "test" }

	content := "![alt](url)"
	_, images := extractMarkdownImages(content)
	if len(images) != 1 {
		t.Fatalf("expected 1 image, got %d", len(images))
	}
	if images[0].widthPt != 0 {
		t.Fatalf("expected widthPt=0, got %v", images[0].widthPt)
	}
	if images[0].heightPt != 0 {
		t.Fatalf("expected heightPt=0, got %v", images[0].heightPt)
	}
}

// ---------------------------------------------------------------------------
// buildImageInsertRequests: dimension handling
// ---------------------------------------------------------------------------

func TestBuildImageInsertRequests_CustomWidth(t *testing.T) {
	img := markdownImage{index: 0, originalRef: "https://x.com/a.png", token: "test", widthPt: 200}
	placeholders := map[string]docRange{
		"<<IMG_test_0>>": {startIndex: 10, endIndex: 24},
	}
	imageURLs := map[int]string{0: "https://x.com/a.png"}

	reqs := buildImageInsertRequests(placeholders, []markdownImage{img}, imageURLs, "")
	if len(reqs) != 2 {
		t.Fatalf("expected 2 requests, got %d", len(reqs))
	}
	ins := reqs[1].InsertInlineImage
	if ins.ObjectSize.Width == nil || ins.ObjectSize.Width.Magnitude != 200 {
		t.Fatalf("expected width=200pt, got %+v", ins.ObjectSize.Width)
	}
	if ins.ObjectSize.Height != nil {
		t.Fatalf("expected nil height for width-only, got %+v", ins.ObjectSize.Height)
	}
}

func TestBuildImageInsertRequests_CustomBothDimensions(t *testing.T) {
	img := markdownImage{index: 0, originalRef: "https://x.com/a.png", token: "test", widthPt: 200, heightPt: 150}
	placeholders := map[string]docRange{
		"<<IMG_test_0>>": {startIndex: 10, endIndex: 24},
	}
	imageURLs := map[int]string{0: "https://x.com/a.png"}

	reqs := buildImageInsertRequests(placeholders, []markdownImage{img}, imageURLs, "")
	if len(reqs) != 2 {
		t.Fatalf("expected 2 requests, got %d", len(reqs))
	}
	ins := reqs[1].InsertInlineImage
	if ins.ObjectSize.Width == nil || ins.ObjectSize.Width.Magnitude != 200 {
		t.Fatalf("expected width=200pt, got %+v", ins.ObjectSize.Width)
	}
	if ins.ObjectSize.Height == nil || ins.ObjectSize.Height.Magnitude != 150 {
		t.Fatalf("expected height=150pt, got %+v", ins.ObjectSize.Height)
	}
}

func TestBuildImageInsertRequests_CustomHeightOnly(t *testing.T) {
	img := markdownImage{index: 0, originalRef: "https://x.com/a.png", token: "test", heightPt: 150}
	placeholders := map[string]docRange{
		"<<IMG_test_0>>": {startIndex: 10, endIndex: 24},
	}
	imageURLs := map[int]string{0: "https://x.com/a.png"}

	reqs := buildImageInsertRequests(placeholders, []markdownImage{img}, imageURLs, "")
	if len(reqs) != 2 {
		t.Fatalf("expected 2 requests, got %d", len(reqs))
	}
	ins := reqs[1].InsertInlineImage
	if ins.ObjectSize.Width != nil {
		t.Fatalf("expected nil width for height-only, got %+v", ins.ObjectSize.Width)
	}
	if ins.ObjectSize.Height == nil || ins.ObjectSize.Height.Magnitude != 150 {
		t.Fatalf("expected height=150pt, got %+v", ins.ObjectSize.Height)
	}
}

func TestBuildImageInsertRequests_DefaultWidth(t *testing.T) {
	img := markdownImage{index: 0, originalRef: "https://x.com/a.png", token: "test"}
	placeholders := map[string]docRange{
		"<<IMG_test_0>>": {startIndex: 10, endIndex: 24},
	}
	imageURLs := map[int]string{0: "https://x.com/a.png"}

	reqs := buildImageInsertRequests(placeholders, []markdownImage{img}, imageURLs, "")
	if len(reqs) != 2 {
		t.Fatalf("expected 2 requests, got %d", len(reqs))
	}
	ins := reqs[1].InsertInlineImage
	if ins.ObjectSize.Width == nil || ins.ObjectSize.Width.Magnitude != defaultImageMaxWidthPt {
		t.Fatalf("expected default width=%vpt, got %+v", defaultImageMaxWidthPt, ins.ObjectSize.Width)
	}
	if ins.ObjectSize.Height != nil {
		t.Fatalf("expected nil height for default, got %+v", ins.ObjectSize.Height)
	}
}

func TestFindPlaceholderIndices_MultipleBullets(t *testing.T) {
	// Simulates the real-world scenario: markdown formatter inserts "• " prefixed
	// list items containing image placeholders.
	doc := &docs.Document{
		Body: &docs.Body{
			Content: []*docs.StructuralElement{
				{
					Paragraph: &docs.Paragraph{
						Elements: []*docs.ParagraphElement{
							{
								StartIndex: 1,
								TextRun:    &docs.TextRun{Content: "• First <<IMG_test_0>> item\n"},
							},
						},
					},
				},
				{
					Paragraph: &docs.Paragraph{
						Elements: []*docs.ParagraphElement{
							{
								StartIndex: 29,
								TextRun:    &docs.TextRun{Content: "• Second <<IMG_test_1>> item\n"},
							},
						},
					},
				},
			},
		},
	}
	result := findPlaceholderIndices(doc, testImages(2))
	if len(result) != 2 {
		t.Fatalf("expected 2 placeholders, got %d", len(result))
	}

	// "• First " = "• " (2 UTF-16) + "First " (6 UTF-16) = 8 UTF-16 units.
	dr0 := result["<<IMG_test_0>>"]
	wantStart0 := int64(1 + 8)
	if dr0.startIndex != wantStart0 {
		t.Fatalf("IMG_0 startIndex = %d, want %d", dr0.startIndex, wantStart0)
	}

	// "• Second " = "• " (2 UTF-16) + "Second " (7 UTF-16) = 9 UTF-16 units.
	dr1 := result["<<IMG_test_1>>"]
	wantStart1 := int64(29 + 9)
	if dr1.startIndex != wantStart1 {
		t.Fatalf("IMG_1 startIndex = %d, want %d", dr1.startIndex, wantStart1)
	}
}
