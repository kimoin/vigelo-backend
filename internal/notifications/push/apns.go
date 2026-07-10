package push

import (
	"context"
	"fmt"
	"strings"
)

// APNs is the Apple Push Notification service sender. Wire
// github.com/sideshow/apns2 (or equivalent) when the native iOS app ships.
//
// Required env when PUSH_PROVIDER=apns:
//   - APNS_KEY_ID
//   - APNS_TEAM_ID
//   - APNS_KEY_PATH   (path to .p8 auth key)
//   - APNS_BUNDLE_ID
//   - APNS_SANDBOX    (true for development builds)
type APNs struct {
	KeyID    string
	TeamID   string
	KeyPath  string
	BundleID string
	Sandbox  bool
}

func (a *APNs) Configured() bool {
	return a != nil &&
		a.KeyID != "" &&
		a.TeamID != "" &&
		a.KeyPath != "" &&
		a.BundleID != ""
}

func (a *APNs) Send(ctx context.Context, msg Message) error {
	if a == nil || !a.Configured() {
		return fmt.Errorf("apns: not configured (set APNS_KEY_ID, APNS_TEAM_ID, APNS_KEY_PATH, APNS_BUNDLE_ID)")
	}
	if msg.Platform != "ios" {
		return fmt.Errorf("apns: unsupported platform %q", msg.Platform)
	}
	if strings.TrimSpace(msg.Token) == "" {
		return fmt.Errorf("apns: empty device token")
	}
	_ = ctx
	return fmt.Errorf("apns: delivery not wired yet; integrate apns2 when the native iOS app is ready")
}
