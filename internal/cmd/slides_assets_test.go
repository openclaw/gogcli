package cmd

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestFASVGURL(t *testing.T) {
	cases := []struct {
		style, name, expected string
	}{
		{"solid", "truck-fast", "https://cdn.jsdelivr.net/npm/@fortawesome/fontawesome-free@6/svgs/solid/truck-fast.svg"},
		{"brands", "github", "https://cdn.jsdelivr.net/npm/@fortawesome/fontawesome-free@6/svgs/brands/github.svg"},
		{"regular", "clock", "https://cdn.jsdelivr.net/npm/@fortawesome/fontawesome-free@6/svgs/regular/clock.svg"},
	}
	for _, tc := range cases {
		assert.Equal(t, tc.expected, faSVGURL(tc.style, tc.name))
	}
}
