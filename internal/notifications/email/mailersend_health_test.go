package email

import (
	"strings"
	"testing"
)

func TestMailerSendHealth_verifiedDomain(t *testing.T) {
	if err := checkMailerSendDomain([]byte(`{"data":[{"name":"vigelo.fi","is_verified":true,"domain_settings":{"send_paused":false}}]}`), "vigelo.fi"); err != nil {
		t.Fatalf("expected ok, got %v", err)
	}
}

func TestMailerSendHealth_unverifiedDomain(t *testing.T) {
	err := checkMailerSendDomain([]byte(`{"data":[{"name":"vigelo.fi","is_verified":false,"domain_settings":{"send_paused":false}}]}`), "vigelo.fi")
	if err == nil || !strings.Contains(err.Error(), "not verified") {
		t.Fatalf("expected not verified error, got %v", err)
	}
}

func TestMailerSendHealth_identityFallback(t *testing.T) {
	err := checkMailerSendIdentity([]byte(`{"data":{"email":"notify@vigelo.fi","is_verified":true}}`), "notify@vigelo.fi")
	if err != nil {
		t.Fatalf("expected ok, got %v", err)
	}
}

func TestMailerSendHealth_domainMissing(t *testing.T) {
	err := checkMailerSendDomain([]byte(`{"data":[{"name":"other.com","is_verified":true,"domain_settings":{"send_paused":false}}]}`), "vigelo.fi")
	if !isNotFoundErr(err) {
		t.Fatalf("expected not found, got %v", err)
	}
}
