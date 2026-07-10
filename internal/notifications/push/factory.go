package push

import (
	"log/slog"
	"strings"

	"github.com/kimoin/vigelo-backend/internal/config"
)

// NewSender returns the configured push provider. Defaults to LogSender.
// When PUSH_PROVIDER=apns but APNS_* is incomplete, falls back to log with a warning.
func NewSender(cfg config.Config, log *slog.Logger) Sender {
	switch strings.ToLower(strings.TrimSpace(cfg.PushProvider)) {
	case "ntfy":
		if log != nil {
			log.Info("ntfy push enabled", "base_url", cfg.NtfyBaseURL)
		}
		return &Ntfy{
			BaseURL: cfg.NtfyBaseURL,
			Token:   cfg.NtfyToken,
		}
	case "apns":
		apns := &APNs{
			KeyID:    cfg.APNsKeyID,
			TeamID:   cfg.APNsTeamID,
			KeyPath:  cfg.APNsKeyPath,
			BundleID: cfg.APNsBundleID,
			Sandbox:  cfg.APNsSandbox,
		}
		if apns.Configured() {
			if log != nil {
				log.Info("apns push provider selected (wire apns2 client in native app phase)")
			}
			return apns
		}
		if log != nil {
			log.Warn("PUSH_PROVIDER=apns but APNS_* is incomplete; push will be logged only")
		}
		return &LogSender{Log: log}
	default:
		if log != nil && cfg.PushProvider != "" && cfg.PushProvider != "log" {
			log.Warn("unknown PUSH_PROVIDER; using log sender", "provider", cfg.PushProvider)
		}
		return &LogSender{Log: log}
	}
}
