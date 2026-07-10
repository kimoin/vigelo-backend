package email

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
)

func (m *MailerSend) Health(ctx context.Context) error {
	if strings.TrimSpace(m.APIToken) == "" {
		return fmt.Errorf("API token not set")
	}
	from := strings.ToLower(strings.TrimSpace(m.FromEmail))
	if from == "" || !strings.Contains(from, "@") {
		return fmt.Errorf("MAILERSEND_FROM_EMAIL is not set")
	}
	fromDomain := strings.ToLower(strings.TrimSpace(strings.Split(from, "@")[1]))

	body, code, err := m.apiGET(ctx, "/v1/domains?limit=25")
	if err != nil {
		return err
	}
	switch code {
	case http.StatusUnauthorized:
		return fmt.Errorf("invalid or paused API token")
	case http.StatusOK:
		if err := checkMailerSendDomain(body, fromDomain); err == nil {
			return nil
		} else if !isNotFoundErr(err) {
			return err
		}
	case http.StatusForbidden:
		// Token may lack domains_read; fall through to sender identity check.
	default:
		if msg := mailerSendAPIError(body); msg != "" {
			return fmt.Errorf("mailersend API: %s", msg)
		}
		return fmt.Errorf("mailersend API returned HTTP %d", code)
	}

	idBody, idCode, err := m.apiGET(ctx, "/v1/identities/email/"+url.PathEscape(from))
	if err != nil {
		return err
	}
	switch idCode {
	case http.StatusOK:
		return checkMailerSendIdentity(idBody, from)
	case http.StatusNotFound:
		return fmt.Errorf("from address %s is not verified (domain %s not found or not verified)", from, fromDomain)
	case http.StatusUnauthorized:
		return fmt.Errorf("invalid or paused API token")
	default:
		if msg := mailerSendAPIError(idBody); msg != "" {
			return fmt.Errorf("mailersend API: %s", msg)
		}
		return fmt.Errorf("from address %s is not verified in MailerSend", from)
	}
}

func (m *MailerSend) apiGET(ctx context.Context, path string) ([]byte, int, error) {
	client := m.Client
	if client == nil {
		client = http.DefaultClient
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://api.mailersend.com"+path, nil)
	if err != nil {
		return nil, 0, err
	}
	req.Header.Set("Authorization", "Bearer "+m.APIToken)
	req.Header.Set("Accept", "application/json")
	res, err := client.Do(req)
	if err != nil {
		return nil, 0, err
	}
	defer res.Body.Close()
	body, err := io.ReadAll(io.LimitReader(res.Body, 64*1024))
	if err != nil {
		return nil, res.StatusCode, err
	}
	return body, res.StatusCode, nil
}

type mailerSendDomainsResp struct {
	Data []struct {
		Name       string `json:"name"`
		IsVerified bool   `json:"is_verified"`
		Settings   struct {
			SendPaused bool `json:"send_paused"`
		} `json:"domain_settings"`
	} `json:"data"`
}

func checkMailerSendDomain(body []byte, fromDomain string) error {
	var resp mailerSendDomainsResp
	if err := json.Unmarshal(body, &resp); err != nil {
		return fmt.Errorf("parse domains response: %w", err)
	}
	for _, d := range resp.Data {
		if strings.EqualFold(strings.TrimSpace(d.Name), fromDomain) {
			if d.Settings.SendPaused {
				return fmt.Errorf("domain %s sending is paused in MailerSend", fromDomain)
			}
			if !d.IsVerified {
				return fmt.Errorf("domain %s is not verified in MailerSend", fromDomain)
			}
			return nil
		}
	}
	return notFoundError{msg: fmt.Sprintf("domain %s not found in MailerSend account", fromDomain)}
}

type mailerSendIdentityResp struct {
	Data struct {
		Email      string `json:"email"`
		IsVerified bool   `json:"is_verified"`
	} `json:"data"`
}

func checkMailerSendIdentity(body []byte, from string) error {
	var resp mailerSendIdentityResp
	if err := json.Unmarshal(body, &resp); err != nil {
		return fmt.Errorf("parse sender identity response: %w", err)
	}
	if !resp.Data.IsVerified {
		return fmt.Errorf("sender identity %s is not verified in MailerSend", from)
	}
	return nil
}

type notFoundError struct{ msg string }

func (e notFoundError) Error() string { return e.msg }

func isNotFoundErr(err error) bool {
	_, ok := err.(notFoundError)
	return ok
}

func mailerSendAPIError(body []byte) string {
	var wrap struct {
		Message string `json:"message"`
		Errors  struct {
			Message string `json:"message"`
		} `json:"errors"`
	}
	if err := json.Unmarshal(body, &wrap); err != nil {
		return ""
	}
	if wrap.Message != "" {
		return wrap.Message
	}
	return wrap.Errors.Message
}
