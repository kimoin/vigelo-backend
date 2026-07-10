package push

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

// Ntfy delivers push via a self-hosted or public ntfy server. The registered
// push token is the topic name (or full topic URL path suffix).
type Ntfy struct {
	BaseURL string
	Token   string
	HTTP    *http.Client
}

func (n *Ntfy) Send(ctx context.Context, msg Message) error {
	base := strings.TrimRight(n.BaseURL, "/")
	if base == "" {
		return fmt.Errorf("ntfy: base URL not configured")
	}
	topic := strings.TrimPrefix(strings.TrimSpace(msg.Token), "/")
	if topic == "" {
		return fmt.Errorf("ntfy: empty topic")
	}
	payload, err := json.Marshal(map[string]any{
		"title":   msg.Title,
		"message": msg.Body,
		"tags":    []string{"vigelo", msg.AlertType},
		"priority": 4,
	})
	if err != nil {
		return err
	}
	client := n.HTTP
	if client == nil {
		client = &http.Client{Timeout: 15 * time.Second}
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, base+"/"+topic, bytes.NewReader(payload))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	if n.Token != "" {
		req.Header.Set("Authorization", "Bearer "+n.Token)
	}
	res, err := client.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if res.StatusCode >= 200 && res.StatusCode < 300 {
		return nil
	}
	body, _ := io.ReadAll(io.LimitReader(res.Body, 4096))
	return fmt.Errorf("ntfy status %d: %s", res.StatusCode, strings.TrimSpace(string(body)))
}

func (n *Ntfy) Health(ctx context.Context) error {
	base := strings.TrimRight(n.BaseURL, "/")
	if base == "" {
		return fmt.Errorf("ntfy base URL not configured")
	}
	client := n.HTTP
	if client == nil {
		client = &http.Client{Timeout: 10 * time.Second}
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, base+"/v1/health", nil)
	if err != nil {
		return err
	}
	if n.Token != "" {
		req.Header.Set("Authorization", "Bearer "+n.Token)
	}
	res, err := client.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if res.StatusCode >= 200 && res.StatusCode < 300 {
		return nil
	}
	body, _ := io.ReadAll(io.LimitReader(res.Body, 4096))
	return fmt.Errorf("ntfy health HTTP %d: %s", res.StatusCode, strings.TrimSpace(string(body)))
}
