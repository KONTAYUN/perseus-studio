package ai

import (
	"context"
	"encoding/json"
	"fmt"
)

func (s *Service) execRunWorkspaceCommand(ctx context.Context, userID, argumentsJSON string) (string, string, string, string, error) {
	files, err := s.factory.Workspace(userID)
	if err != nil {
		return "", "", "正在执行命令...", "", err
	}
	var args struct {
		Command    string `json:"command"`
		Cwd        string `json:"cwd"`
		TimeoutSec int    `json:"timeout_sec"`
	}
	if err := json.Unmarshal([]byte(argumentsJSON), &args); err != nil {
		return "", "", "正在执行命令...", "", fmt.Errorf("invalid tool arguments")
	}
	result, err := files.RunCommand(ctx, args.Command, args.Cwd, args.TimeoutSec)
	if err != nil {
		return "", "", "正在执行命令...", args.Cwd, err
	}
	summary := fmt.Sprintf("command cwd=%s exit_code=%d timed_out=%t stdout=%s stderr=%s",
		valueOrRoot(result.Cwd),
		result.ExitCode,
		result.TimedOut,
		truncate(result.Stdout, 220),
		truncate(result.Stderr, 220),
	)
	return mustJSON(result), summary, "正在执行命令...", result.Cwd, nil
}
