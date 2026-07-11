package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/kimoin/vigelo-backend/internal/auth"
	"github.com/kimoin/vigelo-backend/internal/domain"
	"github.com/kimoin/vigelo-backend/internal/ids"
)

var (
	ErrUserNotFound     = errors.New("user not found")
	ErrProtectedUser    = errors.New("protected user cannot be deleted")
	ErrCannotDeleteSelf = errors.New("cannot delete your own account")
)

type AdminUserRow struct {
	ID                  string     `json:"id"`
	Email               string     `json:"email"`
	DisplayName         string     `json:"display_name"`
	DisabledAt          *time.Time `json:"disabled_at,omitempty"`
	LastLoginAt         *time.Time `json:"last_login_at,omitempty"`
	EmailVerified       bool       `json:"email_verified"`
	ConsoleAdmin        bool       `json:"console_admin"`
	Deletable           bool       `json:"deletable"`
	CreatedAt           time.Time  `json:"created_at"`
	Households          int        `json:"households"`
	Devices             int        `json:"devices"`
	TrialingDevices     int        `json:"trialing_devices"`
	ActiveDevices       int        `json:"active_devices"`
	SubscriptionSummary string     `json:"subscription_summary"`
	PushPlatforms       []string   `json:"push_platforms"`
}

func (s *Store) AdminSearchUsers(ctx context.Context, q, filter string, limit, offset int) ([]AdminUserRow, int, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	filter = strings.ToLower(strings.TrimSpace(filter))
	if filter == "" {
		filter = "email"
	}
	q = strings.TrimSpace(q)
	pattern := "%" + q + "%"

	var countSQL, listSQL string
	var args []any

	switch filter {
	case "device_id":
		countSQL = `
			SELECT COUNT(DISTINCT u.id) FROM users u
			JOIN household_members hm ON hm.user_id = u.id
			JOIN device_bindings b ON b.household_id = hm.household_id AND b.removed_at IS NULL
			WHERE ($1 = '' OR b.device_id ILIKE $2)`
		listSQL = `
			SELECT DISTINCT u.id, u.email, u.display_name, u.disabled_at, u.last_login_at,
			       u.email_verified_at IS NOT NULL, u.created_at
			FROM users u
			JOIN household_members hm ON hm.user_id = u.id
			JOIN device_bindings b ON b.household_id = hm.household_id AND b.removed_at IS NULL
			WHERE ($1 = '' OR b.device_id ILIKE $2)
			ORDER BY u.created_at DESC
			LIMIT $3 OFFSET $4`
		args = []any{q, pattern, limit, offset}
	case "expired_trial":
		countSQL = `
			SELECT COUNT(DISTINCT u.id) FROM users u
			JOIN household_members hm ON hm.user_id = u.id
			JOIN device_bindings b ON b.household_id = hm.household_id AND b.removed_at IS NULL
			JOIN subscriptions sub ON sub.device_binding_id = b.id
			WHERE sub.status = 'trialing' AND sub.trial_ends_at IS NOT NULL AND sub.trial_ends_at < now()
			  AND ($1 = '' OR u.email ILIKE $2 OR u.display_name ILIKE $2 OR b.device_id ILIKE $2)`
		listSQL = `
			SELECT DISTINCT u.id, u.email, u.display_name, u.disabled_at, u.last_login_at,
			       u.email_verified_at IS NOT NULL, u.created_at
			FROM users u
			JOIN household_members hm ON hm.user_id = u.id
			JOIN device_bindings b ON b.household_id = hm.household_id AND b.removed_at IS NULL
			JOIN subscriptions sub ON sub.device_binding_id = b.id
			WHERE sub.status = 'trialing' AND sub.trial_ends_at IS NOT NULL AND sub.trial_ends_at < now()
			  AND ($1 = '' OR u.email ILIKE $2 OR u.display_name ILIKE $2 OR b.device_id ILIKE $2)
			ORDER BY u.created_at DESC
			LIMIT $3 OFFSET $4`
		args = []any{q, pattern, limit, offset}
	case "failed_payment":
		countSQL = `
			SELECT COUNT(DISTINCT u.id) FROM users u
			JOIN household_members hm ON hm.user_id = u.id
			JOIN device_bindings b ON b.household_id = hm.household_id AND b.removed_at IS NULL
			JOIN subscriptions sub ON sub.device_binding_id = b.id
			WHERE (sub.status = 'past_due' OR sub.service_status = 'service_suspended')
			  AND ($1 = '' OR u.email ILIKE $2 OR u.display_name ILIKE $2 OR b.device_id ILIKE $2)`
		listSQL = `
			SELECT DISTINCT u.id, u.email, u.display_name, u.disabled_at, u.last_login_at,
			       u.email_verified_at IS NOT NULL, u.created_at
			FROM users u
			JOIN household_members hm ON hm.user_id = u.id
			JOIN device_bindings b ON b.household_id = hm.household_id AND b.removed_at IS NULL
			JOIN subscriptions sub ON sub.device_binding_id = b.id
			WHERE (sub.status = 'past_due' OR sub.service_status = 'service_suspended')
			  AND ($1 = '' OR u.email ILIKE $2 OR u.display_name ILIKE $2 OR b.device_id ILIKE $2)
			ORDER BY u.created_at DESC
			LIMIT $3 OFFSET $4`
		args = []any{q, pattern, limit, offset}
	default: // email
		if q != "" {
			countSQL = `
				SELECT COUNT(*) FROM users u
				WHERE u.email ILIKE $1 OR u.display_name ILIKE $1 OR u.id ILIKE $1`
			listSQL = `
				SELECT u.id, u.email, u.display_name, u.disabled_at, u.last_login_at,
				       u.email_verified_at IS NOT NULL, u.created_at
				FROM users u
				WHERE u.email ILIKE $1 OR u.display_name ILIKE $1 OR u.id ILIKE $1
				ORDER BY u.created_at DESC
				LIMIT $2 OFFSET $3`
			args = []any{pattern, limit, offset}
		} else {
			countSQL = `SELECT COUNT(*) FROM users`
			listSQL = `
				SELECT u.id, u.email, u.display_name, u.disabled_at, u.last_login_at,
				       u.email_verified_at IS NOT NULL, u.created_at
				FROM users u
				ORDER BY u.created_at DESC
				LIMIT $1 OFFSET $2`
			args = []any{limit, offset}
		}
	}

	var total int
	var rows pgx.Rows
	var err error
	if filter == "email" && q == "" {
		err = s.pool.QueryRow(ctx, countSQL).Scan(&total)
		if err != nil {
			return nil, 0, err
		}
		rows, err = s.pool.Query(ctx, listSQL, args...)
	} else if filter == "email" {
		err = s.pool.QueryRow(ctx, countSQL, args[0]).Scan(&total)
		if err != nil {
			return nil, 0, err
		}
		rows, err = s.pool.Query(ctx, listSQL, args...)
	} else {
		err = s.pool.QueryRow(ctx, countSQL, args[0], args[1]).Scan(&total)
		if err != nil {
			return nil, 0, err
		}
		rows, err = s.pool.Query(ctx, listSQL, args...)
	}
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	var out []AdminUserRow
	for rows.Next() {
		var row AdminUserRow
		var verifiedAt *time.Time
		if err := rows.Scan(&row.ID, &row.Email, &row.DisplayName, &row.DisabledAt, &row.LastLoginAt,
			&row.EmailVerified, &row.CreatedAt); err != nil {
			return nil, 0, err
		}
		_ = verifiedAt
		_ = s.pool.QueryRow(ctx, `SELECT COUNT(*) FROM household_members WHERE user_id = $1`, row.ID).Scan(&row.Households)
		_ = s.pool.QueryRow(ctx, `
			SELECT COUNT(*) FROM device_bindings b
			JOIN household_members hm ON hm.household_id = b.household_id
			WHERE hm.user_id = $1 AND b.removed_at IS NULL
		`, row.ID).Scan(&row.Devices)
		_ = s.pool.QueryRow(ctx, `
			SELECT
				COUNT(*) FILTER (WHERE sub.status = 'trialing'),
				COUNT(*) FILTER (WHERE sub.status = 'active')
			FROM device_bindings b
			JOIN household_members hm ON hm.household_id = b.household_id
			LEFT JOIN subscriptions sub ON sub.device_binding_id = b.id
			WHERE hm.user_id = $1 AND b.removed_at IS NULL
		`, row.ID).Scan(&row.TrialingDevices, &row.ActiveDevices)
		row.SubscriptionSummary = formatUserSubscriptionSummary(row.Devices, row.TrialingDevices, row.ActiveDevices)
		platformRows, _ := s.pool.Query(ctx, `
			SELECT DISTINCT platform FROM push_tokens WHERE user_id = $1 AND enabled = true
		`, row.ID)
		if platformRows != nil {
			for platformRows.Next() {
				var p string
				if platformRows.Scan(&p) == nil {
					row.PushPlatforms = append(row.PushPlatforms, p)
				}
			}
			platformRows.Close()
		}
		out = append(out, row)
	}
	return out, total, rows.Err()
}

func (s *Store) AdminGetUser(ctx context.Context, userID string) (AdminUserDetail, error) {
	var d AdminUserDetail
	err := s.pool.QueryRow(ctx, `
		SELECT id, email, display_name, disabled_at, last_login_at,
		       email_verified_at IS NOT NULL, phone, timezone, created_at
		FROM users WHERE id = $1
	`, userID).Scan(&d.ID, &d.Email, &d.DisplayName, &d.DisabledAt, &d.LastLoginAt,
		&d.EmailVerified, &d.Phone, &d.Timezone, &d.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return AdminUserDetail{}, ErrUserNotFound
	}
	if err != nil {
		return AdminUserDetail{}, err
	}
	hhRefs, err := s.ListUserHouseholds(ctx, userID)
	if err != nil {
		return AdminUserDetail{}, err
	}
	for _, ref := range hhRefs {
		hh, err := s.AdminGetHouseholdDetail(ctx, ref.HouseholdID, ref.Role)
		if err != nil {
			return AdminUserDetail{}, err
		}
		d.Households = append(d.Households, hh)
	}
	d.Sessions, _ = s.AdminListUserSessions(ctx, userID)
	d.PushTokens, _ = s.AdminListUserPushTokens(ctx, userID)
	d.Subscriptions, _ = s.AdminListUserSubscriptions(ctx, userID)
	return d, nil
}

func formatUserSubscriptionSummary(devices, trialing, active int) string {
	if devices == 0 {
		return "no devices"
	}
	parts := []string{}
	if trialing > 0 {
		parts = append(parts, fmt.Sprintf("%d trialing", trialing))
	}
	if active > 0 {
		parts = append(parts, fmt.Sprintf("%d paid", active))
	}
	if len(parts) == 0 {
		return fmt.Sprintf("%d device(s), no active subscription", devices)
	}
	return strings.Join(parts, ", ")
}

type AdminUserDetail struct {
	ID            string                 `json:"id"`
	Email         string                 `json:"email"`
	DisplayName   string                 `json:"display_name"`
	Phone         *string                `json:"phone,omitempty"`
	Timezone      *string                `json:"timezone,omitempty"`
	DisabledAt    *time.Time             `json:"disabled_at,omitempty"`
	LastLoginAt   *time.Time             `json:"last_login_at,omitempty"`
	EmailVerified bool                   `json:"email_verified"`
	ConsoleAdmin  bool                   `json:"console_admin"`
	Deletable     bool                   `json:"deletable"`
	CreatedAt     time.Time              `json:"created_at"`
	Households    []AdminHouseholdDetail `json:"households"`
	Subscriptions []AdminDeviceDetail    `json:"subscriptions"`
	Sessions      []AdminSessionRow      `json:"sessions"`
	PushTokens    []AdminPushTokenRow    `json:"push_tokens"`
}

type AdminHouseholdDetail struct {
	ID       string               `json:"id"`
	Name     string               `json:"name"`
	Timezone string               `json:"timezone"`
	UserRole string               `json:"user_role"`
	Members  []AdminMemberSummary `json:"members"`
	Devices  []AdminDeviceDetail  `json:"devices"`
}

type AdminMemberSummary struct {
	UserID      string     `json:"user_id,omitempty"`
	InviteID    string     `json:"invite_id,omitempty"`
	Email       string     `json:"email"`
	DisplayName string     `json:"display_name,omitempty"`
	Role        string     `json:"role"`
	Status      string     `json:"status"` // registered, invited, failed
	StatusAt    time.Time  `json:"status_at"`
	InvitedAt   *time.Time `json:"invited_at,omitempty"`
	ExpiresAt   *time.Time `json:"expires_at,omitempty"`
}

type AdminDeviceDetail struct {
	BindingID              string     `json:"binding_id"`
	DeviceID               string     `json:"device_id"`
	DisplayName            string     `json:"display_name"`
	RoomLabel              string     `json:"room_label"`
	HouseholdID            string     `json:"household_id,omitempty"`
	HouseholdName          string     `json:"household_name,omitempty"`
	SubscriptionStatus     string     `json:"subscription_status"`
	ServiceStatus          string     `json:"service_status"`
	PlanCode               string     `json:"plan_code"`
	TrialEndsAt            *time.Time `json:"trial_ends_at,omitempty"`
	CurrentPeriodStart     *time.Time `json:"current_period_start,omitempty"`
	CurrentPeriodEnd       *time.Time `json:"current_period_end,omitempty"`
	PaymentProvider        *string    `json:"payment_provider,omitempty"`
	ProviderSubscriptionID *string    `json:"provider_subscription_id,omitempty"`
	CancelledAt            *time.Time `json:"cancelled_at,omitempty"`
	PaymentSummary         string     `json:"payment_summary"`
	LastContactAt                 *time.Time `json:"last_contact_at,omitempty"`
	VNMSLifecycle                 *string    `json:"vnms_lifecycle,omitempty"`
	ClaimedByEmail                string     `json:"claimed_by_email,omitempty"`
	Removed                       bool       `json:"removed"`
	RemovedAt                     *time.Time `json:"removed_at,omitempty"`
	BatteryVoltageV               *float64   `json:"battery_voltage_v,omitempty"`
	BatteryStatus                 string     `json:"battery_status,omitempty"`
	DeviceStatus                  string     `json:"device_status,omitempty"`
	MonitoredWindowsSummary       string     `json:"monitored_windows_summary,omitempty"`
	MonitoredWindowAlertMode      string     `json:"monitored_window_alert_mode,omitempty"`
	MonitoredWindowsDeliveryState string     `json:"monitored_windows_delivery_state,omitempty"`
	MonitoredTimezone             string     `json:"monitored_timezone,omitempty"`
}

type AdminHouseholdMember struct {
	HouseholdID   string
	HouseholdName string
	Role          string
}

type AdminSessionRow struct {
	ID         string     `json:"id"`
	Platform   *string    `json:"platform,omitempty"`
	LastSeenAt *time.Time `json:"last_seen_at,omitempty"`
	CreatedAt  time.Time  `json:"created_at"`
	Revoked    bool       `json:"revoked"`
}

type AdminPushTokenRow struct {
	ID             string    `json:"id"`
	Platform       string    `json:"platform"`
	Environment    string    `json:"environment"`
	Enabled        bool      `json:"enabled"`
	LastRegistered time.Time `json:"last_registered_at"`
}

func (s *Store) AdminGetHouseholdDetail(ctx context.Context, householdID, userRole string) (AdminHouseholdDetail, error) {
	var hh AdminHouseholdDetail
	err := s.pool.QueryRow(ctx, `
		SELECT id, name, timezone FROM households WHERE id = $1 AND archived_at IS NULL
	`, householdID).Scan(&hh.ID, &hh.Name, &hh.Timezone)
	if errors.Is(err, pgx.ErrNoRows) {
		return AdminHouseholdDetail{}, auth.ErrForbidden
	}
	if err != nil {
		return AdminHouseholdDetail{}, err
	}
	hh.UserRole = userRole
	hh.Members, err = s.AdminListHouseholdMembers(ctx, householdID)
	if err != nil {
		return AdminHouseholdDetail{}, err
	}
	hh.Devices, err = s.AdminListHouseholdDevices(ctx, householdID)
	if err != nil {
		return AdminHouseholdDetail{}, err
	}
	return hh, nil
}

func (s *Store) AdminListHouseholdMembers(ctx context.Context, householdID string) ([]AdminMemberSummary, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT u.id, u.email, u.display_name, hm.role, hm.created_at
		FROM household_members hm
		JOIN users u ON u.id = hm.user_id
		WHERE hm.household_id = $1
		ORDER BY hm.role, u.email
	`, householdID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []AdminMemberSummary
	memberEmails := make(map[string]struct{})
	for rows.Next() {
		var m AdminMemberSummary
		var joinedAt time.Time
		if err := rows.Scan(&m.UserID, &m.Email, &m.DisplayName, &m.Role, &joinedAt); err != nil {
			return nil, err
		}
		m.Status = "registered"
		m.StatusAt = joinedAt
		memberEmails[strings.ToLower(m.Email)] = struct{}{}
		out = append(out, m)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	inviteRows, err := s.pool.Query(ctx, `
		SELECT id, email, role, created_at, expires_at
		FROM household_invites
		WHERE household_id = $1 AND accepted_at IS NULL
		ORDER BY created_at DESC
	`, householdID)
	if err != nil {
		return nil, err
	}
	defer inviteRows.Close()
	now := time.Now().UTC()
	for inviteRows.Next() {
		var inv AdminMemberSummary
		var invitedAt, expiresAt time.Time
		if err := inviteRows.Scan(&inv.InviteID, &inv.Email, &inv.Role, &invitedAt, &expiresAt); err != nil {
			return nil, err
		}
		if _, ok := memberEmails[strings.ToLower(inv.Email)]; ok {
			continue
		}
		inv.InvitedAt = &invitedAt
		inv.ExpiresAt = &expiresAt
		if expiresAt.Before(now) || expiresAt.Equal(now) {
			inv.Status = "failed"
			inv.StatusAt = expiresAt
		} else {
			inv.Status = "invited"
			inv.StatusAt = invitedAt
		}
		out = append(out, inv)
	}
	return out, inviteRows.Err()
}

func (s *Store) AdminListHouseholdDevices(ctx context.Context, householdID string) ([]AdminDeviceDetail, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT DISTINCT ON (b.id)
		       b.id, b.device_id, b.display_name, b.room_or_location_label,
		       COALESCE(sub.status, 'none'), COALESCE(sub.service_status, ''),
		       COALESCE(sub.plan_code, ''), sub.trial_ends_at, sub.current_period_start,
		       sub.current_period_end, sub.payment_provider, sub.provider_subscription_id,
		       sub.cancelled_at, b.last_contact_at, b.vnms_lifecycle_cache,
		       COALESCE(u.email, ''), b.removed_at IS NOT NULL, b.removed_at,
		       COALESCE(m.timezone, ''), COALESCE(m.windows_json::text, '[]'),
		       COALESCE(m.alert_mode, ''), COALESCE(m.delivery_state, '')
		FROM device_bindings b
		LEFT JOIN subscriptions sub ON sub.device_binding_id = b.id
		LEFT JOIN users u ON u.id = b.claimed_by_user_id
		LEFT JOIN monitored_window_intent m ON m.device_binding_id = b.id
		WHERE b.household_id = $1
		ORDER BY b.id, b.removed_at NULLS FIRST, b.created_at
	`, householdID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []AdminDeviceDetail
	for rows.Next() {
		var d AdminDeviceDetail
		var windowsJSON, tz, alertMode, delivery string
		if err := rows.Scan(
			&d.BindingID, &d.DeviceID, &d.DisplayName, &d.RoomLabel,
			&d.SubscriptionStatus, &d.ServiceStatus, &d.PlanCode,
			&d.TrialEndsAt, &d.CurrentPeriodStart, &d.CurrentPeriodEnd,
			&d.PaymentProvider, &d.ProviderSubscriptionID, &d.CancelledAt,
			&d.LastContactAt, &d.VNMSLifecycle, &d.ClaimedByEmail, &d.Removed, &d.RemovedAt,
			&tz, &windowsJSON, &alertMode, &delivery,
		); err != nil {
			return nil, err
		}
		d.MonitoredTimezone = tz
		d.MonitoredWindowAlertMode = alertMode
		d.MonitoredWindowsDeliveryState = delivery
		d.MonitoredWindowsSummary = summarizeWindowsJSON(windowsJSON)
		d.PaymentSummary = summarizePayment(d)
		out = append(out, d)
	}
	return out, rows.Err()
}

func (s *Store) AdminListUserSubscriptions(ctx context.Context, userID string) ([]AdminDeviceDetail, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT b.id, b.device_id, b.display_name, b.room_or_location_label,
		       h.id, h.name,
		       COALESCE(sub.status, 'none'), COALESCE(sub.service_status, ''),
		       COALESCE(sub.plan_code, ''), sub.trial_ends_at, sub.current_period_start,
		       sub.current_period_end, sub.payment_provider, sub.provider_subscription_id,
		       sub.cancelled_at, b.last_contact_at, b.vnms_lifecycle_cache,
		       COALESCE(u.email, ''), b.removed_at IS NOT NULL
		FROM device_bindings b
		JOIN households h ON h.id = b.household_id
		JOIN household_members hm ON hm.household_id = b.household_id AND hm.user_id = $1
		LEFT JOIN subscriptions sub ON sub.device_binding_id = b.id
		LEFT JOIN users u ON u.id = b.claimed_by_user_id
		WHERE b.removed_at IS NULL
		ORDER BY h.name, b.display_name
	`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []AdminDeviceDetail
	for rows.Next() {
		var d AdminDeviceDetail
		if err := rows.Scan(
			&d.BindingID, &d.DeviceID, &d.DisplayName, &d.RoomLabel,
			&d.HouseholdID, &d.HouseholdName,
			&d.SubscriptionStatus, &d.ServiceStatus, &d.PlanCode,
			&d.TrialEndsAt, &d.CurrentPeriodStart, &d.CurrentPeriodEnd,
			&d.PaymentProvider, &d.ProviderSubscriptionID, &d.CancelledAt,
			&d.LastContactAt, &d.VNMSLifecycle, &d.ClaimedByEmail, &d.Removed,
		); err != nil {
			return nil, err
		}
		d.PaymentSummary = summarizePayment(d)
		out = append(out, d)
	}
	return out, rows.Err()
}

func summarizeWindowsJSON(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" || raw == "[]" || raw == "null" {
		return "not configured"
	}
	var windows []struct {
		StartTime string `json:"start_time"`
		EndTime   string `json:"end_time"`
	}
	if err := json.Unmarshal([]byte(raw), &windows); err != nil || len(windows) == 0 {
		return "not configured"
	}
	parts := make([]string, 0, len(windows))
	for _, w := range windows {
		if w.StartTime != "" && w.EndTime != "" {
			parts = append(parts, w.StartTime+"–"+w.EndTime)
		}
	}
	if len(parts) == 0 {
		return "not configured"
	}
	return strings.Join(parts, ", ")
}

func summarizePayment(d AdminDeviceDetail) string {
	if d.CancelledAt != nil {
		return "cancelled"
	}
	switch d.SubscriptionStatus {
	case "trialing":
		if d.TrialEndsAt != nil {
			return "trialing until " + d.TrialEndsAt.Format("2006-01-02")
		}
		return "trialing"
	case "active":
		if d.CurrentPeriodEnd != nil {
			return "paid until " + d.CurrentPeriodEnd.Format("2006-01-02")
		}
		return "active (paid)"
	case "past_due":
		return "past due"
	case "none", "":
		return "no subscription"
	default:
		return d.SubscriptionStatus
	}
}

func (s *Store) ListUserHouseholds(ctx context.Context, userID string) ([]AdminHouseholdMember, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT h.id, h.name, hm.role
		FROM household_members hm
		JOIN households h ON h.id = hm.household_id
		WHERE hm.user_id = $1 AND h.archived_at IS NULL
		ORDER BY h.name
	`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []AdminHouseholdMember
	for rows.Next() {
		var m AdminHouseholdMember
		if err := rows.Scan(&m.HouseholdID, &m.HouseholdName, &m.Role); err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

func (s *Store) AdminListUserSessions(ctx context.Context, userID string) ([]AdminSessionRow, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id, platform, last_seen_at, created_at, revoked_at IS NOT NULL
		FROM sessions WHERE user_id = $1 ORDER BY created_at DESC LIMIT 20
	`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []AdminSessionRow
	for rows.Next() {
		var row AdminSessionRow
		if err := rows.Scan(&row.ID, &row.Platform, &row.LastSeenAt, &row.CreatedAt, &row.Revoked); err != nil {
			return nil, err
		}
		out = append(out, row)
	}
	return out, rows.Err()
}

func (s *Store) AdminListUserPushTokens(ctx context.Context, userID string) ([]AdminPushTokenRow, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id, platform, environment, enabled, last_registered_at
		FROM push_tokens WHERE user_id = $1 ORDER BY last_registered_at DESC
	`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []AdminPushTokenRow
	for rows.Next() {
		var row AdminPushTokenRow
		if err := rows.Scan(&row.ID, &row.Platform, &row.Environment, &row.Enabled, &row.LastRegistered); err != nil {
			return nil, err
		}
		out = append(out, row)
	}
	return out, rows.Err()
}

func (s *Store) AdminSetUserDisabled(ctx context.Context, userID string, disabled bool) error {
	if disabled {
		res, err := s.pool.Exec(ctx, `UPDATE users SET disabled_at = now() WHERE id = $1 AND disabled_at IS NULL`, userID)
		if err != nil {
			return err
		}
		if res.RowsAffected() == 0 {
			return ErrUserNotFound
		}
		return nil
	}
	res, err := s.pool.Exec(ctx, `UPDATE users SET disabled_at = NULL WHERE id = $1`, userID)
	if err != nil {
		return err
	}
	if res.RowsAffected() == 0 {
		return ErrUserNotFound
	}
	return nil
}

type AdminCreateUserResult struct {
	User      domain.User
	Household domain.Household
}

// AdminCreateUser registers a user account immediately (email verified, no invite email).
func (s *Store) AdminCreateUser(ctx context.Context, email, password, displayName, timezone string) (AdminCreateUserResult, error) {
	email = strings.ToLower(strings.TrimSpace(email))
	if email == "" {
		return AdminCreateUserResult{}, errors.New("email required")
	}
	hash, err := auth.HashPassword(password)
	if err != nil {
		return AdminCreateUserResult{}, err
	}
	if displayName == "" {
		displayName = email
	}
	if timezone == "" {
		timezone = "Europe/Helsinki"
	}
	userID := ids.New("user")
	householdID := ids.New("hh")
	now := time.Now().UTC()

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return AdminCreateUserResult{}, err
	}
	defer tx.Rollback(ctx)

	_, err = tx.Exec(ctx, `
		INSERT INTO users (id, email, display_name, password_hash, email_verified_at, timezone, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, $5)
	`, userID, email, displayName, hash, now, timezone)
	if pgUniqueViolation(err) {
		return AdminCreateUserResult{}, auth.ErrEmailTaken
	}
	if err != nil {
		return AdminCreateUserResult{}, err
	}

	_, err = tx.Exec(ctx, `
		INSERT INTO households (id, name, timezone, created_by_user_id, created_at)
		VALUES ($1, $2, $3, $4, $5)
	`, householdID, "Home", timezone, userID, now)
	if err != nil {
		return AdminCreateUserResult{}, err
	}

	_, err = tx.Exec(ctx, `
		INSERT INTO household_members (household_id, user_id, role, created_at)
		VALUES ($1, $2, 'owner', $3)
	`, householdID, userID, now)
	if err != nil {
		return AdminCreateUserResult{}, err
	}

	if err := tx.Commit(ctx); err != nil {
		return AdminCreateUserResult{}, err
	}

	return AdminCreateUserResult{
		User: domain.User{
			ID:              userID,
			Email:           email,
			DisplayName:     displayName,
			EmailVerifiedAt: &now,
			Timezone:        &timezone,
			CreatedAt:       now,
		},
		Household: domain.Household{
			ID:        householdID,
			Name:      "Home",
			Timezone:  timezone,
			OwnerID:   userID,
			Role:      "owner",
			CreatedAt: now,
		},
	}, nil
}

// AdminDeleteUser permanently removes a user account and cleans up owned households.
func (s *Store) AdminDeleteUser(ctx context.Context, userID string) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	var email string
	err = tx.QueryRow(ctx, `SELECT email FROM users WHERE id = $1`, userID).Scan(&email)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrUserNotFound
	}
	if err != nil {
		return err
	}

	if _, err := tx.Exec(ctx, `
		DELETE FROM household_invites WHERE invited_by = $1 OR lower(email) = lower($2)
	`, userID, email); err != nil {
		return err
	}

	if _, err := tx.Exec(ctx, `
		UPDATE device_bindings SET claimed_by_user_id = NULL WHERE claimed_by_user_id = $1
	`, userID); err != nil {
		return err
	}

	rows, err := tx.Query(ctx, `SELECT id FROM households WHERE created_by_user_id = $1`, userID)
	if err != nil {
		return err
	}
	var owned []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return err
		}
		owned = append(owned, id)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return err
	}

	for _, hhID := range owned {
		if _, err := tx.Exec(ctx, `
			UPDATE households SET archived_at = COALESCE(archived_at, now()) WHERE id = $1
		`, hhID); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `
			UPDATE device_bindings
			SET removed_at = now(), removed_reason = 'user_deleted', updated_at = now()
			WHERE household_id = $1 AND removed_at IS NULL
		`, hhID); err != nil {
			return err
		}

		var otherMembers int
		if err := tx.QueryRow(ctx, `
			SELECT COUNT(*) FROM household_members WHERE household_id = $1 AND user_id <> $2
		`, hhID, userID).Scan(&otherMembers); err != nil {
			return err
		}
		if otherMembers == 0 {
			if _, err := tx.Exec(ctx, `DELETE FROM households WHERE id = $1`, hhID); err != nil {
				return err
			}
			continue
		}

		var successor string
		err = tx.QueryRow(ctx, `
			SELECT user_id FROM household_members
			WHERE household_id = $1 AND user_id <> $2
			ORDER BY CASE role WHEN 'owner' THEN 0 WHEN 'admin' THEN 1 WHEN 'caregiver' THEN 2 ELSE 3 END, created_at
			LIMIT 1
		`, hhID, userID).Scan(&successor)
		if err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `
			UPDATE households SET created_by_user_id = $2 WHERE id = $1
		`, hhID, successor); err != nil {
			return err
		}
	}

	if _, err := tx.Exec(ctx, `
		UPDATE sessions SET revoked_at = now() WHERE user_id = $1 AND revoked_at IS NULL
	`, userID); err != nil {
		return err
	}

	tag, err := tx.Exec(ctx, `DELETE FROM users WHERE id = $1`, userID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrUserNotFound
	}
	return tx.Commit(ctx)
}

func (s *Store) AdminExtendTrial(ctx context.Context, bindingID string, days int) error {
	if days <= 0 {
		days = 30
	}
	var currentEnd *time.Time
	_ = s.pool.QueryRow(ctx, `
		SELECT trial_ends_at FROM subscriptions WHERE device_binding_id = $1
	`, bindingID).Scan(&currentEnd)
	base := time.Now().UTC()
	if currentEnd != nil && currentEnd.After(base) {
		base = *currentEnd
	}
	end := base.AddDate(0, 0, days)
	tag, err := s.pool.Exec(ctx, `
		UPDATE subscriptions
		SET status = 'trialing', service_status = 'service_active',
		    trial_ends_at = $2, updated_at = now()
		WHERE device_binding_id = $1
	`, bindingID, end)
	if err != nil {
		return err
	}
	if tag.RowsAffected() > 0 {
		return nil
	}
	var householdID string
	err = s.pool.QueryRow(ctx, `
		SELECT household_id FROM device_bindings WHERE id = $1 AND removed_at IS NULL
	`, bindingID).Scan(&householdID)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrBindingNotFound
	}
	if err != nil {
		return err
	}
	subID := ids.New("sub")
	_, err = s.pool.Exec(ctx, `
		INSERT INTO subscriptions (
			id, household_id, device_binding_id, status, service_status, plan_code,
			trial_ends_at, created_at, updated_at
		) VALUES ($1, $2, $3, 'trialing', 'service_active', 'device_monitoring_monthly', $4, now(), now())
	`, subID, householdID, bindingID, end)
	return err
}

func (s *Store) AdminActivateSubscription(ctx context.Context, bindingID string, months int) error {
	if months <= 0 {
		months = 1
	}
	end := time.Now().UTC().AddDate(0, months, 0)
	tag, err := s.pool.Exec(ctx, `
		UPDATE subscriptions
		SET status = 'active', service_status = 'service_active',
		    current_period_start = now(), current_period_end = $2,
		    trial_ends_at = NULL, updated_at = now()
		WHERE device_binding_id = $1
	`, bindingID, end)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrBindingNotFound
	}
	return nil
}

var ErrMemberNotFound = errors.New("household member not found")
var ErrCannotRemoveOwner = errors.New("cannot remove sole household owner")
var ErrAlreadyMember = errors.New("user is already a household member")

func (s *Store) GetUserIDByEmail(ctx context.Context, email string) (string, error) {
	email = strings.ToLower(strings.TrimSpace(email))
	var id string
	err := s.pool.QueryRow(ctx, `SELECT id FROM users WHERE email = $1`, email).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", ErrUserNotFound
	}
	return id, err
}

func (s *Store) GetHouseholdOwnerID(ctx context.Context, householdID string) (string, error) {
	var id string
	err := s.pool.QueryRow(ctx, `
		SELECT user_id FROM household_members
		WHERE household_id = $1 AND role = 'owner'
		LIMIT 1
	`, householdID).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", auth.ErrForbidden
	}
	return id, err
}

func (s *Store) AdminAddHouseholdMember(ctx context.Context, householdID, email, role string) error {
	role = strings.ToLower(strings.TrimSpace(role))
	if role == "" {
		role = "caregiver"
	}
	if role == "owner" {
		return errors.New("cannot add owner role directly")
	}
	userID, err := s.GetUserIDByEmail(ctx, email)
	if err != nil {
		return err
	}
	var exists bool
	if err := s.pool.QueryRow(ctx, `
		SELECT EXISTS(SELECT 1 FROM household_members WHERE household_id = $1 AND user_id = $2)
	`, householdID, userID).Scan(&exists); err != nil {
		return err
	}
	if exists {
		return ErrAlreadyMember
	}
	_, err = s.pool.Exec(ctx, `
		INSERT INTO household_members (household_id, user_id, role, created_at)
		VALUES ($1, $2, $3, now())
	`, householdID, userID, role)
	return err
}

func (s *Store) AdminInviteHouseholdMember(ctx context.Context, householdID, invitedBy, email, role string, ttl time.Duration) (InviteResult, error) {
	role = strings.ToLower(strings.TrimSpace(role))
	if role == "" {
		role = "caregiver"
	}
	if role == "owner" {
		return InviteResult{}, errors.New("cannot invite as owner")
	}
	email = strings.ToLower(strings.TrimSpace(email))
	inviteID := ids.New("inv")
	raw, err := auth.NewToken()
	if err != nil {
		return InviteResult{}, err
	}
	now := time.Now().UTC()
	expires := now.Add(ttl)
	_, err = s.pool.Exec(ctx, `
		INSERT INTO household_invites (id, household_id, email, role, token_hash, invited_by, expires_at, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
	`, inviteID, householdID, email, role, auth.HashToken(raw), invitedBy, expires, now)
	if err != nil {
		return InviteResult{}, err
	}
	return InviteResult{
		TokenRaw: raw,
		Invite: domain.HouseholdInvite{
			ID: inviteID, HouseholdID: householdID, Email: email, Role: role, ExpiresAt: expires, CreatedAt: now,
		},
	}, nil
}

func (s *Store) AdminRemoveHouseholdMember(ctx context.Context, householdID, userID string) error {
	var role string
	err := s.pool.QueryRow(ctx, `
		SELECT role FROM household_members WHERE household_id = $1 AND user_id = $2
	`, householdID, userID).Scan(&role)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrMemberNotFound
	}
	if err != nil {
		return err
	}
	if role == "owner" {
		var owners int
		_ = s.pool.QueryRow(ctx, `
			SELECT COUNT(*) FROM household_members WHERE household_id = $1 AND role = 'owner'
		`, householdID).Scan(&owners)
		if owners <= 1 {
			return ErrCannotRemoveOwner
		}
	}
	tag, err := s.pool.Exec(ctx, `
		DELETE FROM household_members WHERE household_id = $1 AND user_id = $2
	`, householdID, userID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrMemberNotFound
	}
	return nil
}

func (s *Store) AdminMoveDevice(ctx context.Context, bindingID, householdID string) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	tag, err := tx.Exec(ctx, `
		UPDATE device_bindings SET household_id = $2, updated_at = now()
		WHERE id = $1 AND removed_at IS NULL
	`, bindingID, householdID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrBindingNotFound
	}
	_, err = tx.Exec(ctx, `
		UPDATE subscriptions SET household_id = $2, updated_at = now()
		WHERE device_binding_id = $1
	`, bindingID, householdID)
	if err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (s *Store) AdminDeleteHousehold(ctx context.Context, householdID string) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	_, err = tx.Exec(ctx, `UPDATE households SET archived_at = now() WHERE id = $1 AND archived_at IS NULL`, householdID)
	if err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `
		UPDATE device_bindings SET removed_at = now(), removed_reason = 'household_deleted', updated_at = now()
		WHERE household_id = $1 AND removed_at IS NULL
	`, householdID)
	if err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (s *Store) AdminCreateHousehold(ctx context.Context, name, ownerUserID, timezone string) (domain.Household, error) {
	if timezone == "" {
		timezone = "Europe/Helsinki"
	}
	hhID := ids.New("hh")
	now := time.Now().UTC()
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return domain.Household{}, err
	}
	defer tx.Rollback(ctx)
	_, err = tx.Exec(ctx, `
		INSERT INTO households (id, name, timezone, created_by_user_id, created_at)
		VALUES ($1, $2, $3, $4, $5)
	`, hhID, name, timezone, ownerUserID, now)
	if err != nil {
		return domain.Household{}, err
	}
	_, err = tx.Exec(ctx, `
		INSERT INTO household_members (household_id, user_id, role, created_at)
		VALUES ($1, $2, 'owner', $3)
	`, hhID, ownerUserID, now)
	if err != nil {
		return domain.Household{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return domain.Household{}, err
	}
	return domain.Household{ID: hhID, Name: name, Timezone: timezone, CreatedAt: now}, nil
}

type AdminHouseholdRow struct {
	ID        string
	Name      string
	Timezone  string
	Members   int
	Devices   int
	CreatedAt time.Time
}

func (s *Store) AdminListHouseholds(ctx context.Context, q string, limit, offset int) ([]AdminHouseholdRow, int, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	pattern := "%" + q + "%"
	var total int
	var rows pgx.Rows
	var err error
	if q != "" {
		err = s.pool.QueryRow(ctx, `
			SELECT COUNT(*) FROM households WHERE archived_at IS NULL AND (name ILIKE $1 OR id ILIKE $1)
		`, pattern).Scan(&total)
		if err != nil {
			return nil, 0, err
		}
		rows, err = s.pool.Query(ctx, `
			SELECT h.id, h.name, h.timezone, h.created_at,
			       (SELECT COUNT(*) FROM household_members hm WHERE hm.household_id = h.id),
			       (SELECT COUNT(*) FROM device_bindings b WHERE b.household_id = h.id AND b.removed_at IS NULL)
			FROM households h
			WHERE h.archived_at IS NULL AND (h.name ILIKE $1 OR h.id ILIKE $1)
			ORDER BY h.created_at DESC
			LIMIT $2 OFFSET $3
		`, pattern, limit, offset)
	} else {
		err = s.pool.QueryRow(ctx, `SELECT COUNT(*) FROM households WHERE archived_at IS NULL`).Scan(&total)
		if err != nil {
			return nil, 0, err
		}
		rows, err = s.pool.Query(ctx, `
			SELECT h.id, h.name, h.timezone, h.created_at,
			       (SELECT COUNT(*) FROM household_members hm WHERE hm.household_id = h.id),
			       (SELECT COUNT(*) FROM device_bindings b WHERE b.household_id = h.id AND b.removed_at IS NULL)
			FROM households h
			WHERE h.archived_at IS NULL
			ORDER BY h.created_at DESC
			LIMIT $1 OFFSET $2
		`, limit, offset)
	}
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	var out []AdminHouseholdRow
	for rows.Next() {
		var row AdminHouseholdRow
		if err := rows.Scan(&row.ID, &row.Name, &row.Timezone, &row.CreatedAt, &row.Members, &row.Devices); err != nil {
			return nil, 0, err
		}
		out = append(out, row)
	}
	return out, total, rows.Err()
}

type AdminDeviceRow struct {
	BindingID      string
	DeviceID       string
	DisplayName    string
	HouseholdID    string
	HouseholdName  string
	OwnerEmail     string
	Subscription   string
	ServiceStatus  string
	TrialEndsAt    *time.Time
	LastContactAt  *time.Time
	VNMSLifecycle  *string
	Removed        bool
}

func (s *Store) AdminListDevices(ctx context.Context, q string, limit, offset int) ([]AdminDeviceRow, int, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	pattern := "%" + strings.TrimSpace(q) + "%"
	var total int
	var rows pgx.Rows
	var err error
	if strings.TrimSpace(q) != "" {
		err = s.pool.QueryRow(ctx, `
			SELECT COUNT(*) FROM device_bindings b
			JOIN households h ON h.id = b.household_id
			WHERE b.device_id ILIKE $1 OR b.display_name ILIKE $1 OR b.id ILIKE $1 OR h.name ILIKE $1
		`, pattern).Scan(&total)
		if err != nil {
			return nil, 0, err
		}
		rows, err = s.pool.Query(ctx, `
			SELECT b.id, b.device_id, b.display_name, b.household_id, h.name,
			       COALESCE(u.email, ''), COALESCE(sub.status, 'none'), COALESCE(sub.service_status, ''),
			       sub.trial_ends_at, b.last_contact_at, b.vnms_lifecycle_cache,
			       b.removed_at IS NOT NULL
			FROM device_bindings b
			JOIN households h ON h.id = b.household_id
			LEFT JOIN users u ON u.id = b.claimed_by_user_id
			LEFT JOIN subscriptions sub ON sub.device_binding_id = b.id
			WHERE b.device_id ILIKE $1 OR b.display_name ILIKE $1 OR b.id ILIKE $1 OR h.name ILIKE $1
			ORDER BY b.created_at DESC
			LIMIT $2 OFFSET $3
		`, pattern, limit, offset)
	} else {
		err = s.pool.QueryRow(ctx, `SELECT COUNT(*) FROM device_bindings`).Scan(&total)
		if err != nil {
			return nil, 0, err
		}
		rows, err = s.pool.Query(ctx, `
			SELECT b.id, b.device_id, b.display_name, b.household_id, h.name,
			       COALESCE(u.email, ''), COALESCE(sub.status, 'none'), COALESCE(sub.service_status, ''),
			       sub.trial_ends_at, b.last_contact_at, b.vnms_lifecycle_cache,
			       b.removed_at IS NOT NULL
			FROM device_bindings b
			JOIN households h ON h.id = b.household_id
			LEFT JOIN users u ON u.id = b.claimed_by_user_id
			LEFT JOIN subscriptions sub ON sub.device_binding_id = b.id
			ORDER BY b.created_at DESC
			LIMIT $1 OFFSET $2
		`, limit, offset)
	}
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	var out []AdminDeviceRow
	for rows.Next() {
		var row AdminDeviceRow
		if err := rows.Scan(&row.BindingID, &row.DeviceID, &row.DisplayName, &row.HouseholdID, &row.HouseholdName,
			&row.OwnerEmail, &row.Subscription, &row.ServiceStatus, &row.TrialEndsAt, &row.LastContactAt,
			&row.VNMSLifecycle, &row.Removed); err != nil {
			return nil, 0, err
		}
		out = append(out, row)
	}
	return out, total, rows.Err()
}

func (s *Store) ListExpiredTrials(ctx context.Context) ([]TrialExpiredRow, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT b.id, b.device_id, b.household_id, h.name, u.email, sub.trial_ends_at
		FROM subscriptions sub
		JOIN device_bindings b ON b.id = sub.device_binding_id
		JOIN households h ON h.id = b.household_id
		LEFT JOIN users u ON u.id = b.claimed_by_user_id
		WHERE sub.status = 'trialing'
		  AND sub.trial_ends_at IS NOT NULL
		  AND sub.trial_ends_at < now()
		  AND b.removed_at IS NULL
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []TrialExpiredRow
	for rows.Next() {
		var row TrialExpiredRow
		if err := rows.Scan(&row.BindingID, &row.DeviceID, &row.HouseholdID, &row.HouseholdName, &row.OwnerEmail, &row.TrialEndsAt); err != nil {
			return nil, err
		}
		out = append(out, row)
	}
	return out, rows.Err()
}

type TrialExpiredRow struct {
	BindingID     string
	DeviceID      string
	HouseholdID   string
	HouseholdName string
	OwnerEmail    string
	TrialEndsAt   time.Time
}

func (s *Store) SuspendSubscription(ctx context.Context, bindingID string) error {
	tag, err := s.pool.Exec(ctx, `
		UPDATE subscriptions
		SET status = 'past_due', service_status = 'service_suspended', updated_at = now()
		WHERE device_binding_id = $1
	`, bindingID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrBindingNotFound
	}
	return nil
}

func (s *Store) GetUserEmail(ctx context.Context, userID string) (string, error) {
	var email string
	err := s.pool.QueryRow(ctx, `SELECT email FROM users WHERE id = $1`, userID).Scan(&email)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", ErrUserNotFound
	}
	return email, err
}

func (s *Store) AdminDashboardCounts(ctx context.Context) (map[string]int, error) {
	out := map[string]int{}
	queries := map[string]string{
		"users":      `SELECT COUNT(*) FROM users WHERE disabled_at IS NULL`,
		"households": `SELECT COUNT(*) FROM households WHERE archived_at IS NULL`,
		"devices":    `SELECT COUNT(*) FROM device_bindings WHERE removed_at IS NULL`,
		"trialing":   `SELECT COUNT(*) FROM subscriptions WHERE status = 'trialing'`,
		"alerts":     `SELECT COUNT(*) FROM alerts WHERE status = 'active'`,
	}
	for k, q := range queries {
		var n int
		if err := s.pool.QueryRow(ctx, q).Scan(&n); err != nil {
			return nil, err
		}
		out[k] = n
	}
	return out, nil
}
