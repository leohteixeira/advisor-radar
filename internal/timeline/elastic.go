package timeline

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// ElasticStore indexes timeline entries over Elasticsearch's HTTP API.
// It is used only when ELASTICSEARCH_URL is set.
type ElasticStore struct {
	base        string
	client      *http.Client
	clearClient *http.Client
	index       string
}

const (
	// documentTimeout bounds one document write.
	documentTimeout = 5 * time.Second
	// clearTimeout bounds the delete by query that empties the index. It scans
	// and deletes every document, so on a busy single-node cluster it takes far
	// longer than one document write.
	clearTimeout = 2 * time.Minute
)

// NewElasticStore returns a client for baseURL (no trailing slash).
func NewElasticStore(baseURL string) *ElasticStore {
	return newElasticStore(baseURL, documentTimeout, clearTimeout)
}

func newElasticStore(baseURL string, document, clearing time.Duration) *ElasticStore {
	return &ElasticStore{
		base:        strings.TrimRight(baseURL, "/"),
		index:       "customer-timeline",
		client:      &http.Client{Timeout: document},
		clearClient: &http.Client{Timeout: clearing},
	}
}

// Clear deletes every document of the index. The index itself is kept (the
// production role may write documents but not delete an index), and an index
// that does not exist yet is already clear.
func (e *ElasticStore) Clear(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	url := fmt.Sprintf("%s/%s/_delete_by_query", e.base, e.index)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, strings.NewReader(`{"query":{"match_all":{}}}`))
	if err != nil {
		return fmt.Errorf("timeline: elastic request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	res, err := e.clearClient.Do(req)
	if err != nil {
		return fmt.Errorf("timeline: elastic delete by query: %w", err)
	}
	defer res.Body.Close()
	if res.StatusCode == http.StatusNotFound {
		_, _ = io.Copy(io.Discard, res.Body)
		return nil
	}
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		_, _ = io.Copy(io.Discard, res.Body)
		return fmt.Errorf("timeline: elastic delete by query status %d", res.StatusCode)
	}
	// A 200 can still report documents it could not delete.
	var out struct {
		Failures []json.RawMessage `json:"failures"`
	}
	if err := json.NewDecoder(res.Body).Decode(&out); err != nil {
		return fmt.Errorf("timeline: decode elastic delete by query: %w", err)
	}
	if len(out.Failures) > 0 {
		return fmt.Errorf("timeline: elastic delete by query left %d failures", len(out.Failures))
	}
	return nil
}

// IndexDoc writes one entry. The request is an HTTP PUT; tests cover it with httptest.
func (e *ElasticStore) IndexDoc(ctx context.Context, entry Entry) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	body, err := json.Marshal(entry)
	if err != nil {
		return fmt.Errorf("timeline: marshal elastic doc: %w", err)
	}
	url := fmt.Sprintf("%s/%s/_doc/%s", e.base, e.index, entry.EventID)
	req, err := http.NewRequestWithContext(ctx, http.MethodPut, url, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("timeline: elastic request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	res, err := e.client.Do(req)
	if err != nil {
		return fmt.Errorf("timeline: elastic put: %w", err)
	}
	defer res.Body.Close()
	_, _ = io.Copy(io.Discard, res.Body)
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		return fmt.Errorf("timeline: elastic status %d", res.StatusCode)
	}
	return nil
}
