package runtime

import "errors"

var (
	ErrNotFound       = errors.New("not found")
	ErrNotImplemented = errors.New("not implemented")
	ErrReadOnly       = errors.New("read only")
	ErrNotMatched     = errors.New("not matched")
	ErrInvalidUpload  = errors.New("invalid upload session state")
)
