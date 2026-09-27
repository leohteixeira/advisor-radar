package bff

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
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

// ActionsClient reaches advisory over HTTP for Contatado / Adiar / Desfazer.
type ActionsClient interface {
	List(ctx context.Context) ([]SignalAction, error)
	Contact(ctx context.Context, signalID string) error
	Snooze(ctx context.Context, signalID string) error
	Undo(ctx context.Context, signalID string) error
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
