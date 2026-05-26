package util

import "errors"

var (
	ErrUserNotFound       = errors.New("user not found")
	ErrInvalidCredentials = errors.New("invalid username or password")
	ErrUserAlreadyExists  = errors.New("user already exists")
	ErrTokenExpired       = errors.New("token has expired")
	ErrTokenInvalid       = errors.New("token is invalid")
	ErrPasswordTooShort   = errors.New("password must be at least 8 characters")
	ErrAccessDenied       = errors.New("access denied")
	ErrPackageNotFound    = errors.New("package not found")
	ErrVersionNotFound    = errors.New("version not found")
	ErrRoleNotFound       = errors.New("role not found")
	ErrRepoNotFound       = errors.New("repository not found")
)

// IsErr 检查错误是否是目标错误
func IsErr(err, target error) bool {
	return errors.Is(err, target)
}
