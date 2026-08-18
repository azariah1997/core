// Package searchidx is the shared search-indexing contract between
// core-api (which exposes the platform search API applications actually
// call) and worker (which indexes documents as domain events happen).
// Both need to agree on exactly the same Document shape and Provider
// behaviour, the same reason platformkit/rtbus exists for realtime
// pub/sub: two services acting on the same data must speak one contract,
// not two independently-maintained ones that can drift.
package searchidx

import "context"

// DefaultIndex is the shared OpenSearch index name both core-api (query)
// and worker (indexing) use by default - a single constant here so the
// two services can never point at different indices by typo.
const DefaultIndex = "platform_search_documents"

// Document is deliberately generic - Type is a free-form, product-defined
// string ("user", "message", "application", or anything a future domain
// module needs), never a fixed platform enum, the same "do not hardcode
// product concepts" convention this repo already applies to
// RelationshipType, Message.Type and friends. AppID is optional: some
// entities (like platform Users) aren't scoped to one application.
type Document struct {
	Type   string
	ID     string // the source entity's own ID - stable across re-indexing
	AppID  string // empty for platform-global documents
	Fields map[string]any
}

type QueryParams struct {
	Type  string // optional filter
	AppID string // optional filter
	Query string // free-text query against every indexed field
	Limit int
	From  int // offset-based paging - fine for search, unlike cursor-based list APIs elsewhere in this repo
}

type Result struct {
	Document Document
	Score    float64
}

type QueryResult struct {
	Items []Result
	Total int
}

// Provider is the platform's abstraction over the actual search engine -
// "applications should not access OpenSearch directly" is the roadmap's
// own framing, and this interface is the boundary that makes swapping
// OpenSearch for another engine later a Provider implementation, not a
// rewrite of every caller.
type Provider interface {
	Index(ctx context.Context, doc Document) error
	Delete(ctx context.Context, docType, appID, id string) error
	Query(ctx context.Context, params QueryParams) (QueryResult, error)
}
