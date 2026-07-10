package adminstatus

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/kimoin/vigelo-backend/internal/config"
	"github.com/kimoin/vigelo-backend/internal/notifications/email"
	"github.com/kimoin/vigelo-backend/internal/notifications/push"
	"github.com/kimoin/vigelo-backend/internal/notifications/sms"
	"github.com/kimoin/vigelo-backend/internal/vnmsclient"
)

type healthChecker interface {
	Health(ctx context.Context) error
}

type Checker struct {
	Cfg    config.Config
	DB     interface{ Ping(ctx context.Context) error }
	VNMS   *vnmsclient.Client
	Mailer email.Sender
	SMS    sms.Sender
	Push   push.Sender
}

type ServiceStatus struct {
	Name       string `json:"name"`
	Status     string `json:"status"`
	Detail     string `json:"detail,omitempty"`
	Configured bool   `json:"configured"`
}

func (c *Checker) All(ctx context.Context) []ServiceStatus {
	checks := []func(context.Context) ServiceStatus{
		c.database,
		c.vnms,
		c.mailer,
		c.sms,
		func(ctx context.Context) ServiceStatus { return c.apns() },
		c.unifiedPush,
	}
	out := make([]ServiceStatus, len(checks))
	var wg sync.WaitGroup
	for i, check := range checks {
		wg.Add(1)
		go func(i int, check func(context.Context) ServiceStatus) {
			defer wg.Done()
			checkCtx, cancel := context.WithTimeout(ctx, 4*time.Second)
			defer cancel()
			out[i] = check(checkCtx)
		}(i, check)
	}
	wg.Wait()
	return out
}

func (c *Checker) database(ctx context.Context) ServiceStatus {
	st := ServiceStatus{Name: "Database", Configured: c.DB != nil}
	if c.DB == nil {
		st.Status = "unconfigured"
		st.Detail = "VSRV_DATABASE_URL not set"
		return st
	}
	if err := c.DB.Ping(ctx); err != nil {
		st.Status = "down"
		st.Detail = err.Error()
		return st
	}
	st.Status = "ok"
	return st
}

func (c *Checker) vnms(ctx context.Context) ServiceStatus {
	st := ServiceStatus{Name: "VNMS", Configured: c.Cfg.VNMSBaseURL != ""}
	if c.VNMS == nil {
		st.Status = "unconfigured"
		return st
	}
	if err := c.VNMS.Health(ctx); err != nil {
		st.Status = "down"
		st.Detail = healthErrDetail(err, c.Cfg.VNMSBaseURL)
		return st
	}
	st.Status = "ok"
	return st
}

func (c *Checker) mailer(ctx context.Context) ServiceStatus {
	st := ServiceStatus{Name: "MailerSend", Configured: c.Cfg.EmailEnabled()}
	if !st.Configured {
		st.Status = "log_only"
		st.Detail = "MAILERSEND_API_TOKEN not set"
		return st
	}
	if hc, ok := c.Mailer.(healthChecker); ok {
		if err := hc.Health(ctx); err != nil {
			st.Status = "fail"
			st.Detail = err.Error()
			return st
		}
		st.Status = "ok"
		st.Detail = c.Cfg.MailerSendFromEmail + " verified"
		return st
	}
	st.Status = "fail"
	st.Detail = "mailer does not support health checks"
	return st
}

func (c *Checker) sms(ctx context.Context) ServiceStatus {
	st := ServiceStatus{Name: "GatewayAPI", Configured: c.Cfg.SMSEnabled()}
	if !st.Configured {
		st.Status = "log_only"
		st.Detail = "GATEWAYAPI_TOKEN not set"
		return st
	}
	if hc, ok := c.SMS.(healthChecker); ok {
		if err := hc.Health(ctx); err != nil {
			st.Status = "fail"
			st.Detail = err.Error()
			return st
		}
		st.Status = "ok"
		return st
	}
	st.Status = "fail"
	st.Detail = "sms sender does not support health checks"
	return st
}

func healthErrDetail(err error, baseURL string) string {
	if err == nil {
		return ""
	}
	if errors.Is(err, context.DeadlineExceeded) || strings.Contains(err.Error(), "context deadline exceeded") {
		if baseURL != "" {
			return "unreachable at " + baseURL + " (timeout)"
		}
		return "health check timed out"
	}
	return err.Error()
}

func (c *Checker) apns() ServiceStatus {
	provider := strings.ToLower(c.Cfg.PushProvider)
	apns := &push.APNs{
		KeyID:    c.Cfg.APNsKeyID,
		TeamID:   c.Cfg.APNsTeamID,
		KeyPath:  c.Cfg.APNsKeyPath,
		BundleID: c.Cfg.APNsBundleID,
	}
	st := ServiceStatus{Name: "APNs", Configured: apns.Configured()}
	if provider == "apns" && apns.Configured() {
		st.Status = "stub"
		st.Detail = "credentials set; wire apns2 delivery when native app ships"
		return st
	}
	if provider == "apns" {
		st.Status = "misconfigured"
		st.Detail = "PUSH_PROVIDER=apns but APNS_* incomplete"
		return st
	}
	st.Status = "not_active"
	st.Detail = "set PUSH_PROVIDER=apns when iOS app is ready"
	return st
}

func (c *Checker) unifiedPush(ctx context.Context) ServiceStatus {
	st := ServiceStatus{Name: "UnifiedPush / ntfy", Configured: c.Cfg.PushEnabled()}
	switch strings.ToLower(c.Cfg.PushProvider) {
	case "ntfy":
		if hc, ok := c.Push.(healthChecker); ok {
			if err := hc.Health(ctx); err != nil {
				st.Status = "fail"
				st.Detail = err.Error()
				return st
			}
			st.Status = "ok"
			st.Detail = c.Cfg.NtfyBaseURL
			return st
		}
		st.Status = "fail"
		st.Detail = "ntfy sender does not support health checks"
	case "log", "":
		st.Status = "log_only"
		st.Detail = "PUSH_PROVIDER=log"
	default:
		st.Status = c.Cfg.PushProvider
	}
	return st
}

// noop for compile check
var _ = time.Second
var _ = http.StatusOK
