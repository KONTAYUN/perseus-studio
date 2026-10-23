package api

import (
	"bytes"
	"context"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"testing"
	"webide-backend/internal/store"
	"webide-backend/internal/usersvc"
)

func TestBinaryDownloadPreservesBytesAndOwnership(t *testing.T) {
	db, err := store.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	factory := usersvc.NewFactory(db, t.TempDir(), nil)
	files, err := factory.Workspace("owner")
	if err != nil {
		t.Fatal(err)
	}
	body := []byte{0, 255, 128, 13, 10, 65, 0}
	if _, err := files.WriteFile(t.Context(), "中文.pdf", string(body), "test"); err != nil {
		t.Fatal(err)
	}
	s := &Server{factory: factory}
	get := func(user, path, rangeHeader string) *httptest.ResponseRecorder {
		req := httptest.NewRequest("GET", "/api/files/download?path="+url.QueryEscape(path), nil)
		req = req.WithContext(context.WithValue(req.Context(), ctxSessionKey, &SessionInfo{UserID: user}))
		req.Header.Set("Range", rangeHeader)
		rec := httptest.NewRecorder()
		s.handleFilesDownload(rec, req)
		return rec
	}
	full := get("owner", "中文.pdf", "")
	if full.Code != 200 || !bytes.Equal(full.Body.Bytes(), body) || full.Header().Get("Content-Type") != "application/octet-stream" || full.Header().Get("Content-Disposition") == "" {
		t.Fatalf("binary corrupted: %d %v", full.Code, full.Body.Bytes())
	}
	partial := get("owner", "中文.pdf", "bytes=1-3")
	if partial.Code != 206 || !bytes.Equal(partial.Body.Bytes(), body[1:4]) {
		t.Fatal("range corrupted")
	}
	for _, tc := range []struct{ user, path string }{{"other", "中文.pdf"}, {"owner", "../中文.pdf"}, {"owner", ".trash/中文.pdf"}} {
		rec := get(tc.user, tc.path, "")
		if rec.Code < 400 || bytes.Equal(rec.Body.Bytes(), body) {
			t.Fatalf("unsafe download %s %s %d", tc.user, tc.path, rec.Code)
		}
	}
}

func TestForeignMutationsReturnNotFoundWithoutChanges(t *testing.T) {
	env := newAPITestAIEnv(t)
	tasks := store.NewTaskStore(env.db)
	if err := tasks.Migrate(t.Context()); err != nil {
		t.Fatal(err)
	}
	runs := store.NewAgentRunStore(env.db)
	if err := runs.Migrate(t.Context()); err != nil {
		t.Fatal(err)
	}
	conv, err := env.conversations.CreateConversation(t.Context(), store.CreateConversationInput{UserID: env.admin.ID})
	if err != nil {
		t.Fatal(err)
	}
	task, err := tasks.CreateTask(t.Context(), store.CreateTaskInput{ID: "owned", UserID: env.admin.ID, Type: "mock", PayloadJSON: "{}"})
	if err != nil {
		t.Fatal(err)
	}
	s := &Server{conversations: env.conversations, runs: runs, store: tasks}
	for _, tc := range []struct{ method, path string }{{"DELETE", "/api/conversations/" + conv.ID}, {"POST", "/api/tasks/" + task.ID + "/cancel"}} {
		req := httptest.NewRequest(tc.method, tc.path, nil)
		req = req.WithContext(context.WithValue(req.Context(), ctxSessionKey, &SessionInfo{UserID: env.user.ID}))
		rec := httptest.NewRecorder()
		if tc.method == "DELETE" {
			s.handleConversationRoutes(rec, req)
		} else {
			s.handleTaskRoutes(rec, req)
		}
		if rec.Code != 404 {
			t.Fatalf("foreign operation status %d", rec.Code)
		}
	}
	if _, err := env.conversations.GetConversation(t.Context(), env.admin.ID, conv.ID); err != nil {
		t.Fatal("owner conversation lost", err)
	}
	kept, err := tasks.GetTask(t.Context(), env.admin.ID, task.ID)
	if err != nil || kept.Status != store.TaskPending || kept.CancelRequestedAt.Valid {
		t.Fatal("owner task modified", err)
	}
}
