package errors

import (
	"errors"
	"fmt"

	"gorm.io/gorm"
)

var (
	ErrNotFound      = errors.New("resource not found")
	ErrDuplicate     = errors.New("resource already exists")
	ErrUnauthorized  = errors.New("unauthorized")
	ErrForbidden     = errors.New("forbidden")
	ErrBadRequest    = errors.New("bad request")
	ErrInternal      = errors.New("internal server error")
	ErrConflict      = errors.New("resource conflict")
	ErrInvalidID     = errors.New("invalid ID parameter")
	ErrTaskRunning   = errors.New("a task is already running")
	ErrCircuitBreak  = errors.New("circuit breaker open")
	ErrAdapterNotFnd = errors.New("adapter not found")
)

type AppError struct {
	Code    int
	Message string
	Err     error
}

func (e *AppError) Error() string {
	if e.Message != "" {
		return e.Message
	}
	if e.Err != nil {
		return e.Err.Error()
	}
	return "unknown error"
}

func (e *AppError) Unwrap() error {
	return e.Err
}

func NewAppError(code int, message string, err error) *AppError {
	return &AppError{
		Code:    code,
		Message: message,
		Err:     err,
	}
}

func Wrap(err error, message string) error {
	if err == nil {
		return nil
	}
	return &AppError{
		Code:    500,
		Message: message,
		Err:     err,
	}
}

func WrapWithCode(code int, err error, message string) error {
	if err == nil {
		return nil
	}
	return &AppError{
		Code:    code,
		Message: message,
		Err:     err,
	}
}

func IsNotFound(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, ErrNotFound) {
		return true
	}
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return true
	}
	var appErr *AppError
	if errors.As(err, &appErr) {
		return appErr.Code == 404 || errors.Is(appErr.Err, gorm.ErrRecordNotFound)
	}
	return false
}

func IsDuplicate(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, ErrDuplicate) {
		return true
	}
	if errors.Is(err, gorm.ErrDuplicatedKey) {
		return true
	}
	var appErr *AppError
	if errors.As(err, &appErr) {
		return appErr.Code == 409 || errors.Is(appErr.Err, gorm.ErrDuplicatedKey)
	}
	errMsg := err.Error()
	return contains(errMsg, "UNIQUE constraint failed") ||
		contains(errMsg, "duplicate key") ||
		contains(errMsg, "unique constraint")
}

func IsUnauthorized(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, ErrUnauthorized) {
		return true
	}
	var appErr *AppError
	if errors.As(err, &appErr) {
		return appErr.Code == 401
	}
	return false
}

func IsForbidden(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, ErrForbidden) {
		return true
	}
	var appErr *AppError
	if errors.As(err, &appErr) {
		return appErr.Code == 403
	}
	return false
}

func IsBadRequest(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, ErrBadRequest) {
		return true
	}
	var appErr *AppError
	if errors.As(err, &appErr) {
		return appErr.Code == 400
	}
	return false
}

func IsInternalError(err error) bool {
	if err == nil {
		return false
	}
	var appErr *AppError
	if errors.As(err, &appErr) {
		return appErr.Code == 500
	}
	return false
}

func ToAppError(err error) *AppError {
	if err == nil {
		return nil
	}

	var appErr *AppError
	if errors.As(err, &appErr) {
		return appErr
	}

	if errors.Is(err, gorm.ErrRecordNotFound) {
		return &AppError{Code: 404, Message: "resource not found", Err: err}
	}

	if errors.Is(err, gorm.ErrDuplicatedKey) {
		return &AppError{Code: 409, Message: "resource already exists", Err: err}
	}

	return &AppError{Code: 500, Message: "internal server error", Err: err}
}

func GetMessage(err error) string {
	if err == nil {
		return ""
	}

	var appErr *AppError
	if errors.As(err, &appErr) {
		if appErr.Message != "" {
			return appErr.Message
		}
	}

	return err.Error()
}

func GetCode(err error) int {
	if err == nil {
		return 500
	}

	var appErr *AppError
	if errors.As(err, &appErr) {
		return appErr.Code
	}

	if errors.Is(err, gorm.ErrRecordNotFound) {
		return 404
	}

	if errors.Is(err, gorm.ErrDuplicatedKey) {
		return 409
	}

	return 500
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && searchString(s, substr)
}

func searchString(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

func Format(msg string, args ...interface{}) error {
	return &AppError{
		Code:    500,
		Message: fmt.Sprintf(msg, args...),
	}
}

func FormatWithCode(code int, msg string, args ...interface{}) error {
	return &AppError{
		Code:    code,
		Message: fmt.Sprintf(msg, args...),
	}
}
