//go:build windows

package workspace

import (
	"errors"
	"syscall"
)

func transientFileSharingError(err error) bool {
	return errors.Is(err, syscall.Errno(32)) || errors.Is(err, syscall.Errno(33)) // SHARING_VIOLATION / LOCK_VIOLATION
}
