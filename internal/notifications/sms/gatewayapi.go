package sms

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

type GatewayAPI struct {
	Token  string
	Sender string
	HTTP   *http.Client
}

func (g *GatewayAPI) Send(ctx context.Context, msg Message) error {
	if g.Token == "" {
		return fmt.Errorf("gatewayapi token not configured")
	}
	sender := msg.Sender
	if sender == "" {
		sender = g.Sender
	}
	body, err := json.Marshal(map[string]any{
		"sender":  sender,
		"message": msg.Body,
		"recipients": []map[string]any{
			{"msisdn": normalizeMSISDN(msg.To)},
		},
	})
	if err != nil {
		return err
	}
	client := g.HTTP
	if client == nil {
		client = &http.Client{Timeout: 15 * time.Second}
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://gatewayapi.com/rest/mtsms", bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.SetBasicAuth(g.Token, "")
	req.Header.Set("Content-Type", "application/json")
	res, err := client.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if res.StatusCode >= 200 && res.StatusCode < 300 {
		return nil
	}
	payload, _ := io.ReadAll(io.LimitReader(res.Body, 4096))
	return fmt.Errorf("gatewayapi status %d: %s", res.StatusCode, strings.TrimSpace(string(payload)))
}

func normalizeMSISDN(phone string) string {
	phone = strings.TrimSpace(phone)
	phone = strings.TrimPrefix(phone, "+")
	return phone
}
