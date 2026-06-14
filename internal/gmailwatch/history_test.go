package gmailwatch

import (
	"reflect"
	"testing"
)

func TestCollectHistoryMessageIDs(t *testing.T) {
	t.Parallel()

	got := CollectHistoryMessageIDs([]HistoryRecord{
		{
			Added:         []string{"m1", "m1"},
			Deleted:       []string{"m4"},
			LabelsAdded:   []string{"m5"},
			LabelsRemoved: []string{"m6"},
			Messages:      []string{"m2", ""},
		},
		{Messages: []string{"m3"}},
	})

	if want := []string{"m1", "m5", "m6", "m2", "m3"}; !reflect.DeepEqual(got.FetchIDs, want) {
		t.Fatalf("fetch IDs = %v, want %v", got.FetchIDs, want)
	}

	if want := []string{"m4"}; !reflect.DeepEqual(got.DeletedIDs, want) {
		t.Fatalf("deleted IDs = %v, want %v", got.DeletedIDs, want)
	}
}

func TestCollectHistoryMessageIDsDeletedWins(t *testing.T) {
	t.Parallel()

	got := CollectHistoryMessageIDs([]HistoryRecord{
		{Added: []string{"m1", "m2"}},
		{Deleted: []string{"m1"}},
		{Added: []string{"m1"}},
	})

	if want := []string{"m2"}; !reflect.DeepEqual(got.FetchIDs, want) {
		t.Fatalf("fetch IDs = %v, want %v", got.FetchIDs, want)
	}

	if want := []string{"m1"}; !reflect.DeepEqual(got.DeletedIDs, want) {
		t.Fatalf("deleted IDs = %v, want %v", got.DeletedIDs, want)
	}
}

func TestCollectHistoryMessageIDsEmpty(t *testing.T) {
	t.Parallel()

	got := CollectHistoryMessageIDs(nil)
	if len(got.FetchIDs) != 0 || len(got.DeletedIDs) != 0 {
		t.Fatalf("result = %#v", got)
	}
}
