package auth

import "errors"

var (
	ErrEmailTaken       = errors.New("email already registered")
	ErrInvalidLogin     = errors.New("invalid email or password")
	ErrInvalidSession   = errors.New("invalid session")
	ErrInvalidToken     = errors.New("invalid or expired token")
	ErrInviteNotFound   = errors.New("invite not found")
	ErrInviteExpired    = errors.New("invite expired")
	ErrAlreadyMember    = errors.New("already a household member")
	ErrForbidden        = errors.New("forbidden")
	ErrUserDisabled     = errors.New("user disabled")
	ErrWrongPassword    = errors.New("current password is incorrect")
	ErrDatabaseRequired = errors.New("database required")
)
