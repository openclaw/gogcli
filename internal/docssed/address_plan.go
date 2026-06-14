//nolint:err113,wsl_v5 // Address diagnostics retain exact CLI text; planning stages stay adjacent.
package docssed

import (
	"fmt"
	"strings"
)

// AddressElementKind identifies one numbered top-level document element.
type AddressElementKind string

const (
	AddressParagraph AddressElementKind = "paragraph"
	AddressTable     AddressElementKind = "table"
	AddressTOC       AddressElementKind = "toc"
)

// AddressElement is one addressable top-level document element.
type AddressElement struct {
	Number     int
	Kind       AddressElementKind
	Text       string
	StartIndex int64
	EndIndex   int64
}

// AddressMutation describes one delete and/or insertion at a document range.
type AddressMutation struct {
	StartIndex int64
	EndIndex   int64
	InsertText string
}

// ResolveAddress resolves a one-based address against numbered document elements.
func ResolveAddress(address *Address, elements []AddressElement) ([]AddressElement, error) {
	if address == nil {
		return nil, fmt.Errorf("nil address")
	}
	if len(elements) == 0 {
		return nil, fmt.Errorf("document has no paragraphs")
	}

	last := len(elements)
	start := resolveAddressNumber(address.Start, last)
	if start < 1 || start > last {
		return nil, fmt.Errorf("address %d out of range (document has %d paragraphs)", start, last)
	}
	if !address.HasRange {
		return elements[start-1 : start], nil
	}

	end := address.End
	if end == 0 {
		end = start
	} else {
		end = resolveAddressNumber(end, last)
	}
	if end < 1 || end > last {
		return nil, fmt.Errorf("address end %d out of range (document has %d paragraphs)", end, last)
	}
	if end < start {
		return nil, fmt.Errorf("address range %d,%d is reversed", start, end)
	}
	return elements[start-1 : end], nil
}

// PlanAddressedDelete plans deletion ranges in document order.
func PlanAddressedDelete(elements, targets []AddressElement) []AddressMutation {
	if isContiguousTerminalRange(elements, targets) {
		return []AddressMutation{{
			StartIndex: targets[0].StartIndex,
			EndIndex:   targets[len(targets)-1].EndIndex - 1,
		}}
	}

	mutations := make([]AddressMutation, 0, len(targets))
	for _, target := range targets {
		startIndex := target.StartIndex
		endIndex := target.EndIndex
		isLast := target.Number == len(elements)
		switch {
		case isLast && target.Number > 1:
			startIndex = elements[target.Number-2].EndIndex - 1
			endIndex = target.EndIndex - 1
		case isLast && target.Number == 1:
			if target.StartIndex >= target.EndIndex-1 {
				continue
			}
			endIndex = target.EndIndex - 1
		}
		mutations = append(mutations, AddressMutation{
			StartIndex: startIndex,
			EndIndex:   endIndex,
		})
	}
	return mutations
}

func isContiguousTerminalRange(elements, targets []AddressElement) bool {
	if len(targets) < 2 || targets[len(targets)-1].Number != len(elements) {
		return false
	}
	for index := 1; index < len(targets); index++ {
		if targets[index].Number != targets[index-1].Number+1 {
			return false
		}
	}
	return true
}

// PlanAddressedInsert plans text insertion before each target.
func PlanAddressedInsert(targets []AddressElement, replacement string) []AddressMutation {
	insertText := strings.ReplaceAll(replacement, "\\n", "\n")
	if !strings.HasSuffix(insertText, "\n") {
		insertText += "\n"
	}
	mutations := make([]AddressMutation, 0, len(targets))
	for _, target := range targets {
		mutations = append(mutations, AddressMutation{
			StartIndex: target.StartIndex,
			EndIndex:   target.StartIndex,
			InsertText: insertText,
		})
	}
	return mutations
}

// PlanAddressedAppend plans text insertion before each target's trailing newline.
func PlanAddressedAppend(targets []AddressElement, replacement string) []AddressMutation {
	insertText := strings.ReplaceAll(replacement, "\\n", "\n")
	if strings.HasSuffix(insertText, "\n") {
		insertText = "\n" + strings.TrimSuffix(insertText, "\n")
	} else {
		insertText = "\n" + insertText
	}
	mutations := make([]AddressMutation, 0, len(targets))
	for _, target := range targets {
		index := target.EndIndex - 1
		mutations = append(mutations, AddressMutation{
			StartIndex: index,
			EndIndex:   index,
			InsertText: insertText,
		})
	}
	return mutations
}

func resolveAddressNumber(number, last int) int {
	if number == -1 {
		return last
	}
	return number
}
