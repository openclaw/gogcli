//nolint:wsl_v5 // Table-driven parser fixtures stay compact around assertions.
package docssed

import (
	"reflect"
	"testing"
)

func TestParseImageSyntax(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name  string
		input string
		want  *ImageSpec
	}{
		{name: "basic", input: "![](https://example.com/image.png)", want: &ImageSpec{URL: "https://example.com/image.png"}},
		{
			name:  "full",
			input: `![Logo](https://example.com/logo.png "Figure 1"){width=400 height=300}`,
			want: &ImageSpec{
				URL:     "https://example.com/logo.png",
				Alt:     "Logo",
				Caption: "Figure 1",
				Width:   400,
				Height:  300,
			},
		},
		{
			name:  "short dimensions",
			input: "![](https://example.com/img.png){w=300px h=25%}",
			want:  &ImageSpec{URL: "https://example.com/img.png", Width: 300, Height: 25},
		},
		{
			name:  "query and alt",
			input: "![A cool image!](https://example.com/img.png?size=large&format=webp)",
			want: &ImageSpec{
				URL: "https://example.com/img.png?size=large&format=webp",
				Alt: "A cool image!",
			},
		},
		{name: "plain text", input: "hello world"},
		{name: "missing close", input: "![alt text(url)"},
		{name: "empty"},
	}

	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			if got := ParseImageSyntax(test.input); !reflect.DeepEqual(got, test.want) {
				t.Fatalf("ParseImageSyntax(%q) = %#v, want %#v", test.input, got, test.want)
			}
		})
	}
}

func FuzzParseImageSyntax(f *testing.F) {
	for _, seed := range []string{
		"![alt](https://example.com/image.png)",
		`![alt](https://example.com/image.png "title"){width=100 height=200}`,
		"not an image",
		"![",
		"![]()",
	} {
		f.Add(seed)
	}

	f.Fuzz(func(t *testing.T, input string) {
		ParseImageSyntax(input)
	})
}
