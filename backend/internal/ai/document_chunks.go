package ai

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
)

func (s *Service) execStagedDocumentChunk(ctx context.Context, raw string, opts RunOptions, finalize bool) (string, string, string, string, error) {
	files, err := s.factory.Workspace(opts.UserID)
	if err != nil {
		return "", "", "", "", err
	}
	fail := func(path string, err error) (string, string, string, string, error) {
		return "", "", "文稿未提交", path, err
	}
	if !finalize {
		args, err := parseChunkWriteArgs(raw)
		if err != nil {
			return fail("", err)
		}
		if len([]rune(args.Content)) > recommendedWorkspaceChunkChars {
			return fail(args.Path, fmt.Errorf("chunk content too long"))
		}
		c, err := files.StageDocumentChunk(ctx, opts.RunID, args.Path, strings.ToLower(args.Mode), args.Content, args.ChunkIndex)
		if err != nil {
			return fail(args.Path, err)
		}
		return mustJSON(map[string]any{"operation": "document_chunk_staged", "path": c.Path, "hash": c.Hash, "chunk_index": c.Index, "changed_paths": []string{}, "needs_finalize": true, "next_hint": "内容已暂存，正式文稿未修改。继续 append 或 finalize_workspace_write 验证并提交。"}), "分块已暂存", "文稿分块已暂存", c.Path, nil
	}
	var args finalizeWriteArgs
	if err := json.Unmarshal([]byte(raw), &args); err != nil {
		return fail("", err)
	}
	c, err := files.DocumentChunk(ctx, opts.RunID, args.Path)
	if err != nil {
		return fail(args.Path, err)
	}
	if args.ExpectedChunks != c.Index || args.ExpectedLines != countWorkspaceTextLines(c.Content) || args.ExpectedChars != len([]rune(c.Content)) || (args.FinalHash != "" && args.FinalHash != c.Hash) {
		return fail(args.Path, fmt.Errorf("分块数量、行数、字符数或哈希不一致；草稿保留，正式文稿未修改"))
	}
	result, err := files.CommitDocumentChunk(ctx, opts.RunID, args.Path, c.Hash)
	if err != nil {
		return fail(args.Path, err)
	}
	return mustJSON(workspaceToolPayload("finalize_workspace_write", result.Path, map[string]any{"operation": "finalize_write", "path": result.Path, "hash": result.Hash, "version_id": result.VersionID, "changed_paths": result.ChangedPaths, "chunks_verified": c.Index})), "文稿已验证并提交", "文稿已应用", result.Path, nil
}
