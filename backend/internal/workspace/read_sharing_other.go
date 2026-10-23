//go:build !windows

package workspace

func transientFileSharingError(error) bool { return false }
