package postgres_test

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/kimoin/vigelo-backend/internal/auth"
	"github.com/kimoin/vigelo-backend/internal/config"
	"github.com/kimoin/vigelo-backend/internal/store/postgres"
)

func testDB(t *testing.T) *postgres.DB {
	t.Helper()
	url := os.Getenv("VSRV_DATABASE_URL")
	if url == "" {
		url = "postgres://vsrv:vsrv@localhost:5433/vsrv?sslmode=disable"
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	db, err := postgres.Open(ctx, url)
	if err != nil {
		t.Skipf("postgres not available: %v", err)
	}
	return db
}

func TestSignupLoginRefresh(t *testing.T) {
	db := testDB(t)
	defer db.Close()
	store := postgres.NewStore(db)
	ctx := context.Background()
	cfg := config.Load()

	email := "phase2-" + time.Now().Format("150405") + "@example.com"
	signup, err := store.Signup(ctx, email, "password123", "Phase2", cfg.VerifyEmailTTL, cfg.AccessTokenTTL, cfg.RefreshTokenTTL)
	if err != nil {
		t.Fatal(err)
	}
	if signup.User.ID == "" || signup.Tokens.AccessToken == "" {
		t.Fatal("expected signup result")
	}

	user, tokens, err := store.Login(ctx, email, "password123", cfg.AccessTokenTTL, cfg.RefreshTokenTTL)
	if err != nil {
		t.Fatal(err)
	}
	if user.Email != email || tokens.AccessToken == "" {
		t.Fatal("login failed")
	}

	userID, _, err := store.ResolveAccessToken(ctx, signup.Tokens.AccessToken)
	if err != nil {
		t.Fatal(err)
	}
	if userID != signup.User.ID {
		t.Fatalf("user id mismatch %s %s", userID, signup.User.ID)
	}

	newTokens, err := store.RefreshSession(ctx, signup.Tokens.AccessToken, cfg.AccessTokenTTL, cfg.RefreshTokenTTL)
	if err != nil {
		t.Fatal(err)
	}
	if newTokens.AccessToken == signup.Tokens.AccessToken {
		t.Fatal("expected rotated access token")
	}

	_, _, err = store.ResolveAccessToken(ctx, signup.Tokens.AccessToken)
	if err == nil {
		t.Fatal("old access token should be revoked")
	}
	if err != nil && err != auth.ErrInvalidSession {
		// wrapped ok
	}
}
