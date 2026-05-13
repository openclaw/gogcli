package cmd

import (
	"errors"

	"google.golang.org/api/slides/v1"
)

const slideElementTitle = "title"

// SlidesToAPIRequests is stubbed in Task 1; filled in Task 15.
func SlidesToAPIRequests(_ []Slide) ([]*slides.Request, map[int]string) {
	return nil, map[int]string{}
}

// CreatePresentationFromMarkdown is stubbed in Task 1; filled in Task 18.
// Signature matches legacy exactly so slides.go callers still compile.
func CreatePresentationFromMarkdown(title string, markdown string, service *slides.Service) (*slides.Presentation, error) {
	return nil, errors.New("slidey renderer not yet wired (Task 15/18)")
}
