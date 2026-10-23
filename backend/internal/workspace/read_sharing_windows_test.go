//go:build windows

package workspace

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"syscall"
	"testing"
	"time"
)

func TestBoundedReadRecoversFromTransientWindowsFileLock(t *testing.T) {
	path := filepath.Join(t.TempDir(), "locked.bin")
	want := []byte{0, 255, 128, 13, 10}
	if err := os.WriteFile(path, want, 0600); err != nil {
		t.Fatal(err)
	}
	name, err := syscall.UTF16PtrFromString(path)
	if err != nil {
		t.Fatal(err)
	}
	handle, err := syscall.CreateFile(name, syscall.GENERIC_READ, 0, nil, syscall.OPEN_EXISTING, syscall.FILE_ATTRIBUTE_NORMAL, 0)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := readFileBoundedOnce(path, 1024); !transientFileSharingError(err) {
		syscall.CloseHandle(handle)
		t.Fatalf("lock not reproduced: %v", err)
	}
	done := make(chan struct{})
	go func() { time.Sleep(35 * time.Millisecond); syscall.CloseHandle(handle); close(done) }()
	got, err := readFileBounded(path, 1024)
	<-done
	if err != nil || !bytes.Equal(got, want) {
		t.Fatalf("transient read: %v %v", got, err)
	}
}

func TestWindowsReadLockRetriesAreBoundedAndTyped(t *testing.T) {
	path := filepath.Join(t.TempDir(), "locked.txt")
	if err := os.WriteFile(path, []byte("keep"), 0600); err != nil {
		t.Fatal(err)
	}
	name, _ := syscall.UTF16PtrFromString(path)
	handle, err := syscall.CreateFile(name, syscall.GENERIC_READ, 0, nil, syscall.OPEN_EXISTING, syscall.FILE_ATTRIBUTE_NORMAL, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer syscall.CloseHandle(handle)
	start := time.Now()
	_, err = readFileBounded(path, 1024)
	if !transientFileSharingError(err) || time.Since(start) > time.Second {
		t.Fatalf("unbounded or hidden lock: %v", err)
	}
	for _, err := range []error{nil, os.ErrNotExist, os.ErrPermission, errors.New("sharing violation")} {
		if transientFileSharingError(err) {
			t.Fatal("unrelated error retried", err)
		}
	}
}
