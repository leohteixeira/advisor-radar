package jev

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
)

func TestEvaluateSendsContractAndDecodesAnswers(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/evaluate" || r.Method != http.MethodPost {
			t.Fatalf("unexpected %s %s", r.Method, r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer k" {
			t.Fatalf("auth header = %q", got)
		}
		var req Request
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatal(err)
		}
		if req.Model != DefaultModel || req.Questions["route"].Type != TypeChoice {
			t.Fatalf("bad request: %+v", req)
		}
		zdr := req.ProviderOptions["gateway"].(map[string]any)["zeroDataRetention"]
		if zdr != true {
			t.Fatalf("zdr not sent: %+v", req.ProviderOptions)
		}
		_, _ = w.Write([]byte(`{
			"model":"typesafe-ai/jev",
			"answers":{
				"route":{"type":"choice","choice":"billing","probabilities":{"billing":0.97,"shipping":0.03}},
				"refund":{"type":"boolean","probability":0.91},
				"urgency":{"type":"score","score":1.8,"probabilities":{"0":0.05,"1":0.1,"2":0.85}}
			},
			"usage":{"inputTokens":300,"outputTokens":20},
			"providerMetadata":{"gateway":{"cost":"0.00001"}}
		}`))
	}))
	defer srv.Close()

	c := New("k", WithBaseURL(srv.URL), WithZeroDataRetention())
	resp, err := c.Evaluate(context.Background(), "state", map[string]Question{
		"route":   Choice("route", map[string]string{"billing": "b", "shipping": "s"}),
		"refund":  Boolean("refund?"),
		"urgency": Score("urgency", "low", "mid", "high"),
	})
	if err != nil {
		t.Fatal(err)
	}
	if resp.Answers["route"].Choice != "billing" || *resp.Answers["refund"].Probability != 0.91 || *resp.Answers["urgency"].Score != 1.8 {
		t.Fatalf("bad decode: %+v", resp.Answers)
	}
	if resp.Usage.InputTokens != 300 {
		t.Fatalf("usage = %+v", resp.Usage)
	}
}

func TestEvaluateDoesNotRetryServerErrors(t *testing.T) {
	t.Parallel()

	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer srv.Close()

	c := New("k", WithBaseURL(srv.URL))
	_, err := c.Evaluate(context.Background(), "s", nil)
	var apiErr *APIError
	if !errors.As(err, &apiErr) || apiErr.Status != http.StatusServiceUnavailable {
		t.Fatalf("err = %v", err)
	}
	if calls.Load() != 1 {
		t.Fatalf("calls = %d, want 1 (no inner retry)", calls.Load())
	}
}

func TestEvaluateDoesNotRetryInvalidRequest(t *testing.T) {
	t.Parallel()

	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"message":"questions.x.type: expected one of 'boolean','choice','score'","error_type":"invalid_request"}`))
	}))
	defer srv.Close()

	c := New("k", WithBaseURL(srv.URL))
	_, err := c.Evaluate(context.Background(), "s", nil)
	var apiErr *APIError
	if !errors.As(err, &apiErr) || apiErr.ErrorType != "invalid_request" {
		t.Fatalf("err = %v", err)
	}
	if !apiErr.IsInvalidRequest() {
		t.Fatal("expected invalid request")
	}
	if calls.Load() != 1 {
		t.Fatalf("calls = %d", calls.Load())
	}
}

func TestEvaluateCapturesRetryAfter(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Retry-After", "2")
		w.WriteHeader(http.StatusTooManyRequests)
		_, _ = w.Write([]byte(`{"message":"slow down","error_type":"rate_limited"}`))
	}))
	defer srv.Close()

	c := New("k", WithBaseURL(srv.URL))
	_, err := c.Evaluate(context.Background(), "s", nil)
	var apiErr *APIError
	if !errors.As(err, &apiErr) {
		t.Fatalf("err = %v", err)
	}
	if apiErr.RetryAfter.Seconds() != 2 {
		t.Fatalf("RetryAfter = %v", apiErr.RetryAfter)
	}
}
