package coresdk

import (
	"context"
	"reflect"
	"testing"
)

func TestPaginateWalksEveryPage(t *testing.T) {
	pages := map[string]Page[int]{
		"":   {Items: []int{1, 2}, NextCursor: "p2"},
		"p2": {Items: []int{3, 4}, NextCursor: "p3"},
		"p3": {Items: []int{5}, NextCursor: ""},
	}
	fetch := func(_ context.Context, cursor string) (Page[int], error) {
		return pages[cursor], nil
	}

	var got []int
	err := Paginate(context.Background(), fetch, func(v int) error {
		got = append(got, v)
		return nil
	})
	if err != nil {
		t.Fatalf("Paginate returned error: %v", err)
	}
	want := []int{1, 2, 3, 4, 5}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
}

func TestCollectAllStopsAtEmptyCursor(t *testing.T) {
	calls := 0
	fetch := func(_ context.Context, cursor string) (Page[string], error) {
		calls++
		return Page[string]{Items: []string{"only"}, NextCursor: ""}, nil
	}
	items, err := CollectAll(context.Background(), fetch)
	if err != nil {
		t.Fatalf("CollectAll returned error: %v", err)
	}
	if len(items) != 1 || calls != 1 {
		t.Fatalf("expected exactly 1 fetch call and 1 item, got %d calls, %d items", calls, len(items))
	}
}
