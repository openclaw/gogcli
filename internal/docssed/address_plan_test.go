//nolint:wsl_v5 // Address fixtures stay compact around exact diagnostics and ranges.
package docssed

import (
	"reflect"
	"testing"
)

func TestResolveAddress(t *testing.T) {
	t.Parallel()
	elements := addressTestElements()
	tests := []struct {
		name    string
		address *Address
		want    []int
		wantErr string
	}{
		{name: "single", address: &Address{Start: 3}, want: []int{3}},
		{name: "last", address: &Address{Start: -1}, want: []int{5}},
		{name: "range", address: &Address{Start: 2, End: 4, HasRange: true}, want: []int{2, 3, 4}},
		{name: "range to last", address: &Address{Start: 3, End: -1, HasRange: true}, want: []int{3, 4, 5}},
		{name: "implicit range end", address: &Address{Start: 3, HasRange: true}, want: []int{3}},
		{name: "nil", wantErr: "nil address"},
		{
			name:    "start out of range",
			address: &Address{Start: 10},
			wantErr: "address 10 out of range (document has 5 paragraphs)",
		},
		{
			name:    "end out of range",
			address: &Address{Start: 2, End: 10, HasRange: true},
			wantErr: "address end 10 out of range (document has 5 paragraphs)",
		},
		{
			name:    "reversed",
			address: &Address{Start: 4, End: 2, HasRange: true},
			wantErr: "address range 4,2 is reversed",
		},
	}

	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			targets, err := ResolveAddress(test.address, elements)
			if test.wantErr != "" {
				if err == nil || err.Error() != test.wantErr {
					t.Fatalf("error = %v, want %q", err, test.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			numbers := make([]int, len(targets))
			for index, target := range targets {
				numbers[index] = target.Number
			}
			if !reflect.DeepEqual(numbers, test.want) {
				t.Fatalf("numbers = %v, want %v", numbers, test.want)
			}
		})
	}
}

func TestResolveAddressEmptyDocument(t *testing.T) {
	t.Parallel()
	_, err := ResolveAddress(&Address{Start: 1}, nil)
	if err == nil || err.Error() != "document has no paragraphs" {
		t.Fatalf("error = %v", err)
	}
}

func TestPlanAddressedDelete(t *testing.T) {
	t.Parallel()
	elements := addressTestElements()
	mutations := PlanAddressedDelete(elements, []AddressElement{elements[1], elements[4]})
	want := []AddressMutation{
		{StartIndex: 6, EndIndex: 13},
		{StartIndex: 25, EndIndex: 31},
	}
	if !reflect.DeepEqual(mutations, want) {
		t.Fatalf("mutations = %#v, want %#v", mutations, want)
	}

	only := []AddressElement{{Number: 1, Kind: AddressParagraph, StartIndex: 1, EndIndex: 6}}
	if got := PlanAddressedDelete(only, only); !reflect.DeepEqual(got, []AddressMutation{{
		StartIndex: 1,
		EndIndex:   5,
	}}) {
		t.Fatalf("only paragraph = %#v", got)
	}
	empty := []AddressElement{{Number: 1, Kind: AddressParagraph, StartIndex: 1, EndIndex: 2}}
	if got := PlanAddressedDelete(empty, empty); len(got) != 0 {
		t.Fatalf("empty paragraph = %#v", got)
	}

	terminalRange := PlanAddressedDelete(elements, elements[3:])
	if !reflect.DeepEqual(terminalRange, []AddressMutation{{
		StartIndex: 19,
		EndIndex:   31,
	}}) {
		t.Fatalf("terminal range = %#v", terminalRange)
	}
}

func TestPlanAddressedInsertAndAppend(t *testing.T) {
	t.Parallel()
	targets := addressTestElements()[1:3]
	insert := PlanAddressedInsert(targets, `first\nsecond`)
	wantInsert := []AddressMutation{
		{StartIndex: 6, EndIndex: 6, InsertText: "first\nsecond\n"},
		{StartIndex: 13, EndIndex: 13, InsertText: "first\nsecond\n"},
	}
	if !reflect.DeepEqual(insert, wantInsert) {
		t.Fatalf("insert = %#v, want %#v", insert, wantInsert)
	}

	appendPlan := PlanAddressedAppend(targets, "after\n")
	wantAppend := []AddressMutation{
		{StartIndex: 12, EndIndex: 12, InsertText: "\nafter"},
		{StartIndex: 18, EndIndex: 18, InsertText: "\nafter"},
	}
	if !reflect.DeepEqual(appendPlan, wantAppend) {
		t.Fatalf("append = %#v, want %#v", appendPlan, wantAppend)
	}
}

func addressTestElements() []AddressElement {
	return []AddressElement{
		{Number: 1, Kind: AddressParagraph, Text: "first", StartIndex: 0, EndIndex: 6},
		{Number: 2, Kind: AddressTable, Text: "table", StartIndex: 6, EndIndex: 13},
		{Number: 3, Kind: AddressTOC, Text: "toc", StartIndex: 13, EndIndex: 19},
		{Number: 4, Kind: AddressParagraph, Text: "fourth", StartIndex: 19, EndIndex: 26},
		{Number: 5, Kind: AddressParagraph, Text: "fifth", StartIndex: 26, EndIndex: 32},
	}
}
