package searchidx

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"

	opensearch "github.com/opensearch-project/opensearch-go/v4"
	"github.com/opensearch-project/opensearch-go/v4/opensearchapi"
)

type OpenSearchConfig struct {
	Addresses []string
	Username  string
	Password  string
	// Index is the single shared index every document type and app is
	// stored in, distinguished by the "type"/"appId" fields on each
	// document rather than one OpenSearch index per type - simpler to
	// operate for a generic platform capability that doesn't yet know how
	// many document types future domain modules will ever register.
	Index string
}

type OpenSearchProvider struct {
	client *opensearchapi.Client
	index  string
}

// osDocument is the actual shape stored in OpenSearch - Fields nested
// under its own key so a caller's field named e.g. "type" can never
// collide with this envelope's own type/appId/id metadata.
type osDocument struct {
	Type   string         `json:"type"`
	AppID  string         `json:"appId,omitempty"`
	ID     string         `json:"id"`
	Fields map[string]any `json:"fields"`
}

// NewOpenSearchProvider connects and ensures the configured index exists,
// creating it if not - the same self-healing bootstrap precedent as
// OpenFGA's store/model, Keycloak's realm import, and files' S3 bucket:
// a fresh `docker compose up` starts with no OpenSearch indices at all.
func NewOpenSearchProvider(ctx context.Context, cfg OpenSearchConfig) (*OpenSearchProvider, error) {
	client, err := opensearchapi.NewClient(opensearchapi.Config{
		Client: opensearch.Config{Addresses: cfg.Addresses, Username: cfg.Username, Password: cfg.Password},
	})
	if err != nil {
		return nil, fmt.Errorf("create opensearch client: %w", err)
	}
	p := &OpenSearchProvider{client: client, index: cfg.Index}
	if err := p.ensureIndex(ctx); err != nil {
		return nil, err
	}
	return p, nil
}

func (p *OpenSearchProvider) ensureIndex(ctx context.Context) error {
	resp, err := p.client.Indices.Exists(ctx, opensearchapi.IndicesExistsReq{Indices: []string{p.index}})
	if err == nil && resp != nil && resp.StatusCode == 200 {
		return nil
	}
	if _, err := p.client.Indices.Create(ctx, opensearchapi.IndicesCreateReq{Index: p.index}); err != nil {
		return fmt.Errorf("ensure index %q exists: %w", p.index, err)
	}
	return nil
}

// documentID compounds type+appId+id into one OpenSearch document ID, so
// Index (an upsert - re-indexing the same source entity replaces rather
// than duplicates) and Delete are both addressable without a prior
// lookup, and two different document types can never collide even if
// their source IDs happen to match.
func documentID(docType, appID, id string) string {
	return docType + "|" + appID + "|" + id
}

func (p *OpenSearchProvider) Index(ctx context.Context, doc Document) error {
	body, err := json.Marshal(osDocument{Type: doc.Type, AppID: doc.AppID, ID: doc.ID, Fields: doc.Fields})
	if err != nil {
		return fmt.Errorf("marshal document: %w", err)
	}
	_, err = p.client.Index(ctx, opensearchapi.IndexReq{
		Index: p.index, DocumentID: documentID(doc.Type, doc.AppID, doc.ID), Body: bytes.NewReader(body),
	})
	if err != nil {
		return fmt.Errorf("index document: %w", err)
	}
	return nil
}

func (p *OpenSearchProvider) Delete(ctx context.Context, docType, appID, id string) error {
	_, err := p.client.Document.Delete(ctx, opensearchapi.DocumentDeleteReq{
		Index: p.index, DocumentID: documentID(docType, appID, id),
	})
	if err != nil {
		return fmt.Errorf("delete document: %w", err)
	}
	return nil
}

func (p *OpenSearchProvider) Query(ctx context.Context, params QueryParams) (QueryResult, error) {
	var filters []map[string]any
	if params.Type != "" {
		filters = append(filters, map[string]any{"term": map[string]any{"type": params.Type}})
	}
	if params.AppID != "" {
		filters = append(filters, map[string]any{"term": map[string]any{"appId": params.AppID}})
	}
	boolQuery := map[string]any{"filter": filters}
	if params.Query != "" {
		boolQuery["must"] = []map[string]any{
			{"query_string": map[string]any{"query": params.Query, "fields": []string{"fields.*"}}},
		}
	} else {
		boolQuery["must"] = []map[string]any{{"match_all": map[string]any{}}}
	}

	limit := params.Limit
	if limit <= 0 {
		limit = 20
	}
	body, err := json.Marshal(map[string]any{
		"query": map[string]any{"bool": boolQuery}, "from": params.From, "size": limit,
	})
	if err != nil {
		return QueryResult{}, fmt.Errorf("marshal query: %w", err)
	}

	resp, err := p.client.Search(ctx, &opensearchapi.SearchReq{
		Indices: []string{p.index}, Body: bytes.NewReader(body),
	})
	if err != nil {
		return QueryResult{}, fmt.Errorf("search: %w", err)
	}

	result := QueryResult{Total: resp.Hits.Total.Value}
	for _, hit := range resp.Hits.Hits {
		var doc osDocument
		if err := json.Unmarshal(hit.Source, &doc); err != nil {
			return QueryResult{}, fmt.Errorf("unmarshal hit source: %w", err)
		}
		result.Items = append(result.Items, Result{
			Document: Document{Type: doc.Type, AppID: doc.AppID, ID: doc.ID, Fields: doc.Fields},
			Score:    float64(hit.Score),
		})
	}
	return result, nil
}
