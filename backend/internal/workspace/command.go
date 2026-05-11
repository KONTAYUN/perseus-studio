package workspace

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"runtime"
	"strings"
	"time"
)

func (s *Service) RunCommand(ctx context.Context, command, cwd string, timeoutSec int) (CommandResult, error) {
	command = strings.TrimSpace(command)
	if command == "" {
		return CommandResult{}, fmt.Errorf("command is required")
	}
	if err := validateSafeCommand(command); err != nil {
		return CommandResult{}, err
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

	cmd := shellCommand(runCtx, command)
	cmd.Dir = absCwd
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err = cmd.Run()
	finishedAt := time.Now().UTC()
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

	return CommandResult{
		Command:    command,
		Cwd:        cleanCwd,
		Stdout:     truncateCommandOutput(stdout.String()),
		Stderr:     truncateCommandOutput(stderr.String()),
		ExitCode:   exitCode,
		TimedOut:   timedOut,
		StartedAt:  startedAt,
		FinishedAt: finishedAt,
	}, nil
}

func shellCommand(ctx context.Context, command string) *exec.Cmd {
	if runtime.GOOS == "windows" {
		return exec.CommandContext(ctx, "powershell", "-NoProfile", "-NonInteractive", "-Command", command)
	}
	return exec.CommandContext(ctx, "/bin/sh", "-c", command)
}

func validateSafeCommand(command string) error {
	normalized := strings.ToLower(command)
	normalized = strings.Join(strings.Fields(normalized), " ")
	dangerousContains := []string{
		"mkfs",
		"shutdown",
		"reboot",
		"systemctl",
		"service ",
		"docker",
		"ssh",
		"scp",
		"sudo",
	}
	for _, fragment := range dangerousContains {
		if strings.Contains(normalized, fragment) {
			return fmt.Errorf("command rejected by safety policy: %s", strings.TrimSpace(fragment))
		}
	}

	dangerousPatterns := []*regexp.Regexp{
		regexp.MustCompile(`\brm\s+-[^\s]*r[^\s]*f[^\s]*(?:\s+--)?\s+/`),
		regexp.MustCompile(`\brm\s+-[^\s]*r[^\s]*f[^\s]*(?:\s+--)?\s+\*`),
		regexp.MustCompile(`\bcurl\b.*\|\s*bash\b`),
		regexp.MustCompile(`\bwget\b.*\|\s*bash\b`),
		regexp.MustCompile(`\bchmod\s+-r\s+/`),
		regexp.MustCompile(`\bchown\s+-r\s+/`),
		regexp.MustCompile(`\bdd\b`),
		regexp.MustCompile(`\bapt(?:-get)?\b`),
		regexp.MustCompile(`\byum\b`),
		regexp.MustCompile(`\bpacman\b`),
	}
	for _, pattern := range dangerousPatterns {
		if pattern.MatchString(normalized) {
			return fmt.Errorf("command rejected by safety policy")
		}
	}
	return nil
}

func truncateCommandOutput(value string) string {
	const maxRunes = 20000
	runes := []rune(value)
	if len(runes) <= maxRunes {
		return value
	}
	return string(runes[:maxRunes]) + "\n...[output truncated]"
}
