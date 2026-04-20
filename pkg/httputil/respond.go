package httputil

import (
	"errors"
	"log/slog"
	"net/http"

	"UalaTwitter/pkg/apperr"
	"github.com/go-chi/chi/v5/middleware"
)

// WriteError maps apperr.Error to 4xx responses and logs unexpected errors as 500.
func WriteError(w http.ResponseWriter, r *http.Request, err error) {
	var appErr *apperr.Error
	if errors.As(err, &appErr) {
		http.Error(w, appErr.Error(), appErr.Status())
		return
	}
	slog.Error("internal error",
		"error", err,
		"request_id", middleware.GetReqID(r.Context()),
		"method", r.Method,
		"path", r.URL.Path,
	)
	http.Error(w, "internal server error", http.StatusInternalServerError)
}
