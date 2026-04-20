package apperr

import "net/http"

// Error is an application-level error with an HTTP status code and a safe client message.
type Error struct {
	status int
	msg    string
}

func (e *Error) Error() string { return e.msg }
func (e *Error) Status() int   { return e.status }

func NotFound(msg string) *Error   { return &Error{http.StatusNotFound, msg} }
func Invalid(msg string) *Error    { return &Error{http.StatusBadRequest, msg} }
func Conflict(msg string) *Error   { return &Error{http.StatusConflict, msg} }
func NotAllowed(msg string) *Error { return &Error{http.StatusMethodNotAllowed, msg} }

var ErrNotFound = NotFound("not found")
