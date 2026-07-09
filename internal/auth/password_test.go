package auth

import "testing"

func TestHashAndVerifyPassword(t *testing.T) {
	hash, err := HashPassword("test-password-123")
	if err != nil {
		t.Fatal(err)
	}
	ok, err := VerifyPassword("test-password-123", hash)
	if err != nil || !ok {
		t.Fatalf("verify ok=%v err=%v", ok, err)
	}
	ok, err = VerifyPassword("wrong", hash)
	if err != nil || ok {
		t.Fatalf("wrong password should fail ok=%v err=%v", ok, err)
	}
}

func TestHashTokenDeterministic(t *testing.T) {
	a := HashToken("abc")
	b := HashToken("abc")
	if a != b || a == "" {
		t.Fatalf("hash = %q %q", a, b)
	}
}
