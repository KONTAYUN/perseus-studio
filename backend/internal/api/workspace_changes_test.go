package api

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"webide-backend/internal/runqueue"
	"webide-backend/internal/store"
	"webide-backend/internal/usersvc"
	"webide-backend/internal/workspace"
)

func TestChangeAPIUsesOwnedLedgerAndReturnsEditorRefreshEvent(t *testing.T) {
	db, err := store.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	conversations := store.NewConversationStore(db)
	if err := conversations.Migrate(t.Context()); err != nil {
		t.Fatal(err)
	}
	factory := usersvc.NewFactory(db, t.TempDir(), nil)
	files, err := factory.Workspace("owner")
	if err != nil {
		t.Fatal(err)
	}
	before, err := files.CreateFile(t.Context(), "a.txt", "before")
	if err != nil {
		t.Fatal(err)
	}
	write, err := files.WriteFile(t.Context(), "a.txt", "after", "test")
	if err != nil {
		t.Fatal(err)
	}
	raw, err := json.Marshal(map[string]any{"changes": []workspace.FileChange{{Operation: "update", Path: "a.txt", Hash: write.Hash, BeforeHash: before.Hash, VersionID: write.VersionID}}})
	if err != nil {
		t.Fatal(err)
	}
	call, err := conversations.RecordToolCall(t.Context(), store.ToolCallInput{UserID: "owner", ToolName: "apply_workspace_patch", ResultJSON: string(raw)})
	if err != nil {
		t.Fatal(err)
	}
	queue := runqueue.New(1, 4)
	defer queue.Shutdown(t.Context())
	s := &Server{conversations: conversations, factory: factory, runQueue: queue}
	post := func(user, body string) *httptest.ResponseRecorder {
		req := httptest.NewRequest("POST", "/api/workspace/changes", strings.NewReader(body))
		req = req.WithContext(context.WithValue(req.Context(), ctxSessionKey, &SessionInfo{UserID: user}))
		rec := httptest.NewRecorder()
		s.handleWorkspaceChanges(rec, req)
		return rec
	}
	body := `{"tool_call_id":"` + call.ID + `","index":0,"path":"untrusted-path.txt","hash":"untrusted-hash"}`
	if rec := post("intruder", body); rec.Code != 404 {
		t.Fatalf("cross-user ledger accepted: %d %s", rec.Code, rec.Body.String())
	}
	if read, err := files.ReadFile("a.txt"); err != nil || read.Content != "after" {
		t.Fatal("unauthorized mutation")
	}
	rec := post("owner", body)
	if rec.Code != 200 {
		t.Fatalf("revert failed: %d %s", rec.Code, rec.Body.String())
	}
	var event map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &event); err != nil {
		t.Fatal(err)
	}
	if event["type"] != "workspace_file_changed" || event["path"] != "a.txt" || event["status"] != "success" {
		t.Fatalf("invalid editor refresh event: %+v", event)
	}
	if read, err := files.ReadFile("a.txt"); err != nil || read.Content != "before" {
		t.Fatalf("trusted snapshot not restored: %+v %v", read, err)
	}
}

func TestBodyLimitsRejectOversizedAndBoundChunkedRequests(t *testing.T) {
	s := &Server{}
	for _, chunked := range []bool{false, true} {
		t.Run(map[bool]string{false: "content-length", true: "chunked"}[chunked], func(t *testing.T) {
			req := httptest.NewRequest("POST", "/api/auth/register", strings.NewReader(strings.Repeat("x", (2<<20)+1)))
			if chunked {
				req.ContentLength = -1
			}
			rec := httptest.NewRecorder()
			s.withBodyLimits(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				_, err := io.Copy(io.Discard, r.Body)
				var tooLarge *http.MaxBytesError
				if !errors.As(err, &tooLarge) {
					t.Fatalf("chunked body not bounded: %v", err)
				}
				w.WriteHeader(http.StatusRequestEntityTooLarge)
			})).ServeHTTP(rec, req)
			if rec.Code != 413 {
				t.Fatalf("oversized request accepted: %d", rec.Code)
			}
		})
	}
}
