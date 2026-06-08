package middleware_test

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	appjwt "UalaTwitter/pkg/jwt"
	"UalaTwitter/pkg/middleware"
)

const testSecret = "test-secret-32-chars-padding-here"

func TestJWTMiddleware_ValidToken(t *testing.T) {
	token, _ := appjwt.Issue("user-abc", "alice", testSecret, time.Hour)

	called := false
	handler := middleware.JWT(testSecret)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		id, ok := middleware.UserIDFromCtx(r.Context())
		if !ok || id != "user-abc" {
			t.Errorf("expected userID user-abc, got %q ok=%v", id, ok)
		}
	}))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	handler.ServeHTTP(httptest.NewRecorder(), req)

	if !called {
		t.Error("handler was not called")
	}
}

func TestJWTMiddleware_MissingToken_Returns401(t *testing.T) {
	handler := middleware.JWT(testSecret)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("handler should not be called")
	}))
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w.Code)
	}
}

func TestJWTMiddleware_InvalidToken_Returns401(t *testing.T) {
	handler := middleware.JWT(testSecret)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("handler should not be called")
	}))
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Authorization", "Bearer not-a-valid-jwt")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w.Code)
	}
}

func TestUserIDFromCtx_Empty(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	id, ok := middleware.UserIDFromCtx(req.Context())
	if ok || id != "" {
		t.Error("expected no userID in plain context")
	}
}
