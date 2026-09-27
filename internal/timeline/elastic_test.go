package timeline_test

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

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
