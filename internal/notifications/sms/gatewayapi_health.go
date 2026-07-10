package sms

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

func (g *GatewayAPI) Health(ctx context.Context) error {
	if strings.TrimSpace(g.Token) == "" {
		return fmt.Errorf("API token not set")
	}
	client := g.HTTP
	if client == nil {
		client = &http.Client{Timeout: 10 * time.Second}
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://gatewayapi.com/rest/me", nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Token "+g.Token)
	req.Header.Set("Accept", "application/json")
	res, err := client.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(res.Body, 4096))
	switch res.StatusCode {
	case http.StatusOK:
		return nil
	case http.StatusUnauthorized:
		return fmt.Errorf("invalid API token")
	case http.StatusForbidden:
		return fmt.Errorf("API token rejected (check IP allowlist)")
	default:
		msg := strings.TrimSpace(string(body))
		if msg != "" {
			return fmt.Errorf("gatewayapi HTTP %d: %s", res.StatusCode, msg)
		}
		return fmt.Errorf("gatewayapi HTTP %d", res.StatusCode)
	}
}
