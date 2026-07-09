package authz

import "strings"

type Role string

const (
	RoleOwner     Role = "owner"
	RoleAdmin     Role = "admin"
	RoleCaregiver Role = "caregiver"
	RoleMember    Role = "member"
)

func CanManageMembers(role Role) bool {
	return role == RoleOwner || role == RoleAdmin
}

func CanClaimDevices(role Role) bool {
	return role == RoleOwner || role == RoleAdmin
}

func CanConfigureDevices(role Role) bool {
	return role == RoleOwner || role == RoleAdmin
}

func CanViewDevices(role Role) bool {
	switch role {
	case RoleOwner, RoleAdmin, RoleCaregiver, RoleMember:
		return true
	default:
		return false
	}
}

func CanManageBilling(role Role) bool {
	return role == RoleOwner
}

func ParseRole(s string) Role {
	return Role(strings.ToLower(strings.TrimSpace(s)))
}
