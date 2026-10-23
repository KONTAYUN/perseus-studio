package ai

import (
	"encoding/json"
	"fmt"

	"webide-backend/internal/workspace"
)

func verifyWorkspacePatchChanges(files *workspace.Service, resultJSON string) ToolVerification {
	var result struct {
		Changes []workspace.FileChange `json:"changes"`
	}
	if err := json.Unmarshal([]byte(resultJSON), &result); err != nil {
		return ToolVerification{Status: "failed", Message: "invalid patch result"}
	}
	if len(result.Changes) == 0 {
		return ToolVerification{Status: "failed", Message: "patch result contains no changes"}
	}
	if err := files.VerifyChanges(result.Changes); err != nil {
		return ToolVerification{Status: "failed", Message: err.Error()}
	}
	return ToolVerification{Status: "verified", Message: fmt.Sprintf("verified %d file changes", len(result.Changes)), Path: result.Changes[0].Path}
}
