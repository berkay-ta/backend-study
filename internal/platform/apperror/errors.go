package apperror

import (
	"errors"
	"fmt"
	"net/http"
)

type FieldError struct {
	Field   string `json:"field"`
	Message string `json:"message"`
}

type Error struct {
	Status int
	Code   Code
	Msg    string
	Cause  error
	Fields []FieldError
}

func (e *Error) Error() string {
	if e == nil {
		return ""
	}
	if e.Cause != nil {
		return fmt.Sprintf("%s: %s (%v)", e.Code, e.Msg, e.Cause)
	}
	return fmt.Sprintf("%s: %s", e.Code, e.Msg)
}

func (e *Error) Unwrap() error { return e.Cause }

func (e *Error) WithCause(err error) *Error {
	if e == nil {
		return nil
	}
	cp := *e
	cp.Cause = err
	return &cp
}

func (e *Error) WithFields(fields ...FieldError) *Error {
	if e == nil {
		return nil
	}
	cp := *e
	cp.Fields = append([]FieldError(nil), fields...)
	return &cp
}

func New(status int, code Code, msg string) *Error {
	return &Error{Status: status, Code: code, Msg: msg}
}

func As(err error) (*Error, bool) {
	var ae *Error
	if errors.As(err, &ae) {
		return ae, true
	}
	return nil, false
}

// FromError lifts any error into an *Error: typed errors pass through,
// anything else becomes a 500 internal_error.
func FromError(err error) *Error {
	if err == nil {
		return nil
	}
	if ae, ok := As(err); ok {
		return ae
	}
	return &Error{
		Status: http.StatusInternalServerError,
		Code:   CodeInternal,
		Msg:    "internal server error",
		Cause:  err,
	}
}

func BadRequest(msg string) *Error {
	return New(http.StatusBadRequest, CodeBadRequest, msg)
}

func ValidationFailed(fields ...FieldError) *Error {
	return New(http.StatusBadRequest, CodeValidationFailed, "validation failed").WithFields(fields...)
}

func NotFound(code Code, msg string) *Error {
	return New(http.StatusNotFound, code, msg)
}

func Conflict(code Code, msg string) *Error {
	return New(http.StatusConflict, code, msg)
}

func Internal(cause error) *Error {
	return (&Error{
		Status: http.StatusInternalServerError,
		Code:   CodeInternal,
		Msg:    "internal server error",
	}).WithCause(cause)
}

func Unavailable(code Code, msg string) *Error {
	return New(http.StatusServiceUnavailable, code, msg)
}

func BadGateway(code Code, msg string) *Error {
	return New(http.StatusBadGateway, code, msg)
}
