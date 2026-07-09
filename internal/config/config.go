package config

import (
	"os"
	"strings"
	"time"
)

type Config struct {
	Addr            string
	DatabaseURL     string
	CORSOrigins     []string
	PublicURL       string
	FrontendBaseURL string
	LogLevel        string
	OfflineHours    int
	TrialDays       int

	VNMSBaseURL   string
	VNMSHTTPToken string
	VNMSTLSCAFile string

	MailerSendAPIToken  string
	MailerSendFromEmail string
	MailerSendFromName  string

	GatewayAPIToken  string
	GatewayAPISender string

	AccessTokenTTL  time.Duration
	RefreshTokenTTL time.Duration
	InviteTTL       time.Duration
	VerifyEmailTTL  time.Duration
	ResetPasswordTTL time.Duration
}

func Load() Config {
	return Config{
		Addr:            env("VSRV_ADDR", "127.0.0.1:8090"),
		DatabaseURL:     env("VSRV_DATABASE_URL", ""),
		PublicURL:       env("VSRV_PUBLIC_URL", ""),
		FrontendBaseURL: env("FRONTEND_BASE_URL", "http://127.0.0.1:5173"),
		LogLevel:        env("VSRV_LOG_LEVEL", "info"),
		OfflineHours:    envInt("OFFLINE_THRESHOLD_HOURS", 3),
		TrialDays:         envInt("DEFAULT_TRIAL_DAYS", 30),
		CORSOrigins:       splitCSV(env("VSRV_CORS_ORIGIN", "http://127.0.0.1:5173,http://localhost:5173")),

		VNMSBaseURL:   env("VNMS_BASE_URL", ""),
		VNMSHTTPToken: env("VNMS_HTTP_TOKEN", ""),
		VNMSTLSCAFile: env("VNMS_TLS_CA", ""),

		MailerSendAPIToken:  env("MAILERSEND_API_TOKEN", ""),
		MailerSendFromEmail: env("MAILERSEND_FROM_EMAIL", "notify@vigelo.fi"),
		MailerSendFromName:  env("MAILERSEND_FROM_NAME", "Vigelo"),

		GatewayAPIToken:  env("GATEWAYAPI_TOKEN", ""),
		GatewayAPISender: env("GATEWAYAPI_SENDER", "Vigelo"),

		AccessTokenTTL:   envDurationHours("VSRV_ACCESS_TOKEN_TTL_HOURS", 1),
		RefreshTokenTTL:  envDurationDays("VSRV_REFRESH_TOKEN_TTL_DAYS", 30),
		InviteTTL:        envDurationDays("VSRV_INVITE_TTL_DAYS", 7),
		VerifyEmailTTL:   envDurationHours("VSRV_VERIFY_EMAIL_TTL_HOURS", 48),
		ResetPasswordTTL: envDurationHours("VSRV_RESET_PASSWORD_TTL_HOURS", 2),
	}
}

func (c Config) SMSEnabled() bool {
	return c.GatewayAPIToken != ""
}

func (c Config) EmailEnabled() bool {
	return c.MailerSendAPIToken != ""
}

func env(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func envInt(key string, def int) int {
	v := os.Getenv(key)
	if v == "" {
		return def
	}
	var n int
	for _, c := range v {
		if c < '0' || c > '9' {
			return def
		}
		n = n*10 + int(c-'0')
	}
	if n == 0 && v != "0" {
		return def
	}
	return n
}

func envDurationHours(key string, hours int) time.Duration {
	if v := os.Getenv(key); v != "" {
		if n := envInt(key, hours); n > 0 {
			return time.Duration(n) * time.Hour
		}
	}
	return time.Duration(hours) * time.Hour
}

func envDurationDays(key string, days int) time.Duration {
	if v := os.Getenv(key); v != "" {
		if n := envInt(key, days); n > 0 {
			return time.Duration(n) * 24 * time.Hour
		}
	}
	return time.Duration(days) * 24 * time.Hour
}

func splitCSV(raw string) []string {
	var out []string
	for _, part := range strings.Split(raw, ",") {
		if part = strings.TrimSpace(part); part != "" {
			out = append(out, part)
		}
	}
	return out
}
