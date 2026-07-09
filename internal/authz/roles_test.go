package authz

import "testing"

func TestCanClaimDevices(t *testing.T) {
	if !CanClaimDevices(RoleOwner) || !CanClaimDevices(RoleAdmin) {
		t.Fatal("owner/admin should claim")
	}
	if CanClaimDevices(RoleCaregiver) {
		t.Fatal("caregiver should not claim")
	}
}
