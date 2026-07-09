package vnmsclient

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestVerifyEnrollmentAndBatchGet(t *testing.T) {
	const key = "000102030405060708090a0b0c0d0e0f"
	mux := http.NewServeMux()
	mux.HandleFunc("POST /v1/devices/dev-1/verify-enrollment", func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			DeviceKeyHex string `json:"device_key_hex"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.DeviceKeyHex != key {
			http.Error(w, "bad key", http.StatusForbidden)
			return
		}
		_ = json.NewEncoder(w).Encode(EnrollmentView{
			Verified:       true,
			LifecycleState: "disabled",
			Provisioned:    true,
		})
	})
	mux.HandleFunc("POST /v1/devices/dev-1/enable", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	mux.HandleFunc("POST /v1/devices:batchGet", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"devices": []DeviceState{{
				DeviceID:       "dev-1",
				LifecycleState: "active",
			}},
			"missing_device_ids": []string{},
		})
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	c, err := New(Config{BaseURL: srv.URL})
	if err != nil {
		t.Fatal(err)
	}
	view, err := c.VerifyEnrollment(context.Background(), "dev-1", key)
	if err != nil || !view.Verified || view.LifecycleState != "disabled" {
		t.Fatalf("verify: %+v err=%v", view, err)
	}
	if err := c.Enable(context.Background(), "dev-1"); err != nil {
		t.Fatal(err)
	}
	states, err := c.BatchGet(context.Background(), []string{"dev-1"})
	if err != nil || states["dev-1"].LifecycleState != "active" {
		t.Fatalf("batch: %+v err=%v", states, err)
	}
}
