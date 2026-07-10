package vnmsclient

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"
)

var (
	ErrNotConfigured = errors.New("vnms client is not configured")
	ErrNotFound      = errors.New("vnms resource not found")
	ErrForbidden     = errors.New("vnms request forbidden")
	ErrConflict      = errors.New("vnms request conflict")
)

type Config struct {
	BaseURL   string
	Token     string
	TLSCAFile string
}

type Client struct {
	baseURL string
	token   string
	http    *http.Client
}

type EnrollmentView struct {
	Verified       bool   `json:"verified"`
	LifecycleState string `json:"lifecycle_state"`
	Provisioned    bool   `json:"provisioned"`
}

type Window struct {
	StartHour     int `json:"start_hour"`
	DurationHours int `json:"duration_hours"`
}

type DeviceState struct {
	DeviceID       string     `json:"device_id"`
	LifecycleState string     `json:"lifecycle_state"`
	LastContactAt  *time.Time `json:"last_contact_at"`
	LastVoltageMv  *int       `json:"last_voltage_mv"`
	MonitoredWindows []Window `json:"monitored_windows"`
}

type APIError struct {
	Status  int
	Code    string
	Message string
	Field   string
}

func (e *APIError) Error() string {
	if e.Code != "" {
		return fmt.Sprintf("vnms %s: %s", e.Code, e.Message)
	}
	return fmt.Sprintf("vnms http %d", e.Status)
}

func New(cfg Config) (*Client, error) {
	baseURL := strings.TrimRight(strings.TrimSpace(cfg.BaseURL), "/")
	if baseURL == "" {
		return nil, ErrNotConfigured
	}
	transport := http.DefaultTransport.(*http.Transport).Clone()
	if cfg.TLSCAFile != "" {
		pem, err := os.ReadFile(cfg.TLSCAFile)
		if err != nil {
			return nil, fmt.Errorf("read vnms tls ca: %w", err)
		}
		pool := x509.NewCertPool()
		if !pool.AppendCertsFromPEM(pem) {
			return nil, fmt.Errorf("parse vnms tls ca")
		}
		transport.TLSClientConfig = &tls.Config{
			RootCAs: pool,
		}
	}
	return &Client{
		baseURL: baseURL,
		token:   strings.TrimSpace(cfg.Token),
		http: &http.Client{
			Timeout:   15 * time.Second,
			Transport: transport,
		},
	}, nil
}

func (c *Client) VerifyEnrollment(ctx context.Context, deviceID, deviceKeyHex string) (EnrollmentView, error) {
	var out EnrollmentView
	err := c.post(ctx, fmt.Sprintf("/v1/devices/%s/verify-enrollment", deviceID), map[string]string{
		"device_key_hex": deviceKeyHex,
	}, &out)
	return out, err
}

func (c *Client) Enable(ctx context.Context, deviceID string) error {
	return c.post(ctx, fmt.Sprintf("/v1/devices/%s/enable", deviceID), nil, nil)
}

func (c *Client) Disable(ctx context.Context, deviceID string) error {
	return c.post(ctx, fmt.Sprintf("/v1/devices/%s/disable", deviceID), nil, nil)
}

func (c *Client) Unprovision(ctx context.Context, deviceID string) error {
	return c.post(ctx, fmt.Sprintf("/v1/devices/%s/unprovision", deviceID), nil, nil)
}

func (c *Client) Health(ctx context.Context) error {
	return c.do(ctx, http.MethodGet, "/healthz", nil, nil)
}

func (c *Client) BatchGet(ctx context.Context, deviceIDs []string) (map[string]DeviceState, error) {
	var resp struct {
		Devices          []DeviceState `json:"devices"`
		MissingDeviceIDs []string      `json:"missing_device_ids"`
	}
	if err := c.post(ctx, "/v1/devices:batchGet", map[string]any{
		"device_ids": deviceIDs,
	}, &resp); err != nil {
		return nil, err
	}
	out := make(map[string]DeviceState, len(resp.Devices))
	for _, d := range resp.Devices {
		out[d.DeviceID] = d
	}
	return out, nil
}

func (c *Client) SetMonitoredWindows(ctx context.Context, deviceID string, windows []Window) ([]Window, error) {
	var resp struct {
		MonitoredWindows []Window `json:"monitored_windows"`
	}
	err := c.do(ctx, http.MethodPut, fmt.Sprintf("/v1/devices/%s/monitored-windows", deviceID), map[string]any{
		"monitored_windows": windows,
	}, &resp)
	return resp.MonitoredWindows, err
}

type Event struct {
	EventID        int64           `json:"event_id"`
	EventType      string          `json:"event_type"`
	AggregateType  string          `json:"aggregate_type"`
	AggregateID    string          `json:"aggregate_id"`
	DeviceID       string          `json:"device_id,omitempty"`
	OccurredAt     time.Time       `json:"occurred_at"`
	IdempotencyKey string          `json:"idempotency_key"`
	Payload        json.RawMessage `json:"payload"`
}

type EventsView struct {
	Events     []Event `json:"events"`
	NextCursor int64   `json:"next_cursor"`
}

func (c *Client) ListEvents(ctx context.Context, after int64, limit int) (EventsView, error) {
	var out EventsView
	path := fmt.Sprintf("/v1/events?after=%d&limit=%d", after, limit)
	err := c.do(ctx, http.MethodGet, path, nil, &out)
	return out, err
}

func (c *Client) post(ctx context.Context, path string, body any, out any) error {
	return c.do(ctx, http.MethodPost, path, body, out)
}

func (c *Client) do(ctx context.Context, method, path string, body any, out any) error {
	if c == nil {
		return ErrNotConfigured
	}
	var r io.Reader = http.NoBody
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return err
		}
		r = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, r)
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	if c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}
	res, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	payload, _ := io.ReadAll(io.LimitReader(res.Body, 1<<20))
	if res.StatusCode >= 200 && res.StatusCode < 300 {
		if out != nil && len(payload) > 0 {
			if err := json.Unmarshal(payload, out); err != nil {
				return fmt.Errorf("decode vnms response: %w", err)
			}
		}
		return nil
	}
	var apiErr struct {
		Error struct {
			Code    string `json:"code"`
			Message string `json:"message"`
			Field   string `json:"field"`
		} `json:"error"`
	}
	_ = json.Unmarshal(payload, &apiErr)
	errOut := &APIError{
		Status:  res.StatusCode,
		Code:    apiErr.Error.Code,
		Message: apiErr.Error.Message,
		Field:   apiErr.Error.Field,
	}
	switch res.StatusCode {
	case http.StatusNotFound:
		return fmt.Errorf("%w: %s", ErrNotFound, errOut.Error())
	case http.StatusForbidden:
		return fmt.Errorf("%w: %s", ErrForbidden, errOut.Error())
	case http.StatusConflict:
		return fmt.Errorf("%w: %s", ErrConflict, errOut.Error())
	default:
		return errOut
	}
}
