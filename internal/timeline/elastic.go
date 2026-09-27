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
	base   string
	client *http.Client
	index  string
}

// NewElasticStore returns a client for baseURL (no trailing slash).
func NewElasticStore(baseURL string) *ElasticStore {
	return &ElasticStore{
		base:  strings.TrimRight(baseURL, "/"),
		index: "customer-timeline",
		client: &http.Client{
			Timeout: 5 * time.Second,
		},
	}
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
