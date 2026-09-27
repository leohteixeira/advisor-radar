// Package jev is a thin client for the Jev evaluation model exposed by the
// Vercel AI Gateway native HTTP API (POST /v1/evaluate).
//
// Retries belong to the resilience stack; this client performs one HTTP attempt.
package jev

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"time"
)

const (
	DefaultBaseURL = "https://ai-gateway.vercel.sh"
	DefaultModel   = "typesafe-ai/jev"
)

// Question types accepted by the gateway evaluation API.
const (
	TypeBoolean = "boolean"
	TypeChoice  = "choice"
	TypeScore   = "score"
)

// Question is one typed question evaluated against the shared state.
// Criteria is map[string]string for choice, []string (lowest to highest) for
// score, and optionally {"true": ..., "false": ...} for boolean.
type Question struct {
	Type         string `json:"type"`
	Instructions string `json:"instructions"`
	Criteria     any    `json:"criteria,omitempty"`
}

func Boolean(instructions string) Question {
	return Question{Type: TypeBoolean, Instructions: instructions}
}

func BooleanWith(instructions, whenTrue, whenFalse string) Question {
	return Question{Type: TypeBoolean, Instructions: instructions,
		Criteria: map[string]string{"true": whenTrue, "false": whenFalse}}
}

func Choice(instructions string, options map[string]string) Question {
	return Question{Type: TypeChoice, Instructions: instructions, Criteria: options}
}

func Score(instructions string, levels ...string) Question {
	return Question{Type: TypeScore, Instructions: instructions, Criteria: levels}
}

// Request mirrors the /v1/evaluate body. State may be a string, object or array.
type Request struct {
	Model           string              `json:"model"`
	State           any                 `json:"state"`
	Questions       map[string]Question `json:"questions"`
	ProviderOptions map[string]any      `json:"providerOptions,omitempty"`
}

// Answer holds the union of fields returned by the three question types.
type Answer struct {
	Type          string             `json:"type"`
	Probability   *float64           `json:"probability,omitempty"` // boolean
	Choice        string             `json:"choice,omitempty"`      // choice
	Score         *float64           `json:"score,omitempty"`       // score
	Probabilities map[string]float64 `json:"probabilities,omitempty"`
	Confidence    *float64           `json:"confidence,omitempty"`
}

type Usage struct {
	InputTokens  int `json:"inputTokens"`
	OutputTokens int `json:"outputTokens"`
}

type Response struct {
	Model            string            `json:"model"`
	Answers          map[string]Answer `json:"answers"`
	Usage            Usage             `json:"usage"`
	ProviderMetadata json.RawMessage   `json:"providerMetadata,omitempty"`
}

// APIError is returned for non 2xx responses.
type APIError struct {
	Status     int
	Message    string `json:"message"`
	ErrorType  string `json:"error_type"`
	RetryAfter time.Duration
}

func (e *APIError) Error() string {
	return fmt.Sprintf("jev: http %d %s: %s", e.Status, e.ErrorType, e.Message)
}

// IsProviderFailure reports whether the error counts against the circuit breaker.
func (e *APIError) IsProviderFailure() bool {
	if e == nil {
		return false
	}
	switch e.Status {
	case http.StatusTooManyRequests, http.StatusUnauthorized, http.StatusPaymentRequired, http.StatusForbidden:
		return true
	}
	return e.Status >= 500
}

// IsInvalidRequest reports HTTP 400 invalid_request payload defects.
func (e *APIError) IsInvalidRequest() bool {
	return e != nil && e.Status == http.StatusBadRequest && e.ErrorType == "invalid_request"
}

// IsQuotaExceeded reports 402 quota_for_entity_exceeded.
func (e *APIError) IsQuotaExceeded() bool {
	return e != nil && e.Status == http.StatusPaymentRequired && e.ErrorType == "quota_for_entity_exceeded"
}

type Client struct {
	baseURL string
	apiKey  string
	model   string
	http    *http.Client
	zdr     bool
}

type Option func(*Client)

func WithBaseURL(u string) Option          { return func(c *Client) { c.baseURL = u } }
func WithModel(m string) Option            { return func(c *Client) { c.model = m } }
func WithHTTPClient(h *http.Client) Option { return func(c *Client) { c.http = h } }

// WithZeroDataRetention asks the gateway to only route to providers that
// honour zero data retention for this request.
func WithZeroDataRetention() Option { return func(c *Client) { c.zdr = true } }

func New(apiKey string, opts ...Option) *Client {
	c := &Client{
		baseURL: DefaultBaseURL,
		apiKey:  apiKey,
		model:   DefaultModel,
		http:    &http.Client{},
	}
	for _, o := range opts {
		o(c)
	}
	return c
}

// Evaluate sends one state with its questions in a single HTTP attempt.
func (c *Client) Evaluate(ctx context.Context, state any, questions map[string]Question) (*Response, error) {
	req := Request{Model: c.model, State: state, Questions: questions}
	if c.zdr {
		req.ProviderOptions = map[string]any{"gateway": map[string]any{"zeroDataRetention": true}}
	}
	body, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("jev: marshal request: %w", err)
	}
	return c.do(ctx, body)
}

func (c *Client) do(ctx context.Context, body []byte) (*Response, error) {
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/v1/evaluate", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("jev: build request: %w", err)
	}
	httpReq.Header.Set("Authorization", "Bearer "+c.apiKey)
	httpReq.Header.Set("Content-Type", "application/json")

	res, err := c.http.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("jev: request: %w", err)
	}
	defer res.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(res.Body, 1<<20))
	if err != nil {
		return nil, fmt.Errorf("jev: read body: %w", err)
	}

	if res.StatusCode < 200 || res.StatusCode > 299 {
		apiErr := &APIError{Status: res.StatusCode, RetryAfter: parseRetryAfter(res.Header.Get("Retry-After"))}
		if json.Unmarshal(raw, apiErr) != nil || apiErr.Message == "" {
			apiErr.Message = string(raw)
		}
		return nil, apiErr
	}

	var out Response
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, fmt.Errorf("jev: decode response: %w", err)
	}
	return &out, nil
}

func parseRetryAfter(v string) time.Duration {
	if v == "" {
		return 0
	}
	if secs, err := strconv.Atoi(v); err == nil && secs >= 0 {
		return time.Duration(secs) * time.Second
	}
	if t, err := http.ParseTime(v); err == nil {
		d := time.Until(t)
		if d > 0 {
			return d
		}
	}
	return 0
}
