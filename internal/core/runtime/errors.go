package runtime

import "errors"

var (
	ErrNotFound       = errors.New("not found")
	ErrNotImplemented = errors.New("not implemented")
	ErrReadOnly       = errors.New("read only")
	ErrNotMatched     = errors.New("not matched")
	ErrInvalidUpload  = errors.New("invalid upload session state")
	ErrBlocked        = errors.New("blocked by rule")
)

// BlockedError preserves a safe, user-facing rule reason while remaining
// compatible with callers that use errors.Is(err, ErrBlocked).
type BlockedError struct {
	Reason string
}

func (e *BlockedError) Error() string { return ErrBlocked.Error() }
func (e *BlockedError) Unwrap() error { return ErrBlocked }

func NewBlockedError(reason string) error {
	if reason == "" {
		return ErrBlocked
	}
	return &BlockedError{Reason: reason}
}

func BlockReason(err error) string {
	var blocked *BlockedError
	if errors.As(err, &blocked) {
		return blocked.Reason
	}
	return ""
}
