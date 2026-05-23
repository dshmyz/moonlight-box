package types

import "errors"

var (
	ErrNotFound       = errors.New("not found")
	ErrReadOnly       = errors.New("repository is read only")
	ErrNotMatched     = errors.New("route not matched")
	ErrInvalidPath    = errors.New("invalid path")
	ErrInvalidKey     = errors.New("invalid artifact key")
	ErrBlobNotFound   = errors.New("blob not found")
	ErrBlobCorrupted  = errors.New("blob corrupted")
	ErrNotImplemented = errors.New("not implemented")
)
