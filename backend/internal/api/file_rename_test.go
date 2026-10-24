package api

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"webide-backend/internal/store"
	"webide-backend/internal/usersvc"
)

func TestRenameDocumentProtectsContentAndDestination(t *testing.T) {
	for _, tc := range []struct {
		name, hash       string
		existing, legacy bool
		status           int
	}{
		{name: "matching hash", hash: "current", status: 200},
		{name: "stale hash", hash: "stale", status: 409},
		{name: "empty explicit hash", status: 409},
		{name: "existing markdown", hash: "current", existing: true, status: 409},
		{name: "legacy rename", legacy: true, status: 200},
	} {
		t.Run(tc.name, func(t *testing.T) {
			db, err := store.Open(filepath.Join(t.TempDir(), "test.db"))
			if err != nil {
				t.Fatal(err)
			}
			defer db.Close()
			factory := usersvc.NewFactory(db, t.TempDir(), nil)
			files, err := factory.Workspace("user1")
			if err != nil {
				t.Fatal(err)
			}
			const source = "# 原文\r\n\r\n**保留**所有内容\r\n"
			if _, err := files.WriteFile(t.Context(), "draft.txt", source, "test"); err != nil {
				t.Fatal(err)
			}
			file, err := files.ReadFile("draft.txt")
			if err != nil {
				t.Fatal(err)
			}
			if tc.existing {
				if _, err := files.WriteFile(t.Context(), "draft.md", "existing", "test"); err != nil {
					t.Fatal(err)
				}
			}
			body := map[string]string{"old_path": "draft.txt", "new_path": "draft.md"}
			if !tc.legacy {
				body["expected_hash"] = tc.hash
				if tc.hash == "current" {
					body["expected_hash"] = file.Hash
				}
			}
			payload, _ := json.Marshal(body)
			req := httptest.NewRequest("POST", "/api/files/rename", strings.NewReader(string(payload)))
			req = req.WithContext(context.WithValue(req.Context(), ctxSessionKey, &SessionInfo{UserID: "user1"}))
			rec := httptest.NewRecorder()
			(&Server{factory: factory}).handleFilesRename(rec, req)
			if rec.Code != tc.status {
				t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
			}
			path := "draft.txt"
			if tc.status == 200 {
				path = "draft.md"
			}
			result, err := files.ReadFile(path)
			if err != nil || result.Content != source {
				t.Fatalf("source lost: %v %q", err, result.Content)
			}
			if tc.status == 200 {
				if _, err := files.ReadFile("draft.txt"); err == nil {
					t.Fatal("old TXT still exists")
				}
			}
			if tc.existing {
				result, err := files.ReadFile("draft.md")
				if err != nil || result.Content != "existing" {
					t.Fatal("destination overwritten")
				}
			}
		})
	}
}
