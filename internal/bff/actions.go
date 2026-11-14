package bff

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"
)

// ErrActionsUnavailable is returned when ADVISORY_HTTP_URL is unset.
var ErrActionsUnavailable = errors.New("bff: advisory actions unavailable")

// SignalAction is the advisory action row as seen by the BFF.
type SignalAction struct {
	SignalID     string     `json:"signal_id"`
	ContactedAt  *time.Time `json:"contacted_at,omitempty"`
	SnoozedUntil *time.Time `json:"snoozed_until,omitempty"`
}

// CaseCommand asks advisory to open a case from a queue signal (ADR 0013).
// Label is the signal's display label; the four facts are the ones that halve
// an SLA. IdempotencyKey is the browser's: the same key for the same signal
// is the same command.
type CaseCommand struct {
	SignalID           string
	CustomerID         string
	IdempotencyKey     string
	Label              string
	ChurnRisk          bool
	Frustration        int
	HumanRequested     bool
	RelevantWithdrawal bool
}

// ActionsClient reaches advisory over HTTP for what an advisor does to a
// signal: Contatado / Adiar / Desfazer and Abrir caso.
type ActionsClient interface {
	List(ctx context.Context) ([]SignalAction, error)
	Contact(ctx context.Context, signalID string) error
	Snooze(ctx context.Context, signalID string) error
	Undo(ctx context.Context, signalID string) error
	// OpenCase hands the command to advisory and returns the id advisory
	// accepted it under. The case itself is opened later, by cases.
	OpenCase(ctx context.Context, cmd CaseCommand) (requestID string, err error)
}

// UnavailableActions always returns ErrActionsUnavailable.
type UnavailableActions struct{}

// List implements ActionsClient.
func (UnavailableActions) List(context.Context) ([]SignalAction, error) {
	return nil, ErrActionsUnavailable
}

// Contact implements ActionsClient.
func (UnavailableActions) Contact(context.Context, string) error {
	return ErrActionsUnavailable
}

// Snooze implements ActionsClient.
func (UnavailableActions) Snooze(context.Context, string) error {
	return ErrActionsUnavailable
}

// Undo implements ActionsClient.
func (UnavailableActions) Undo(context.Context, string) error {
	return ErrActionsUnavailable
}

// OpenCase implements ActionsClient.
func (UnavailableActions) OpenCase(context.Context, CaseCommand) (string, error) {
	return "", ErrActionsUnavailable
}

// HTTPActionsClient calls advisory's action routes.
type HTTPActionsClient struct {
	base   string
	client *http.Client
}

// NewHTTPActionsClient returns a client for baseURL (no trailing slash).
func NewHTTPActionsClient(baseURL string) *HTTPActionsClient {
	return &HTTPActionsClient{
		base: baseURL,
		client: &http.Client{
			Timeout: 5 * time.Second,
		},
	}
}

// NewActionsClient returns UnavailableActions when url is empty.
func NewActionsClient(url string) ActionsClient {
	if url == "" {
		return UnavailableActions{}
	}
	return NewHTTPActionsClient(url)
}

// List implements ActionsClient.
func (c *HTTPActionsClient) List(ctx context.Context) ([]SignalAction, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.base+"/v1/actions", nil)
	if err != nil {
		return nil, fmt.Errorf("bff: actions list request: %w", err)
	}
	res, err := c.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("bff: actions list: %w", err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("bff: actions list: status %d", res.StatusCode)
	}
	var body struct {
		Items []SignalAction `json:"items"`
	}
	if err := json.NewDecoder(res.Body).Decode(&body); err != nil {
		return nil, fmt.Errorf("bff: actions list decode: %w", err)
	}
	return body.Items, nil
}

// Contact implements ActionsClient.
func (c *HTTPActionsClient) Contact(ctx context.Context, signalID string) error {
	return c.putAction(ctx, signalID, "contact")
}

// Snooze implements ActionsClient.
func (c *HTTPActionsClient) Snooze(ctx context.Context, signalID string) error {
	return c.putAction(ctx, signalID, "snooze")
}

// Undo implements ActionsClient.
func (c *HTTPActionsClient) Undo(ctx context.Context, signalID string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodDelete, c.base+"/v1/actions/"+signalID, nil)
	if err != nil {
		return fmt.Errorf("bff: actions undo request: %w", err)
	}
	res, err := c.client.Do(req)
	if err != nil {
		return fmt.Errorf("bff: actions undo: %w", err)
	}
	defer res.Body.Close()
	_, _ = io.Copy(io.Discard, res.Body)
	if res.StatusCode != http.StatusNoContent && res.StatusCode != http.StatusOK {
		return fmt.Errorf("bff: actions undo: status %d", res.StatusCode)
	}
	return nil
}

func (c *HTTPActionsClient) putAction(ctx context.Context, signalID, action string) error {
	payload, err := json.Marshal(map[string]string{"action": action})
	if err != nil {
		return fmt.Errorf("bff: actions encode: %w", err)
	}
	req, err := http.NewRequestWithContext(
		ctx,
		http.MethodPut,
		c.base+"/v1/actions/"+signalID,
		bytes.NewReader(payload),
	)
	if err != nil {
		return fmt.Errorf("bff: actions put request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	res, err := c.client.Do(req)
	if err != nil {
		return fmt.Errorf("bff: actions put: %w", err)
	}
	defer res.Body.Close()
	_, _ = io.Copy(io.Discard, res.Body)
	if res.StatusCode != http.StatusNoContent && res.StatusCode != http.StatusOK {
		return fmt.Errorf("bff: actions put: status %d", res.StatusCode)
	}
	return nil
}

// OpenCase implements ActionsClient.
func (c *HTTPActionsClient) OpenCase(ctx context.Context, cmd CaseCommand) (string, error) {
	payload, err := json.Marshal(struct {
		CustomerID         string `json:"customer_id"`
		Label              string `json:"label,omitempty"`
		ChurnRisk          bool   `json:"churn_risk,omitempty"`
		Frustration        int    `json:"frustration,omitempty"`
		HumanRequested     bool   `json:"human_requested,omitempty"`
		RelevantWithdrawal bool   `json:"relevant_withdrawal,omitempty"`
	}{
		CustomerID:         cmd.CustomerID,
		Label:              cmd.Label,
		ChurnRisk:          cmd.ChurnRisk,
		Frustration:        cmd.Frustration,
		HumanRequested:     cmd.HumanRequested,
		RelevantWithdrawal: cmd.RelevantWithdrawal,
	})
	if err != nil {
		return "", fmt.Errorf("bff: case command encode: %w", err)
	}
	req, err := http.NewRequestWithContext(
		ctx,
		http.MethodPost,
		c.base+"/v1/signals/"+url.PathEscape(cmd.SignalID)+"/case",
		bytes.NewReader(payload),
	)
	if err != nil {
		return "", fmt.Errorf("bff: case command request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Idempotency-Key", cmd.IdempotencyKey)
	res, err := c.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("bff: case command: %w", err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusAccepted {
		_, _ = io.Copy(io.Discard, res.Body)
		return "", fmt.Errorf("bff: case command: status %d", res.StatusCode)
	}
	var body struct {
		RequestID string `json:"request_id"`
	}
	if err := json.NewDecoder(io.LimitReader(res.Body, 1<<16)).Decode(&body); err != nil {
		return "", fmt.Errorf("bff: case command decode: %w", err)
	}
	if body.RequestID == "" {
		return "", errors.New("bff: case command: advisory answered without a request id")
	}
	return body.RequestID, nil
}
