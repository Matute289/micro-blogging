package middleware

import (
	"net/http"
	"strings"

	appjwt "UalaTwitter/pkg/jwt"
)

// JWT returns a chi middleware that validates a Bearer JWT in the Authorization header.
// Valid tokens inject the userID into the request context via WithUserID.
// Missing or invalid tokens return 401.
func JWT(secret string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			token := bearerToken(r)
			if token == "" {
				http.Error(w, "missing or malformed Authorization header", http.StatusUnauthorized)
				return
			}
			claims, err := appjwt.Verify(token, secret)
			if err != nil {
				http.Error(w, "invalid or expired token", http.StatusUnauthorized)
				return
			}
			next.ServeHTTP(w, r.WithContext(WithUserID(r.Context(), claims.UserID)))
		})
	}
}

func bearerToken(r *http.Request) string {
	h := r.Header.Get("Authorization")
	if !strings.HasPrefix(h, "Bearer ") {
		return ""
	}
	return strings.TrimPrefix(h, "Bearer ")
}
