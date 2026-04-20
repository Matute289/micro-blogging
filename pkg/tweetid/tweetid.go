package tweetid

import (
	"fmt"
	"strconv"
	"strings"
	"time"
)

// Compose builds a tweet ID from a user UUID and creation time.
// Format: {32-hex-user-id}_{unix_milliseconds}
func Compose(userID string, t time.Time) string {
	return strings.ReplaceAll(userID, "-", "") + "_" + strconv.FormatInt(t.UnixMilli(), 10)
}

// Decompose splits a tweet ID back into user UUID and creation time.
func Decompose(id string) (string, time.Time, error) {
	idx := strings.LastIndex(id, "_")
	if idx < 0 {
		return "", time.Time{}, fmt.Errorf("invalid tweet id: %s", id)
	}
	hex := id[:idx]
	if len(hex) != 32 {
		return "", time.Time{}, fmt.Errorf("invalid tweet id: bad user segment")
	}
	ms, err := strconv.ParseInt(id[idx+1:], 10, 64)
	if err != nil {
		return "", time.Time{}, fmt.Errorf("invalid tweet id: bad timestamp")
	}
	userID := fmt.Sprintf("%s-%s-%s-%s-%s", hex[0:8], hex[8:12], hex[12:16], hex[16:20], hex[20:32])
	return userID, time.UnixMilli(ms).UTC(), nil
}
