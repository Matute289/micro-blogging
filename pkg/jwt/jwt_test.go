package jwt_test

import (
	"testing"
	"time"

	appjwt "UalaTwitter/pkg/jwt"
)

func TestIssueAndVerify_RoundTrip(t *testing.T) {
	token, err := appjwt.Issue("user-123", "alice", "secret", 1*time.Hour)
	if err != nil {
		t.Fatalf("Issue: %v", err)
	}
	if token == "" {
		t.Fatal("expected non-empty token")
	}
	claims, err := appjwt.Verify(token, "secret")
	if err != nil {
		t.Fatalf("Verify: %v", err)
	}
	if claims.UserID != "user-123" {
		t.Errorf("UserID: got %q, want %q", claims.UserID, "user-123")
	}
	if claims.Username != "alice" {
		t.Errorf("Username: got %q, want %q", claims.Username, "alice")
	}
}

func TestVerify_WrongSecret(t *testing.T) {
	token, _ := appjwt.Issue("uid", "u", "secret1", time.Hour)
	_, err := appjwt.Verify(token, "wrong-secret")
	if err == nil {
		t.Fatal("expected error for wrong secret")
	}
}

func TestVerify_Expired(t *testing.T) {
	token, _ := appjwt.Issue("uid", "u", "secret", -1*time.Second)
	_, err := appjwt.Verify(token, "secret")
	if err == nil {
		t.Fatal("expected error for expired token")
	}
}

func TestVerify_InvalidToken(t *testing.T) {
	_, err := appjwt.Verify("not.a.token", "secret")
	if err == nil {
		t.Fatal("expected error for malformed token")
	}
}
