package search_test

import (
	"context"
	"errors"
	"testing"

	"github.com/example/core-platform/backend/core-api/internal/search"
	"github.com/example/core-platform/packages/go/platformkit/searchidx"
)

type fakeAdmin struct{ admins map[string]bool }

func (a fakeAdmin) IsPlatformAdmin(ctx context.Context, userID string) (bool, error) {
	return a.admins[userID], nil
}

type fakeProvider struct {
	docs    map[string]searchidx.Document
	queried searchidx.QueryParams
}

func newFakeProvider() *fakeProvider { return &fakeProvider{docs: map[string]searchidx.Document{}} }

func key(t, a, id string) string { return t + "|" + a + "|" + id }

func (p *fakeProvider) Index(ctx context.Context, doc searchidx.Document) error {
	p.docs[key(doc.Type, doc.AppID, doc.ID)] = doc
	return nil
}

func (p *fakeProvider) Delete(ctx context.Context, docType, appID, id string) error {
	delete(p.docs, key(docType, appID, id))
	return nil
}

func (p *fakeProvider) Query(ctx context.Context, params searchidx.QueryParams) (searchidx.QueryResult, error) {
	p.queried = params
	var items []searchidx.Result
	for _, d := range p.docs {
		if params.Type != "" && d.Type != params.Type {
			continue
		}
		if params.AppID != "" && d.AppID != params.AppID {
			continue
		}
		items = append(items, searchidx.Result{Document: d, Score: 1})
	}
	return searchidx.QueryResult{Items: items, Total: len(items)}, nil
}

func TestQueryIsOpenToAnyAuthenticatedCaller(t *testing.T) {
	provider := newFakeProvider()
	_ = provider.Index(context.Background(), searchidx.Document{Type: "user", ID: "u1", Fields: map[string]any{"displayName": "Alice"}})
	svc := search.NewService(provider, fakeAdmin{})

	result, err := svc.Query(context.Background(), search.QueryInput{Type: "user"})
	if err != nil {
		t.Fatalf("query: %v", err)
	}
	if len(result.Items) != 1 {
		t.Fatalf("expected 1 result, got %d", len(result.Items))
	}
}

func TestIndexRequiresAdmin(t *testing.T) {
	svc := search.NewService(newFakeProvider(), fakeAdmin{})
	err := svc.Index(context.Background(), "u1", search.IndexInput{Type: "user", ID: "u1"})
	if !errors.Is(err, search.ErrForbidden) {
		t.Fatalf("expected ErrForbidden, got %v", err)
	}
}

func TestAdminCanIndexAndDelete(t *testing.T) {
	provider := newFakeProvider()
	svc := search.NewService(provider, fakeAdmin{admins: map[string]bool{"admin": true}})
	ctx := context.Background()

	if err := svc.Index(ctx, "admin", search.IndexInput{Type: "user", ID: "u1", Fields: map[string]any{"displayName": "Alice"}}); err != nil {
		t.Fatalf("index: %v", err)
	}
	result, err := svc.Query(ctx, search.QueryInput{Type: "user"})
	if err != nil {
		t.Fatalf("query: %v", err)
	}
	if len(result.Items) != 1 {
		t.Fatalf("expected 1 result after indexing, got %d", len(result.Items))
	}

	if err := svc.Delete(ctx, "admin", "user", "", "u1"); err != nil {
		t.Fatalf("delete: %v", err)
	}
	result, err = svc.Query(ctx, search.QueryInput{Type: "user"})
	if err != nil {
		t.Fatalf("query after delete: %v", err)
	}
	if len(result.Items) != 0 {
		t.Fatalf("expected 0 results after delete, got %d", len(result.Items))
	}
}

func TestQueryClampsLimitToDefault(t *testing.T) {
	provider := newFakeProvider()
	svc := search.NewService(provider, fakeAdmin{})
	if _, err := svc.Query(context.Background(), search.QueryInput{Limit: -5}); err == nil {
		t.Fatal("expected negative limit to be rejected by Validate")
	}
	if _, err := svc.Query(context.Background(), search.QueryInput{Limit: 99999}); err != nil {
		t.Fatalf("query: %v", err)
	}
	if provider.queried.Limit != 20 {
		t.Fatalf("expected an oversized limit to clamp to the default (20), got %d", provider.queried.Limit)
	}
}
