package coresdk

import "context"

// Page is the shape every cursor-paginated list endpoint in this
// platform returns: {"items": [...], "nextCursor": "..."} - an empty
// NextCursor means there is no further page.
type Page[T any] struct {
	Items      []T    `json:"items"`
	NextCursor string `json:"nextCursor"`
}

// PageFetcher fetches one page given an opaque cursor (""  for the
// first page) - each typed List method in operations.go adapts itself
// to this shape so Paginate works the same way regardless of endpoint.
type PageFetcher[T any] func(ctx context.Context, cursor string) (Page[T], error)

// Paginate walks every page from fetch, calling visit with each item in
// order. It stops at the first error from either fetch or visit.
// This is "pagination" as a real SDK responsibility: a caller writes
// one visit function instead of hand-rolling a cursor loop per list
// endpoint they touch.
func Paginate[T any](ctx context.Context, fetch PageFetcher[T], visit func(T) error) error {
	cursor := ""
	for {
		page, err := fetch(ctx, cursor)
		if err != nil {
			return err
		}
		for _, item := range page.Items {
			if err := visit(item); err != nil {
				return err
			}
		}
		if page.NextCursor == "" {
			return nil
		}
		cursor = page.NextCursor
	}
}

// CollectAll drains every page from fetch into a single slice - use for
// small lists where holding everything in memory is fine; Paginate is
// the streaming alternative for large ones.
func CollectAll[T any](ctx context.Context, fetch PageFetcher[T]) ([]T, error) {
	var all []T
	err := Paginate(ctx, fetch, func(item T) error {
		all = append(all, item)
		return nil
	})
	return all, err
}
