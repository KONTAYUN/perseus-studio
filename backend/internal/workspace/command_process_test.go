package workspace

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"
)

func TestCancelCommandStopsItsChildProcess(t *testing.T) {
	root := t.TempDir()
	command := `(sleep 2; echo orphan > orphan.txt) & echo ready > ready.txt; wait`
	if runtime.GOOS == "windows" {
		command = `$child = Start-Process powershell -WindowStyle Hidden -PassThru -ArgumentList '-NoProfile -NonInteractive -Command "Start-Sleep -Seconds 2; Set-Content orphan.txt orphan"'; Set-Content ready.txt ready; Wait-Process -Id $child.Id`
	}
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	cmd := shellCommand(ctx, command)
	cmd.Dir = root
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	deadline := time.Now().Add(8 * time.Second)
	for {
		if _, err := os.Stat(filepath.Join(root, "ready.txt")); err == nil {
			break
		}
		if time.Now().After(deadline) {
			cancel()
			<-done
			t.Fatal("child process did not start")
		}
		time.Sleep(20 * time.Millisecond)
	}
	cancel()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("canceled command did not terminate")
	}
	time.Sleep(2500 * time.Millisecond)
	if _, err := os.Stat(filepath.Join(root, "orphan.txt")); !os.IsNotExist(err) {
		t.Fatalf("child kept executing after cancel: %v", err)
	}
}
