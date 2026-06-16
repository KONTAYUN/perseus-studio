package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"webide-backend/internal/store"
)

func TestAdminUsageSummaryRequiresAdminAndReturnsAggregates(t *testing.T) {
	db, err := store.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("Open returned error: %v", err)
	}
	defer db.Close()
	users := store.NewUserStore(db)
	if err := users.Migrate(t.Context()); err != nil {
		t.Fatalf("users.Migrate returned error: %v", err)
	}
	admin, err := users.CreateUser(t.Context(), "usage-admin", "password", "admin")
	if err != nil {
		t.Fatalf("CreateUser admin returned error: %v", err)
	}
	ordinary, err := users.CreateUser(t.Context(), "usage-user", "password", "user")
	if err != nil {
		t.Fatalf("CreateUser user returned error: %v", err)
	}
	sessions := store.NewSessionStore(db)
	if err := sessions.Migrate(t.Context()); err != nil {
		t.Fatalf("sessions.Migrate returned error: %v", err)
	}
	configs := store.NewConfigStore(db)
	if err := configs.Migrate(t.Context()); err != nil {
		t.Fatalf("configs.Migrate returned error: %v", err)
	}
	usage := store.NewUsageStore(db)
	if err := usage.Migrate(t.Context()); err != nil {
		t.Fatalf("usage.Migrate returned error: %v", err)
	}
	if _, err := usage.AppendEvent(t.Context(), store.LLMUsageEventInput{
		UserID:       ordinary.ID,
		Status:       store.UsageStatusSucceeded,
		UsageSource:  store.UsageSourceProviderReported,
		InputTokens:  10,
		OutputTokens: 5,
	}); err != nil {
		t.Fatalf("AppendEvent returned error: %v", err)
	}
	server := &Server{
		configs:  configs,
		mux:      http.NewServeMux(),
		sessions: sessions,
		users:    users,
		usage:    usage,
	}
	server.register()

	userReq := httptest.NewRequest(http.MethodGet, "/api/admin/usage/summary?range=30d", nil)
	attachTestSession(t, server, userReq, "usage-user-token", ordinary)
	userRec := httptest.NewRecorder()
	server.Routes().ServeHTTP(userRec, userReq)
	if userRec.Code != http.StatusForbidden {
		t.Fatalf("expected ordinary user 403, got %d: %s", userRec.Code, userRec.Body.String())
	}

	adminReq := httptest.NewRequest(http.MethodGet, "/api/admin/usage/summary?range=30d", nil)
	attachTestSession(t, server, adminReq, "usage-admin-token", admin)
	adminRec := httptest.NewRecorder()
	server.Routes().ServeHTTP(adminRec, adminReq)
	if adminRec.Code != http.StatusOK {
		t.Fatalf("expected admin 200, got %d: %s", adminRec.Code, adminRec.Body.String())
	}
	var payload struct {
		Summary struct {
			Totals struct {
				RequestCount int64 `json:"request_count"`
				TotalTokens  int64 `json:"total_tokens"`
			} `json:"totals"`
		} `json:"summary"`
	}
	if err := json.Unmarshal(adminRec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("Unmarshal returned error: %v", err)
	}
	if payload.Summary.Totals.RequestCount != 1 || payload.Summary.Totals.TotalTokens != 15 {
		t.Fatalf("unexpected payload: %s", adminRec.Body.String())
	}
}
