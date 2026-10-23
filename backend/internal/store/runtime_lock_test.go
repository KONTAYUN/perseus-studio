package store

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

func TestRuntimeLockRejectsDuplicateAndAllowsReleasedOwner(t *testing.T) {
	path := filepath.Join(t.TempDir(), "owner.lock")
	first, err := AcquireRuntimeLock(path)
	if err != nil {
		t.Fatal(err)
	}
	defer first.Close()
	if second, err := AcquireRuntimeLock(path); err == nil {
		second.Close()
		t.Fatal("duplicate server ownership allowed")
	}
	first.Close()
	next, err := AcquireRuntimeLock(path)
	if err != nil {
		t.Fatal(err)
	}
	next.Close()
}

func TestRuntimeLockReleasedWhenProcessDies(t *testing.T) {
	path := filepath.Join(t.TempDir(), "owner.lock")
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	child := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestRuntimeLockChildHelper$")
	child.Env = append(os.Environ(), "PERSEUS_LOCK_TEST_PATH="+path)
	input, err := child.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}
	defer input.Close()
	output, err := child.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := child.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() { child.Process.Kill(); child.Wait() }()
	ready, err := bufio.NewReader(output).ReadString('\n')
	if err != nil || ready != "locked\n" {
		t.Fatalf("child ownership failed: %q %v", ready, err)
	}
	if other, err := AcquireRuntimeLock(path); err == nil {
		other.Close()
		t.Fatal("second process acquired active lock")
	}
	child.Process.Kill()
	child.Wait()
	next, err := AcquireRuntimeLock(path)
	if err != nil {
		t.Fatalf("crash left stale ownership: %v", err)
	}
	next.Close()
}

func TestRuntimeLockChildHelper(t *testing.T) {
	path := os.Getenv("PERSEUS_LOCK_TEST_PATH")
	if path == "" {
		return
	}
	lock, err := AcquireRuntimeLock(path)
	if err != nil {
		t.Fatal(err)
	}
	defer lock.Close()
	fmt.Print("locked\n")
	_, _ = bufio.NewReader(os.Stdin).ReadString('\n')
}
