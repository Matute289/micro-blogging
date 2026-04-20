package tweetid_test

import (
	"strings"
	"testing"
	"time"

	"UalaTwitter/pkg/tweetid"
)

func TestComposeDecompose(t *testing.T) {
	userID := "550e8400-e29b-41d4-a716-446655440000"
	now := time.UnixMilli(1713456789123).UTC()

	id := tweetid.Compose(userID, now)

	gotUserID, gotTime, err := tweetid.Decompose(id)
	if err != nil {
		t.Fatalf("Decompose error: %v", err)
	}
	if gotUserID != userID {
		t.Errorf("userID: got %s, want %s", gotUserID, userID)
	}
	if !gotTime.Equal(now) {
		t.Errorf("time: got %v, want %v", gotTime, now)
	}
}

func TestCompose_FormatIsStable(t *testing.T) {
	userID := "550e8400-e29b-41d4-a716-446655440000"
	now := time.UnixMilli(1713456789123).UTC()
	id := tweetid.Compose(userID, now)

	if !strings.Contains(id, "_") {
		t.Errorf("expected underscore separator in %q", id)
	}
	parts := strings.SplitN(id, "_", 2)
	if len(parts[0]) != 32 {
		t.Errorf("hex segment length: got %d, want 32", len(parts[0]))
	}
}

func TestDecompose_Invalid(t *testing.T) {
	cases := []string{
		"",
		"nounderscore",
		"tooshort_123",
		"550e8400e29b41d4a716446655440000",
		"550e8400e29b41d4a716446655440000_notanumber",
	}
	for _, id := range cases {
		_, _, err := tweetid.Decompose(id)
		if err == nil {
			t.Errorf("expected error for %q, got nil", id)
		}
	}
}
