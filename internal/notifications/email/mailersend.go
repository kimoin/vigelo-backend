package email

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

type MailerSend struct {
	APIToken  string
	FromEmail string
	FromName  string
	Client    *http.Client
}

func (m *MailerSend) Send(ctx context.Context, msg Message) error {
	if m.Client == nil {
		m.Client = &http.Client{Timeout: 15 * time.Second}
	}
	body := map[string]any{
		"from": map[string]string{
			"email": m.FromEmail,
			"name":  m.FromName,
		},
		"to": []map[string]string{
			{"email": msg.To},
		},
		"subject": msg.Subject,
		"text":    msg.Text,
		"html":    msg.HTML,
	}
	if msg.HTML == "" {
		body["html"] = "<p>" + msg.Text + "</p>"
	}
	raw, err := json.Marshal(body)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://api.mailersend.com/v1/email", bytes.NewReader(raw))
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+m.APIToken)
	req.Header.Set("Content-Type", "application/json")
	res, err := m.Client.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if res.StatusCode >= 300 {
		return fmt.Errorf("mailersend: HTTP %d", res.StatusCode)
	}
	return nil
}
