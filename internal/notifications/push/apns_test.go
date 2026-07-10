package push

import "testing"

func TestAPNsConfigured(t *testing.T) {
	if (&APNs{}).Configured() {
		t.Fatal("expected empty APNs to be unconfigured")
	}
	a := &APNs{KeyID: "k", TeamID: "t", KeyPath: "/p.p8", BundleID: "fi.vigelo.app"}
	if !a.Configured() {
		t.Fatal("expected full APNs config")
	}
}
