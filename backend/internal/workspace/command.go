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
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"mvdan.cc/sh/v3/syntax"
)

type CommandAction struct {
	Executable string   `json:"executable"`
	Arguments  []string `json:"arguments,omitempty"`
	Redirects  []string `json:"redirects,omitempty"`
	InPipeline bool     `json:"in_pipeline"`
	RiskLevel  string   `json:"risk_level"`
}

func (s *Service) RunCommand(ctx context.Context, command, cwd string, timeoutSec int) (CommandResult, error) {
	command = strings.TrimSpace(command)
	if command == "" {
		return CommandResult{}, ErrCommandRequired
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
	before, err := s.commandWorkspaceSnapshot()
	if err != nil {
		return CommandResult{}, err
	}
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
	after, snapshotErr := s.commandWorkspaceSnapshot()
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
	if snapshotErr != nil {
		return CommandResult{}, snapshotErr
	}
	changedPaths := diffCommandWorkspaceSnapshots(before, after)
	workspaceMutation := len(changedPaths) > 0
	hash := ""
	if workspaceMutation {
		hash = hashCommandChangedPaths(changedPaths, after)
	}

	return CommandResult{
		Command:           command,
		Cwd:               cleanCwd,
		Stdout:            truncateCommandOutput(stdout.String()),
		Stderr:            truncateCommandOutput(stderr.String()),
		ExitCode:          exitCode,
		TimedOut:          timedOut,
		StartedAt:         startedAt,
		FinishedAt:        finishedAt,
		ChangedPaths:      changedPaths,
		WorkspaceMutation: workspaceMutation,
		Hash:              hash,
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
	if runtime.GOOS == "windows" {
		return exec.CommandContext(ctx, "powershell", "-NoProfile", "-NonInteractive", "-Command", command)
	}
	return exec.CommandContext(ctx, "/bin/sh", "-c", command)
}

func validateSafeCommand(command string) error {
	actions, err := analyzeCommand(command)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrCommandRejected, err)
	}
	for _, action := range actions {
		if action.RiskLevel == ToolCommandRiskHigh {
			return fmt.Errorf("%w: executable %s is not allowed", ErrCommandRejected, action.Executable)
		}
		for _, target := range action.Redirects {
			if commandPathEscapesWorkspace(target) {
				return fmt.Errorf("%w: redirection target escapes the workspace", ErrCommandRejected)
			}
		}
	}
	if unsafeDownloadPipe(actions) {
		return fmt.Errorf("%w: downloaded content cannot be piped into a shell", ErrCommandRejected)
	}
	return nil
}

const (
	ToolCommandRiskLow  = "low"
	ToolCommandRiskHigh = "high"
)

func analyzeCommand(command string) ([]CommandAction, error) {
	file, err := syntax.NewParser().Parse(strings.NewReader(command), "")
	if err != nil {
		return nil, err
	}
	var actions []CommandAction
	var analysisErr error
	syntax.Walk(file, func(node syntax.Node) bool {
		if analysisErr != nil {
			return false
		}
		call, ok := node.(*syntax.CallExpr)
		if !ok {
			return true
		}
		words := make([]string, 0, len(call.Args))
		for _, word := range call.Args {
			value, literal := commandLiteralWord(word)
			if !literal {
				analysisErr = fmt.Errorf("dynamic command words are not allowed")
				return false
			}
			words = append(words, value)
		}
		if len(words) == 0 {
			return true
		}
		action := CommandAction{
			Executable: filepath.Base(strings.ToLower(words[0])),
			Arguments:  append([]string(nil), words[1:]...),
			RiskLevel:  commandRiskLevel(filepath.Base(strings.ToLower(words[0])), words[1:]),
		}
		actions = append(actions, action)
		return true
	})
	if analysisErr != nil {
		return nil, analysisErr
	}
	if len(actions) == 0 {
		return nil, fmt.Errorf("command contains no executable action")
	}

	pipeline := false
	redirects := []string{}
	syntax.Walk(file, func(node syntax.Node) bool {
		switch item := node.(type) {
		case *syntax.BinaryCmd:
			if item.Op.String() == "|" || item.Op.String() == "|&" {
				pipeline = true
			}
		case *syntax.Redirect:
			target, literal := commandLiteralWord(item.Word)
			if !literal {
				analysisErr = fmt.Errorf("dynamic redirection targets are not allowed")
				return false
			}
			redirects = append(redirects, target)
		}
		return analysisErr == nil
	})
	if analysisErr != nil {
		return nil, analysisErr
	}
	if pipeline {
		for index := range actions {
			actions[index].InPipeline = true
		}
	}
	if len(redirects) > 0 {
		actions[len(actions)-1].Redirects = redirects
	}
	return actions, nil
}

func commandLiteralWord(word *syntax.Word) (string, bool) {
	if word == nil {
		return "", true
	}
	var value strings.Builder
	for _, part := range word.Parts {
		switch item := part.(type) {
		case *syntax.Lit:
			value.WriteString(item.Value)
		case *syntax.SglQuoted:
			value.WriteString(item.Value)
		case *syntax.DblQuoted:
			for _, quotedPart := range item.Parts {
				literal, ok := quotedPart.(*syntax.Lit)
				if !ok {
					return "", false
				}
				value.WriteString(literal.Value)
			}
		default:
			return "", false
		}
	}
	return value.String(), true
}

func commandRiskLevel(executable string, args []string) string {
	switch executable {
	case "mkfs", "mkfs.ext2", "mkfs.ext3", "mkfs.ext4", "mkfs.xfs",
		"shutdown", "reboot", "systemctl", "service", "docker",
		"ssh", "scp", "sudo", "dd", "apt", "apt-get", "yum", "pacman":
		return ToolCommandRiskHigh
	case "rm":
		if commandHasFlag(args, "r") && commandHasFlag(args, "f") && commandHasRootTarget(args) {
			return ToolCommandRiskHigh
		}
	case "chmod", "chown":
		if commandHasRecursiveFlag(args) && commandHasRootTarget(args) {
			return ToolCommandRiskHigh
		}
	}
	return ToolCommandRiskLow
}

func commandHasFlag(args []string, flag string) bool {
	for _, arg := range args {
		if strings.HasPrefix(arg, "-") && strings.Contains(strings.TrimLeft(arg, "-"), flag) {
			return true
		}
	}
	return false
}

func commandHasRecursiveFlag(args []string) bool {
	for _, arg := range args {
		if arg == "--recursive" || commandHasFlag([]string{arg}, "R") || commandHasFlag([]string{arg}, "r") {
			return true
		}
	}
	return false
}

func commandHasRootTarget(args []string) bool {
	for _, arg := range args {
		if strings.HasPrefix(arg, "-") {
			continue
		}
		switch strings.TrimSpace(arg) {
		case "/", "/*", "*":
			return true
		}
	}
	return false
}

func commandPathEscapesWorkspace(target string) bool {
	target = filepath.ToSlash(strings.TrimSpace(target))
	return filepath.IsAbs(target) || target == ".." || strings.HasPrefix(target, "../")
}

func unsafeDownloadPipe(actions []CommandAction) bool {
	if len(actions) < 2 {
		return false
	}
	for index := 0; index < len(actions)-1; index++ {
		if actions[index].Executable != "curl" && actions[index].Executable != "wget" {
			continue
		}
		switch actions[index+1].Executable {
		case "sh", "bash", "zsh", "dash", "ksh":
			return true
		}
	}
	return false
}

func truncateCommandOutput(value string) string {
	const maxRunes = 20000
	runes := []rune(value)
	if len(runes) <= maxRunes {
		return value
	}
	return string(runes[:maxRunes]) + "\n...[output truncated]"
}
