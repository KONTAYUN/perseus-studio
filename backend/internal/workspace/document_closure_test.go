package workspace

import (
	"context"
	"errors"
	"os"
	"testing"
	"webide-backend/internal/document"
)

func TestDocumentRepairStagingLimitsConflictCancellationAndConfirmation(t *testing.T) {
	s := newTestService(t)
	ctx := t.Context()
	head, err := s.CreateFile(ctx, "修复.md", "正文 123\n")
	if err != nil {
		t.Fatal(err)
	}
	action := &document.Action{Type: "format_repair", Path: head.Path, BaseHash: head.Hash, Profile: document.Version, Diagnostics: []document.Issue{{Code: "conversion", Message: "测试诊断"}}}
	candidate, err := s.StageDocumentRepair(ctx, "r", action, "改写正文 456\n")
	if err != nil {
		t.Fatal(err)
	}
	if candidate.Equivalent {
		t.Fatal("body rewrite treated as formatting")
	}
	original, _ := s.ReadFile(head.Path)
	if original.Hash != head.Hash {
		t.Fatal("staging overwrote original")
	}
	if _, err = s.StageDocumentRepair(ctx, "r", action, "正文 123\n\n"); err != nil {
		t.Fatal(err)
	}
	if _, err = s.StageDocumentRepair(ctx, "r", action, "第三次"); err == nil {
		t.Fatal("unbounded repair")
	}
	canceled, cancel := context.WithCancel(ctx)
	cancel()
	if _, err = s.DecideDocumentRepair(canceled, candidate.ID, head.Hash, true); err == nil {
		t.Fatal("canceled apply succeeded")
	}
	original, _ = s.ReadFile(head.Path)
	if original.Hash != head.Hash {
		t.Fatal("cancel overwrote original")
	}
	if _, err = s.DecideDocumentRepair(ctx, candidate.ID, "stale", true); !errors.Is(err, ErrFileConflict) {
		t.Fatal(err)
	}
	applied, err := s.DecideDocumentRepair(ctx, candidate.ID, head.Hash, true)
	if err != nil {
		t.Fatal(err)
	}
	if applied.Hash != candidate.Hash {
		t.Fatal("wrong candidate applied")
	}
	versions, err := s.DocumentVersions(ctx, head.Path)
	if err != nil || len(versions) == 0 {
		t.Fatal("original not versioned", err)
	}
}
func TestDocumentChunkNeverPublishesPartialContentOrOverwritesNewHead(t *testing.T) {
	s := newTestService(t)
	ctx := t.Context()
	c, err := s.StageDocumentChunk(ctx, "run", "分块.md", "create", "```md\n", 1)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.ReadFile(c.Path); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("partial file exposed", err)
	}
	retry, err := s.StageDocumentChunk(ctx, "run", c.Path, "create", "```md\n", 1)
	if err != nil || retry.Content != c.Content {
		t.Fatal("retry changed draft", err)
	}
	c, err = s.StageDocumentChunk(ctx, "run", c.Path, "append", "完整代码\n```\n", 2)
	if err != nil {
		t.Fatal(err)
	}
	result, err := s.CommitDocumentChunk(ctx, "run", c.Path, c.Hash)
	if err != nil {
		t.Fatal(err)
	}
	if result.Hash != c.Hash {
		t.Fatal("wrong chunk head")
	}
	if _, err = s.StageDocumentChunk(ctx, "other", c.Path, "overwrite", "新草稿", 1); err != nil {
		t.Fatal(err)
	}
	head, err := s.WriteFile(ctx, c.Path, "用户新正文", "test")
	if err != nil {
		t.Fatal(err)
	}
	pending, err := s.DocumentChunk(ctx, "other", c.Path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.CommitDocumentChunk(ctx, "other", c.Path, pending.Hash); !errors.Is(err, ErrFileConflict) {
		t.Fatal(err)
	}
	got, _ := s.ReadFile(c.Path)
	if got.Hash != head.Hash {
		t.Fatal("overwrote concurrent user")
	}
}
func TestDocumentHistoryFollowsChineseFolderMovesAndDoesNotLeakAfterDeletion(t *testing.T) {
	s := newTestService(t)
	ctx := t.Context()
	a, err := s.CreateFile(ctx, "中文😀目录/稿件.md", "第一版")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.WriteFileIfUnchanged(ctx, a.Path, "第二版", "test", &a.Hash); err != nil {
		t.Fatal(err)
	}
	if _, err = s.Rename("中文😀目录", "移动目录"); err != nil {
		t.Fatal(err)
	}
	history, err := s.DocumentVersions(ctx, "移动目录/稿件.md")
	if err != nil || len(history) != 1 {
		t.Fatalf("lost identity: %+v %v", history, err)
	}
	if _, err = s.Delete("移动目录"); err != nil {
		t.Fatal(err)
	}
	if _, err = s.CreateFile(ctx, "移动目录/稿件.md", "同名新文稿"); err != nil {
		t.Fatal(err)
	}
	history, err = s.DocumentVersions(ctx, "移动目录/稿件.md")
	if err != nil || len(history) != 0 {
		t.Fatalf("history leaked: %+v %v", history, err)
	}
}
