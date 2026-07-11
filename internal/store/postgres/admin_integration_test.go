package postgres_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/kimoin/vigelo-backend/internal/auth"
	"github.com/kimoin/vigelo-backend/internal/config"
	"github.com/kimoin/vigelo-backend/internal/store/postgres"
)

func TestAdminCreateAndDeleteUser(t *testing.T) {
	db := testDB(t)
	defer db.Close()
	store := postgres.NewStore(db)
	ctx := context.Background()
	cfg := config.Load()

	suffix := time.Now().Format("150405.000000")
	email := "admin-create-" + suffix + "@example.com"
	password := "password123"

	res, err := store.AdminCreateUser(ctx, email, password, "Created By Admin", "")
	if err != nil {
		t.Fatal(err)
	}
	if res.User.ID == "" || res.Household.ID == "" {
		t.Fatal("expected user and household")
	}
	if res.User.EmailVerifiedAt == nil {
		t.Fatal("admin-created user should be email verified immediately")
	}

	user, _, err := store.Login(ctx, email, password, cfg.AccessTokenTTL, cfg.RefreshTokenTTL)
	if err != nil {
		t.Fatal("admin-created user should login without email verification:", err)
	}
	if user.Email != email {
		t.Fatalf("email mismatch: %s", user.Email)
	}

	if err := store.AdminDeleteUser(ctx, res.User.ID); err != nil {
		t.Fatal(err)
	}
	if err := store.AdminDeleteUser(ctx, res.User.ID); err != postgres.ErrUserNotFound {
		t.Fatalf("second delete: got %v, want ErrUserNotFound", err)
	}
}

func TestAdminCreateUserDuplicateEmail(t *testing.T) {
	db := testDB(t)
	defer db.Close()
	store := postgres.NewStore(db)
	ctx := context.Background()

	suffix := time.Now().Format("150405.000000")
	email := "admin-dup-" + suffix + "@example.com"

	if _, err := store.AdminCreateUser(ctx, email, "password123", "", ""); err != nil {
		t.Fatal(err)
	}
	defer store.AdminDeleteUser(ctx, mustUserID(ctx, t, store, email))

	if _, err := store.AdminCreateUser(ctx, email, "password456", "", ""); err != auth.ErrEmailTaken {
		t.Fatalf("duplicate create: got %v, want ErrEmailTaken", err)
	}
}

func mustUserID(ctx context.Context, t *testing.T, store *postgres.Store, email string) string {
	t.Helper()
	rows, total, err := store.AdminSearchUsers(ctx, strings.ToLower(email), "email", 5, 0)
	if err != nil || total != 1 || len(rows) != 1 {
		t.Fatalf("lookup user %s: total=%d err=%v", email, total, err)
	}
	return rows[0].ID
}

func TestChangePassword(t *testing.T) {
	db := testDB(t)
	defer db.Close()
	store := postgres.NewStore(db)
	ctx := context.Background()
	cfg := config.Load()

	email := "change-pw-" + time.Now().Format("150405.000000") + "@example.com"
	oldPassword := "password123"
	newPassword := "newpassword456"

	res, err := store.AdminCreateUser(ctx, email, oldPassword, "", "")
	if err != nil {
		t.Fatal(err)
	}
	defer store.AdminDeleteUser(ctx, res.User.ID)

	if err := store.ChangePassword(ctx, res.User.ID, "wrong-password", newPassword); err != auth.ErrWrongPassword {
		t.Fatalf("wrong current password: got %v, want ErrWrongPassword", err)
	}

	if err := store.ChangePassword(ctx, res.User.ID, oldPassword, newPassword); err != nil {
		t.Fatal(err)
	}

	if _, _, err := store.Login(ctx, email, oldPassword, cfg.AccessTokenTTL, cfg.RefreshTokenTTL); err == nil {
		t.Fatal("old password should no longer work")
	}

	if _, _, err := store.Login(ctx, email, newPassword, cfg.AccessTokenTTL, cfg.RefreshTokenTTL); err != nil {
		t.Fatal("login with new password failed:", err)
	}
}
