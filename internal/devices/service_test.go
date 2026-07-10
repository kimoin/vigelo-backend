package devices

import (
	"context"
	"errors"
	"testing"

	"github.com/kimoin/vigelo-backend/internal/vnmsclient"
)

type stubVNMS struct {
	verify            func(ctx context.Context, deviceID, secret string) (vnmsclient.EnrollmentView, error)
	provisionInventory func(ctx context.Context, deviceID, secret string) error
	enable            func(ctx context.Context, deviceID string) error
	batchGet          func(ctx context.Context, deviceIDs []string) (map[string]vnmsclient.DeviceState, error)
}

func (s stubVNMS) VerifyEnrollment(ctx context.Context, deviceID, secret string) (vnmsclient.EnrollmentView, error) {
	return s.verify(ctx, deviceID, secret)
}

func (s stubVNMS) ProvisionInventory(ctx context.Context, deviceID, secret string) error {
	if s.provisionInventory != nil {
		return s.provisionInventory(ctx, deviceID, secret)
	}
	return nil
}

func (s stubVNMS) Enable(ctx context.Context, deviceID string) error {
	if s.enable != nil {
		return s.enable(ctx, deviceID)
	}
	return nil
}

func (s stubVNMS) BatchGet(ctx context.Context, deviceIDs []string) (map[string]vnmsclient.DeviceState, error) {
	if s.batchGet != nil {
		return s.batchGet(ctx, deviceIDs)
	}
	return nil, nil
}

func (s stubVNMS) SetMonitoredWindows(ctx context.Context, deviceID string, windows []vnmsclient.Window) ([]vnmsclient.Window, error) {
	return windows, nil
}

func TestEnsureVNMSDeviceProvisionsNewDevice(t *testing.T) {
	const key = "000102030405060708090a0b0c0d0e0f"
	var provisioned bool
	svc := &Service{
		VNMS: stubVNMS{
			verify: func(_ context.Context, _, _ string) (vnmsclient.EnrollmentView, error) {
				return vnmsclient.EnrollmentView{}, vnmsclient.ErrNotFound
			},
			provisionInventory: func(_ context.Context, deviceID, secret string) error {
				if deviceID != "new-dev" || secret != key {
					t.Fatalf("provisionInventory(%q, %q)", deviceID, secret)
				}
				provisioned = true
				return nil
			},
		},
	}
	view, err := svc.ensureVNMSDevice(context.Background(), "new-dev", key)
	if err != nil {
		t.Fatal(err)
	}
	if !provisioned || !view.Provisioned || view.LifecycleState != "disabled" {
		t.Fatalf("view=%+v provisioned=%v", view, provisioned)
	}
}

func TestEnsureVNMSDeviceRejectsWrongKey(t *testing.T) {
	svc := &Service{
		VNMS: stubVNMS{
			verify: func(_ context.Context, _, _ string) (vnmsclient.EnrollmentView, error) {
				return vnmsclient.EnrollmentView{}, vnmsclient.ErrForbidden
			},
		},
	}
	_, err := svc.ensureVNMSDevice(context.Background(), "dev-1", "000102030405060708090a0b0c0d0e0f")
	if !errors.Is(err, ErrEnrollmentRejected) {
		t.Fatalf("err=%v", err)
	}
}

func TestEnsureVNMSDeviceRejectsActiveDevice(t *testing.T) {
	svc := &Service{
		VNMS: stubVNMS{
			verify: func(_ context.Context, _, _ string) (vnmsclient.EnrollmentView, error) {
				return vnmsclient.EnrollmentView{}, vnmsclient.ErrConflict
			},
		},
	}
	_, err := svc.ensureVNMSDevice(context.Background(), "dev-1", "000102030405060708090a0b0c0d0e0f")
	if !errors.Is(err, ErrDeviceAlreadyActive) {
		t.Fatalf("err=%v", err)
	}
}

func TestValidEnrollmentSecret(t *testing.T) {
	if !validEnrollmentSecret("000102030405060708090a0b0c0d0e0f") {
		t.Fatal("expected valid key")
	}
	if validEnrollmentSecret("badkey") {
		t.Fatal("expected invalid key")
	}
}
