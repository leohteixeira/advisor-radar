package timeline_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/leohteixeira/advisor-radar/internal/event"
	"github.com/leohteixeira/advisor-radar/internal/identity"
	"github.com/leohteixeira/advisor-radar/internal/timeline"
)

func TestElasticStore_IndexDoc(t *testing.T) {
	t.Parallel()
	var gotMethod, gotPath, gotBody string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		gotPath = r.URL.Path
		b, _ := io.ReadAll(r.Body)
		gotBody = string(b)
		w.WriteHeader(http.StatusCreated)
	}))
	t.Cleanup(srv.Close)

	store := timeline.NewElasticStore(srv.URL)
	err := store.IndexDoc(context.Background(), timeline.Entry{
		EventID:    "ev-1",
		CustomerID: "c01",
		Kind:       "nota",
		Title:      "Nota",
		Text:       "Orlando",
		Meta:       "Ana",
		Ago:        10,
	})
	if err != nil {
		t.Fatalf("IndexDoc: %v", err)
	}
	if gotMethod != http.MethodPut {
		t.Fatalf("method = %q", gotMethod)
	}
	if !strings.Contains(gotPath, "ev-1") {
		t.Fatalf("path = %q", gotPath)
	}
	if !strings.Contains(gotBody, "Orlando") {
		t.Fatalf("body = %q", gotBody)
	}
}

// The stored document keeps the event time and no age: an age is only true at
// the instant it is read.
func TestElasticStore_IndexDocStoresOccurredAtNotAgo(t *testing.T) {
	t.Parallel()
	var doc map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&doc); err != nil {
			t.Errorf("decode doc: %v", err)
		}
		w.WriteHeader(http.StatusCreated)
	}))
	t.Cleanup(srv.Close)

	at := time.Date(2026, 11, 13, 19, 13, 29, 0, time.UTC)
	err := timeline.NewElasticStore(srv.URL).IndexDoc(context.Background(), timeline.Entry{
		EventID: "ev-1", CustomerID: "c01", Kind: "aporte", Title: "Aporte", Text: "US$ 10.000,00", Ago: 7, OccurredAt: at,
	})
	if err != nil {
		t.Fatalf("IndexDoc: %v", err)
	}
	if _, ok := doc["ago"]; ok {
		t.Errorf("doc carries ago: %v", doc)
	}
	if doc["occurred_at"] != "2026-11-13T19:13:29Z" || doc["text"] != "US$ 10.000,00" {
		t.Errorf("doc = %v, want occurred_at and text", doc)
	}
}

// elasticFake records the requests an ElasticStore makes and answers the
// delete by query with the given status and body.
type elasticFake struct {
	mu           sync.Mutex
	requests     []string
	bodies       []string
	clearStatus  int
	clearPayload string
}

func (f *elasticFake) handler(t *testing.T) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Errorf("read body: %v", err)
		}
		f.mu.Lock()
		f.requests = append(f.requests, r.Method+" "+r.URL.Path)
		f.bodies = append(f.bodies, string(body))
		f.mu.Unlock()
		if strings.HasSuffix(r.URL.Path, "/_delete_by_query") {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(f.clearStatus)
			_, _ = io.WriteString(w, f.clearPayload)
			return
		}
		w.WriteHeader(http.StatusCreated)
	})
}

func (f *elasticFake) seen() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return slices.Clone(f.requests)
}

func TestElasticStore_Clear(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		status  int
		payload string
		wantErr bool
	}{
		{name: "documents deleted", status: http.StatusOK, payload: `{"deleted":12,"failures":[]}`},
		{name: "index does not exist yet", status: http.StatusNotFound, payload: `{"error":{"type":"index_not_found_exception"},"status":404}`},
		{name: "some documents were not deleted", status: http.StatusOK, payload: `{"deleted":11,"failures":[{"index":"customer-timeline","id":"ev-1","status":409}]}`, wantErr: true},
		{name: "not allowed", status: http.StatusForbidden, payload: `{"error":{"type":"security_exception"},"status":403}`, wantErr: true},
		{name: "unavailable", status: http.StatusServiceUnavailable, payload: ``, wantErr: true},
		{name: "answer that is not JSON", status: http.StatusOK, payload: `<html>`, wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			fake := &elasticFake{clearStatus: tt.status, clearPayload: tt.payload}
			srv := httptest.NewServer(fake.handler(t))
			t.Cleanup(srv.Close)

			err := timeline.NewElasticStore(srv.URL + "/").Clear(context.Background())
			if (err != nil) != tt.wantErr {
				t.Fatalf("Clear error = %v, wantErr %v", err, tt.wantErr)
			}
			if got, want := fake.seen(), []string{"POST /customer-timeline/_delete_by_query"}; !slices.Equal(got, want) {
				t.Fatalf("requests = %v, want %v", got, want)
			}
			var query struct {
				Query struct {
					MatchAll *struct{} `json:"match_all"`
				} `json:"query"`
			}
			if err := json.Unmarshal([]byte(fake.bodies[0]), &query); err != nil || query.Query.MatchAll == nil {
				t.Fatalf("body = %q, %v, want a match_all query", fake.bodies[0], err)
			}
		})
	}
}

// A delete by query outlasts one document write on a busy cluster, so Clear
// must not share the document timeout.
func TestElasticStore_ClearOutlastsDocumentTimeout(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		time.Sleep(150 * time.Millisecond)
		_, _ = w.Write([]byte(`{}`))
	}))
	t.Cleanup(srv.Close)
	store := timeline.NewElasticStoreWithTimeouts(srv.URL, 20*time.Millisecond, 5*time.Second)
	if err := store.Clear(context.Background()); err != nil {
		t.Fatalf("Clear slower than the document timeout = %v, want nil", err)
	}
}

func TestElasticStore_ClearCanceledContext(t *testing.T) {
	t.Parallel()
	fake := &elasticFake{clearStatus: http.StatusOK, clearPayload: `{}`}
	srv := httptest.NewServer(fake.handler(t))
	t.Cleanup(srv.Close)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := timeline.NewElasticStore(srv.URL).Clear(ctx); err == nil {
		t.Fatal("Clear with a canceled context returned nil")
	}
	if got := fake.seen(); len(got) != 0 {
		t.Fatalf("requests = %v, want none", got)
	}
}

// At startup the Elasticsearch index is emptied before anything is replayed,
// so it ends holding exactly what the outboxes hold: a document whose event a
// reseed removed does not survive the restart.
func TestReplayOutboxes_ClearsElasticBeforeIndexing(t *testing.T) {
	t.Parallel()
	fake := &elasticFake{clearStatus: http.StatusOK, clearPayload: `{"deleted":3,"failures":[]}`}
	srv := httptest.NewServer(fake.handler(t))
	t.Cleanup(srv.Close)
	store := timeline.NewElasticStore(srv.URL)
	idx := timeline.NewIndex()
	ctx := context.Background()

	// No database is configured: the memory index is empty, and so must the
	// Elasticsearch index be.
	if err := timeline.ReplayOutboxes(ctx, idx, store); err != nil {
		t.Fatalf("ReplayOutboxes: %v", err)
	}
	row := outboxRow(t, event.NameAlertRaised, identity.MustNewV7(), map[string]any{"kind": "segmento", "reason": "Desceu de Advance para Essencial"})
	if err := timeline.ReplayRows(ctx, idx, store, []timeline.OutboxRow{row}); err != nil {
		t.Fatalf("ReplayRows: %v", err)
	}

	want := []string{
		"POST /customer-timeline/_delete_by_query",
		"PUT /customer-timeline/_doc/" + row.EventID,
	}
	if got := fake.seen(); !slices.Equal(got, want) {
		t.Fatalf("requests = %v, want %v", got, want)
	}
}

// A failed clear stops the startup: indexing over stale documents would leave
// Elasticsearch holding events the outboxes no longer have.
func TestReplayOutboxes_ClearFailureStops(t *testing.T) {
	t.Parallel()
	fake := &elasticFake{clearStatus: http.StatusServiceUnavailable}
	srv := httptest.NewServer(fake.handler(t))
	t.Cleanup(srv.Close)

	err := timeline.ReplayOutboxes(context.Background(), timeline.NewIndex(), timeline.NewElasticStore(srv.URL))
	if err == nil || !strings.Contains(err.Error(), "clear index") {
		t.Fatalf("ReplayOutboxes error = %v, want a clear index error", err)
	}
}

// Without Elasticsearch there is nothing to clear.
func TestReplayOutboxes_WithoutElastic(t *testing.T) {
	t.Parallel()
	if err := timeline.ReplayOutboxes(context.Background(), timeline.NewIndex(), nil); err != nil {
		t.Fatalf("ReplayOutboxes: %v", err)
	}
}
