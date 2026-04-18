package auth

import (
	"context"
	"testing"

	"github.com/google/uuid"

	"github.com/batuhan/marketintel/internal/domain"
)

func TestJWTRoundTrip(t *testing.T) {
	mgr := NewJWTManager("test-secret-key-12345", 30, 7)
	userID := uuid.New()

	pair, err := mgr.GenerateTokenPair(userID)
	if err != nil {
		t.Fatalf("GenerateTokenPair failed: %v", err)
	}

	if pair.TokenType != "bearer" {
		t.Errorf("expected bearer, got %s", pair.TokenType)
	}
	if pair.AccessToken == "" {
		t.Error("access token is empty")
	}
	if pair.RefreshToken == "" {
		t.Error("refresh token is empty")
	}

	// Validate access token
	gotID, err := mgr.ValidateToken(pair.AccessToken, "access")
	if err != nil {
		t.Fatalf("ValidateToken(access) failed: %v", err)
	}
	if gotID != userID {
		t.Errorf("expected userID %s, got %s", userID, gotID)
	}

	// Validate refresh token
	gotID, err = mgr.ValidateToken(pair.RefreshToken, "refresh")
	if err != nil {
		t.Fatalf("ValidateToken(refresh) failed: %v", err)
	}
	if gotID != userID {
		t.Errorf("expected userID %s, got %s", userID, gotID)
	}
}

func TestJWTWrongType(t *testing.T) {
	mgr := NewJWTManager("test-secret", 30, 7)
	pair, _ := mgr.GenerateTokenPair(uuid.New())

	// Access token should not validate as refresh
	_, err := mgr.ValidateToken(pair.AccessToken, "refresh")
	if err == nil {
		t.Error("expected error validating access token as refresh")
	}

	// Refresh token should not validate as access
	_, err = mgr.ValidateToken(pair.RefreshToken, "access")
	if err == nil {
		t.Error("expected error validating refresh token as access")
	}
}

func TestJWTWrongSecret(t *testing.T) {
	mgr1 := NewJWTManager("secret-1", 30, 7)
	mgr2 := NewJWTManager("secret-2", 30, 7)

	pair, _ := mgr1.GenerateTokenPair(uuid.New())
	_, err := mgr2.ValidateToken(pair.AccessToken, "access")
	if err == nil {
		t.Error("expected error with wrong secret")
	}
}

func TestJWTInvalidToken(t *testing.T) {
	mgr := NewJWTManager("secret", 30, 7)
	_, err := mgr.ValidateToken("not.a.valid.token", "access")
	if err == nil {
		t.Error("expected error for invalid token")
	}
}

func TestPasswordHashAndVerify(t *testing.T) {
	password := "my-secure-password-123!"
	hash, err := HashPassword(password)
	if err != nil {
		t.Fatalf("HashPassword failed: %v", err)
	}

	if hash == password {
		t.Error("hash should not equal plaintext")
	}

	if !VerifyPassword(password, hash) {
		t.Error("VerifyPassword should return true for correct password")
	}

	if VerifyPassword("wrong-password", hash) {
		t.Error("VerifyPassword should return false for wrong password")
	}
}

func TestPasswordHashDifferentOutputs(t *testing.T) {
	password := "same-password"
	hash1, _ := HashPassword(password)
	hash2, _ := HashPassword(password)

	if hash1 == hash2 {
		t.Error("two hashes of same password should differ (different salts)")
	}

	// Both should verify
	if !VerifyPassword(password, hash1) || !VerifyPassword(password, hash2) {
		t.Error("both hashes should verify correctly")
	}
}

func TestUserContext(t *testing.T) {
	ctx := context.Background()

	// No user in context
	if user := UserFromContext(ctx); user != nil {
		t.Error("expected nil user from empty context")
	}

	// With user
	u := &domain.User{
		ID:    uuid.New(),
		Email: "test@example.com",
	}
	ctx = WithUser(ctx, u)
	got := UserFromContext(ctx)
	if got == nil {
		t.Fatal("expected user from context")
	}
	if got.ID != u.ID {
		t.Errorf("expected user ID %s, got %s", u.ID, got.ID)
	}
	if got.Email != u.Email {
		t.Errorf("expected email %s, got %s", u.Email, got.Email)
	}
}
