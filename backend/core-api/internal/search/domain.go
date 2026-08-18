// Package search is the platform's single search entry point -
// "applications should not access OpenSearch directly" is the roadmap's
// own framing. Service wraps a platformkit/searchidx.Provider (OpenSearch
// in production and locally); callers never see OpenSearch, only this
// package's Query/Index/Delete.
package search

import (
	"errors"
	"strings"

	"github.com/example/core-platform/packages/go/platformkit/searchidx"
)

type ValidationError struct{ Message string }

func (e *ValidationError) Error() string { return e.Message }

var ErrForbidden = errors.New("not permitted to manage the search index")

// QueryInput mirrors searchidx.QueryParams - kept as its own type so
// callers depend on this package's contract, not the shared
// infrastructure package's, the same layering as every other module here.
type QueryInput struct {
	Type  string
	AppID string
	Query string
	Limit int
	From  int
}

func (in QueryInput) Validate() error {
	if in.Limit < 0 || in.From < 0 {
		return &ValidationError{"limit and from must not be negative"}
	}
	return nil
}

// IndexInput is the manual/admin indexing path - the automatic path is
// worker's event-driven indexer (see backend/worker/internal/indexer),
// not this package; this exists for on-demand re-indexing.
type IndexInput struct {
	Type   string
	AppID  string
	ID     string
	Fields map[string]any
}

func (in IndexInput) Validate() error {
	switch {
	case strings.TrimSpace(in.Type) == "":
		return &ValidationError{"type is required"}
	case strings.TrimSpace(in.ID) == "":
		return &ValidationError{"id is required"}
	}
	return nil
}

func toParams(in QueryInput) searchidx.QueryParams {
	return searchidx.QueryParams{Type: in.Type, AppID: in.AppID, Query: in.Query, Limit: in.Limit, From: in.From}
}

func toDocument(in IndexInput) searchidx.Document {
	return searchidx.Document{Type: in.Type, AppID: in.AppID, ID: in.ID, Fields: in.Fields}
}
