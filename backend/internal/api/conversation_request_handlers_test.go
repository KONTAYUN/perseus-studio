package api

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strings"
	"testing"

	"webide-backend/internal/store"
	"webide-backend/internal/usersvc"
)

func TestParseChatReferencesKeepsWorkspaceFilesMetadataOnly(t *testing.T) {
	db, err := store.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("Open returned error: %v", err)
	}
	t.Cleanup(func() {
		_ = db.Close()
	})

	skills := store.NewSkillStore(db)
	if err := skills.Migrate(context.Background()); err != nil {
		t.Fatalf("skills.Migrate returned error: %v", err)
	}
	factory := usersvc.NewFactory(db, filepath.Join(t.TempDir(), "users"), skills)
	files, err := factory.Workspace("user_1")
	if err != nil {
		t.Fatalf("Workspace returned error: %v", err)
	}
	if _, err := files.CreateFile(context.Background(), "drafts/secret.txt", "SECRET WORKSPACE BODY"); err != nil {
		t.Fatalf("CreateFile workspace ref returned error: %v", err)
	}
	if _, err := files.CreateFile(context.Background(), "drafts/current.txt", "CURRENT FILE BODY"); err != nil {
		t.Fatalf("CreateFile current ref returned error: %v", err)
	}

	server := &Server{factory: factory}
	form := url.Values{}
	form.Set("workspace_refs_json", `[{"id":"ref_1","name":"secret.txt","path":"drafts/secret.txt","file_type":"text"}]`)
	form.Set("editor_context_json", `{"id":"editor_panels_context","type":"editor_panels_context","panels":[{"panel_id":"editor_1","index":1,"position_label":"current","active_file_path":"drafts/current.txt","active_file_name":"current.txt","open_tabs":["drafts/current.txt"]}]}`)
	req := httptest.NewRequest("POST", "/api/conversations/conv/messages", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req = req.WithContext(context.WithValue(req.Context(), ctxSessionKey, &SessionInfo{UserID: "user_1", Role: "user"}))

	payloads, metadata, err := server.parseChatReferences(req)
	if err != nil {
		t.Fatalf("parseChatReferences returned error: %v", err)
	}
	if len(payloads) != 2 {
		t.Fatalf("expected two payloads, got %#v", payloads)
	}
	for _, payload := range payloads {
		if payload.Text != "" || payload.IncludedInContext || payload.Truncated {
			t.Fatalf("workspace/current payload must be metadata only, got %#v", payload)
		}
		if payload.Kind == "workspace_file" && payload.Path == "" {
			t.Fatalf("expected payload path, got %#v", payload)
		}
		if payload.Kind == "editor_panels_context" && (len(payload.EditorPanels) != 1 || payload.EditorPanels[0].ActiveFilePath != "drafts/current.txt") {
			t.Fatalf("expected path-only editor panel metadata, got %#v", payload)
		}
	}
	if len(metadata) != 2 {
		t.Fatalf("expected two metadata items, got %#v", metadata)
	}
	for _, item := range metadata {
		if item.IncludedInContext || item.Truncated {
			t.Fatalf("workspace/current metadata must not claim context inclusion, got %#v", item)
		}
		if item.Kind == "workspace_file" && item.Path == "" {
			t.Fatalf("expected metadata path, got %#v", item)
		}
	}

	rawMetadata, err := json.Marshal(metadata)
	if err != nil {
		t.Fatalf("Marshal metadata returned error: %v", err)
	}
	for _, forbidden := range []string{"SECRET WORKSPACE BODY", "CURRENT FILE BODY"} {
		if strings.Contains(string(rawMetadata), forbidden) {
			t.Fatalf("metadata leaked file body %q: %s", forbidden, rawMetadata)
		}
	}
}
