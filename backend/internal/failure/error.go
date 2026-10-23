// Package failure carries machine-readable local failure identity independently
// of user-facing wording. Wrapping with %w preserves both code and cause.
package failure

import "errors"

type Error struct {
	Code    string
	Message string
	Cause   error
}

func (e *Error) Error() string { return e.Message }
func (e *Error) Unwrap() error { return e.Cause }
func New(code, message string, cause error) error {
	return &Error{Code: code, Message: message, Cause: cause}
}
func Code(err error) string {
	var target *Error
	if errors.As(err, &target) {
		return target.Code
	}
	return ""
}
