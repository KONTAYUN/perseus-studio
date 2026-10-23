package ai

import (
	"context"
	"encoding/json"
	"fmt"
	"webide-backend/internal/failure"
)

func (s *Service) execRunWorkspaceCommand(ctx context.Context, userID, argumentsJSON string) (string, string, string, string, error) {
	files, err := s.factory.Workspace(userID)
	if err != nil {
		return "", "", "正在执行命令...", "", err
	}
	var args struct {
		Command           string `json:"command"`
		Cwd               string `json:"cwd"`
		TimeoutSec        int    `json:"timeout_sec"`
		ExpectedExitCodes []int  `json:"expected_exit_codes"`
	}
	if err := json.Unmarshal([]byte(argumentsJSON), &args); err != nil {
		return "", "", "正在执行命令...", "", failure.New("tool_invalid_arguments", "invalid tool arguments", nil)
	}
	result, err := files.RunCommand(ctx, args.Command, args.Cwd, args.TimeoutSec)
	if err != nil {
		return "", "", "正在执行命令...", args.Cwd, err
	}
	if len(args.ExpectedExitCodes) > 0 && result.ExecutionOutcome == "failed" {
		for _, code := range args.ExpectedExitCodes {
			if result.ExitCode == code {
				result.ExecutionOutcome = "success"
				break
			}
		}
	}
	if !result.ObservationComplete && result.ExecutionOutcome == "success" {
		result.ExecutionOutcome = "observation_incomplete"
	}
	summary := fmt.Sprintf("command cwd=%s exit_code=%d timed_out=%t stdout=%s stderr=%s",
		valueOrRoot(result.Cwd),
		result.ExitCode,
		result.TimedOut,
		truncate(result.Stdout, 220),
		truncate(result.Stderr, 220),
	)
	path := result.Cwd
	if len(result.ChangedPaths) > 0 {
		path = result.ChangedPaths[0]
		summary = fmt.Sprintf("%s changed_paths=%d", summary, len(result.ChangedPaths))
	}
	return mustJSON(result), summary, "正在执行命令...", path, nil
}
