package push

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestNtfySend(t *testing.T) {
	var gotMethod, gotPath, gotAuth string
	var body []byte
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		gotPath = r.URL.Path
		gotAuth = r.Header.Get("Authorization")
		body, _ = io.ReadAll(r.Body)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	n := &Ntfy{BaseURL: srv.URL, Token: "secret"}
	err := n.Send(context.Background(), Message{
		Platform: "ntfy",
		Token:    "vigelo-alerts-user1",
		Title:    "Device offline",
		Body:     "Kitchen has not checked in.",
		AlertType: "device_offline",
	})
	if err != nil {
		t.Fatal(err)
	}
	if gotMethod != http.MethodPost {
		t.Fatalf("method = %q", gotMethod)
	}
	if gotPath != "/vigelo-alerts-user1" {
		t.Fatalf("path = %q", gotPath)
	}
	if gotAuth != "Bearer secret" {
		t.Fatalf("auth = %q", gotAuth)
	}
	if !strings.Contains(string(body), "Device offline") {
		t.Fatalf("body = %s", body)
	}
}
