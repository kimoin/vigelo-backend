package domain

import "time"

type User struct {
	ID              string     `json:"id"`
	Email           string     `json:"email"`
	EmailVerifiedAt *time.Time `json:"email_verified_at,omitempty"`
	Phone           *string    `json:"phone,omitempty"`
	DisplayName     string     `json:"display_name"`
	Timezone        *string    `json:"timezone,omitempty"`
	PasswordHash    string     `json:"-"`
	CreatedAt       time.Time  `json:"created_at"`
}

type Household struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	Timezone  string    `json:"timezone"`
	Country   *string   `json:"country,omitempty"`
	OwnerID   string    `json:"-"`
	Role      string    `json:"role,omitempty"`
	CreatedAt time.Time `json:"created_at"`
}

type HouseholdMember struct {
	UserID      string    `json:"user_id"`
	Email       string    `json:"email"`
	DisplayName string    `json:"display_name"`
	Role        string    `json:"role"`
	CreatedAt   time.Time `json:"created_at"`
}

type SessionTokens struct {
	AccessToken  string
	RefreshToken string
	SessionID    string
}

type MonitoredWindow struct {
	StartTime string `json:"start_time"`
	EndTime   string `json:"end_time"`
}

type Subscription struct {
	Status           string     `json:"status"`
	ServiceStatus    string     `json:"service_status"`
	PlanCode         string     `json:"plan_code"`
	CurrentPeriodEnd *time.Time `json:"current_period_end,omitempty"`
	NextAction       *string    `json:"next_action"`
}

type DeviceBinding struct {
	ID                            string            `json:"id"`
	HouseholdID                   string            `json:"household_id"`
	DeviceID                      string            `json:"device_id,omitempty"`
	DisplayName                   string            `json:"display_name"`
	RoomOrLocationLabel           string            `json:"room_or_location_label"`
	Status                        string            `json:"status"`
	LastSeenAt                    *time.Time        `json:"last_seen_at"`
	BatteryVoltageV               *float64          `json:"battery_voltage_v"`
	BatteryStatus                 string            `json:"battery_status"`
	SubscriptionStatus            string            `json:"subscription_status"`
	MonitoredWindows              []MonitoredWindow `json:"monitored_windows"`
	MonitoredWindowsDeliveryState string            `json:"monitored_windows_delivery_state"`
	MonitoredWindowAlertMode      string            `json:"monitored_window_alert_mode,omitempty"`
	ActiveAlertCount              int               `json:"active_alert_count"`
	Subscription                  Subscription      `json:"subscription"`
	CreatedAt                     time.Time         `json:"created_at"`
	UpdatedAt                     time.Time         `json:"updated_at"`
}

type Alert struct {
	ID              string     `json:"id"`
	DeviceBindingID string     `json:"device_binding_id"`
	Type            string     `json:"type"`
	Severity        string     `json:"severity"`
	Status          string     `json:"status"`
	Title           string     `json:"title"`
	Body            string     `json:"body"`
	FirstSeenAt     time.Time  `json:"first_seen_at"`
	LastSeenAt      time.Time  `json:"last_seen_at"`
	AcknowledgedAt  *time.Time `json:"acknowledged_at,omitempty"`
}

type PushToken struct {
	ID          string    `json:"id"`
	UserID      string    `json:"-"`
	Platform    string    `json:"platform"`
	Environment string    `json:"environment,omitempty"`
	TokenHint   string    `json:"token_hint"`
	CreatedAt   time.Time `json:"created_at"`
}

type HouseholdInvite struct {
	ID          string     `json:"id"`
	HouseholdID string     `json:"household_id"`
	Email       string     `json:"email"`
	Role        string     `json:"role"`
	ExpiresAt   time.Time  `json:"expires_at"`
	AcceptedAt  *time.Time `json:"accepted_at,omitempty"`
	CreatedAt   time.Time  `json:"created_at"`
}
