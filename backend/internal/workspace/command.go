package workspace

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"time"
)

func (s *Service) RunCommand(ctx context.Context, command, cwd string, timeoutSec int) (CommandResult, error) {
	if !TrustedHostCommandsEnabled() {
		return CommandResult{}, commandDisabledError()
	}
	command = strings.TrimSpace(command)
	if command == "" {
		return CommandResult{}, fmt.Errorf("command is required")
	}

	if timeoutSec <= 0 {
		timeoutSec = 120
	}
	if timeoutSec > 600 {
		timeoutSec = 600
	}

	cleanCwd, absCwd, err := s.sanitizeOptionalPath(cwd)
	if err != nil {
		return CommandResult{}, err
	}
	if isAnyTrashPath(cleanCwd) {
		return CommandResult{}, trashManagedOnlyError()
	}
	info, err := os.Stat(absCwd)
	if err != nil {
		return CommandResult{}, err
	}
	if !info.IsDir() {
		return CommandResult{}, fmt.Errorf("cwd is not a directory")
	}

	startedAt := time.Now().UTC()
	runCtx, cancel := context.WithTimeout(ctx, time.Duration(timeoutSec)*time.Second)
	defer cancel()
	before, err := s.commandWorkspaceSnapshotContext(runCtx)
	if err != nil {
		return CommandResult{}, err
	}

	cmd := shellCommand(runCtx, command)
	cmd.Dir = absCwd
	cmd.Env, err = s.commandEnvironment()
	if err != nil {
		return CommandResult{}, err
	}
	var stdout boundedCommandOutput
	var stderr boundedCommandOutput
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	cmd.WaitDelay = 2 * time.Second

	err = cmd.Run()
	finishedAt := time.Now().UTC()
	scanCtx, scanCancel := context.WithTimeout(context.WithoutCancel(ctx), 3*time.Second)
	after, snapshotErr := s.commandWorkspaceSnapshotContext(scanCtx)
	scanCancel()
	timedOut := errors.Is(runCtx.Err(), context.DeadlineExceeded)
	exitCode := 0
	if timedOut {
		exitCode = -1
	} else if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			exitCode = exitErr.ExitCode()
		} else {
			return CommandResult{}, err
		}
	}

	var changedPaths []string
	if snapshotErr == nil {
		changedPaths = diffCommandWorkspaceSnapshots(before, after)
	}
	outcome := "success"
	if exitCode != 0 {
		outcome = "failed"
	}
	if timedOut {
		outcome = "timed_out"
	}
	if errors.Is(runCtx.Err(), context.Canceled) {
		outcome = "canceled"
	}
	observationError := ""
	if snapshotErr != nil {
		observationError = snapshotErr.Error()
	}
	workspaceMutation := len(changedPaths) > 0
	hash := ""
	if workspaceMutation {
		hash = hashCommandChangedPaths(changedPaths, after)
	}

	return CommandResult{
		ExecutionOutcome:    outcome,
		ObservationComplete: snapshotErr == nil,
		ObservationError:    observationError,
		Command:             command,
		Cwd:                 cleanCwd,
		Stdout:              truncateCommandOutput(stdout.String()),
		Stderr:              truncateCommandOutput(stderr.String()),
		ExitCode:            exitCode,
		TimedOut:            timedOut,
		StartedAt:           startedAt,
		FinishedAt:          finishedAt,
		ChangedPaths:        changedPaths,
		WorkspaceMutation:   workspaceMutation,
		Hash:                hash,
	}, nil
}

func hashCommandChangedPaths(paths []string, snapshot commandWorkspaceSnapshot) string {
	if len(paths) == 0 {
		return ""
	}
	var builder strings.Builder
	for _, path := range paths {
		item, exists := snapshot[path]
		builder.WriteString(path)
		builder.WriteByte('\x00')
		if !exists {
			builder.WriteString("deleted")
		} else {
			builder.WriteString(item.Type)
			builder.WriteByte(':')
			builder.WriteString(item.Hash)
		}
		builder.WriteByte('\n')
	}
	sum := sha256.Sum256([]byte(builder.String()))
	return hex.EncodeToString(sum[:])
}

func shellCommand(ctx context.Context, command string) *exec.Cmd {
	var cmd *exec.Cmd
	if runtime.GOOS == "windows" {
		cmd = exec.CommandContext(ctx, "powershell", "-NoProfile", "-NonInteractive", "-Command", command)
	} else {
		cmd = exec.CommandContext(ctx, "/bin/sh", "-c", command)
	}
	configureCommandCancellation(cmd)
	return cmd
}

func truncateCommandOutput(value string) string {
	const maxRunes = 20000
	runes := []rune(value)
	if len(runes) <= maxRunes {
		return value
	}
	return string(runes[:maxRunes]) + "\n...[output truncated]"
}

// Cap output while draining the process pipes. Truncating after Run returns
// still lets an unbounded command exhaust the server's memory.
type boundedCommandOutput struct {
	buffer    bytes.Buffer
	truncated bool
}

func (b *boundedCommandOutput) Write(p []byte) (int, error) {
	const maxBytes = 80000
	n := len(p)
	remaining := maxBytes - b.buffer.Len()
	if len(p) > remaining {
		p = p[:remaining]
		b.truncated = true
	}
	_, _ = b.buffer.Write(p)
	return n, nil
}

func (b *boundedCommandOutput) String() string {
	text := b.buffer.String()
	if b.truncated {
		text += "\n...[output truncated]"
	}
	return text
}
