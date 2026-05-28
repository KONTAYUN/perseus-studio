package store

import (
	"path/filepath"
	"testing"
	"time"
)

func TestSessionStorePersistsSession(t *testing.T) {
	ctx := t.Context()
	db, err := Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("Open returned error: %v", err)
	}
	defer db.Close()
	users := NewUserStore(db)
	if err := users.Migrate(ctx); err != nil {
		t.Fatalf("users.Migrate returned error: %v", err)
	}
	user, err := users.CreateUser(ctx, "admin", "password", "admin")
	if err != nil {
		t.Fatalf("CreateUser returned error: %v", err)
	}
	sessions := NewSessionStore(db)
	if err := sessions.Migrate(ctx); err != nil {
		t.Fatalf("sessions.Migrate returned error: %v", err)
	}
	expires := time.Now().Add(time.Hour)
	if err := sessions.CreateSession(ctx, "token-a", user.ID, expires); err != nil {
		t.Fatalf("CreateSession returned error: %v", err)
	}

	reloaded := NewSessionStore(db)
	session, ok, err := reloaded.GetSession(ctx, "token-a", time.Now())
	if err != nil {
		t.Fatalf("GetSession returned error: %v", err)
	}
	if !ok {
		t.Fatalf("expected persisted session")
	}
	if session.UserID != user.ID || session.Username != user.Username || session.Role != user.Role {
		t.Fatalf("unexpected session: %#v", session)
	}
}

func TestSessionStoreDropsExpiredSession(t *testing.T) {
	ctx := t.Context()
	db, err := Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("Open returned error: %v", err)
	}
	defer db.Close()
	users := NewUserStore(db)
	if err := users.Migrate(ctx); err != nil {
		t.Fatalf("users.Migrate returned error: %v", err)
	}
	user, err := users.CreateUser(ctx, "admin", "password", "admin")
	if err != nil {
		t.Fatalf("CreateUser returned error: %v", err)
	}
	sessions := NewSessionStore(db)
	if err := sessions.Migrate(ctx); err != nil {
		t.Fatalf("sessions.Migrate returned error: %v", err)
	}
	if err := sessions.CreateSession(ctx, "expired-token", user.ID, time.Now().Add(-time.Minute)); err != nil {
		t.Fatalf("CreateSession returned error: %v", err)
	}

	_, ok, err := sessions.GetSession(ctx, "expired-token", time.Now())
	if err != nil {
		t.Fatalf("GetSession returned error: %v", err)
	}
	if ok {
		t.Fatalf("expected expired session to be rejected")
	}
	_, ok, err = sessions.GetSession(ctx, "expired-token", time.Now())
	if err != nil {
		t.Fatalf("second GetSession returned error: %v", err)
	}
	if ok {
		t.Fatalf("expected expired session to be removed")
	}
}

func TestSessionStoreDropsDisabledUserSession(t *testing.T) {
	ctx := t.Context()
	db, err := Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("Open returned error: %v", err)
	}
	defer db.Close()
	users := NewUserStore(db)
	if err := users.Migrate(ctx); err != nil {
		t.Fatalf("users.Migrate returned error: %v", err)
	}
	admin, err := users.CreateUser(ctx, "admin", "password", "admin")
	if err != nil {
		t.Fatalf("CreateUser admin returned error: %v", err)
	}
	user, err := users.CreateUser(ctx, "user", "password", "user")
	if err != nil {
		t.Fatalf("CreateUser user returned error: %v", err)
	}
	sessions := NewSessionStore(db)
	if err := sessions.Migrate(ctx); err != nil {
		t.Fatalf("sessions.Migrate returned error: %v", err)
	}
	if err := sessions.CreateSession(ctx, "token-user", user.ID, time.Now().Add(time.Hour)); err != nil {
		t.Fatalf("CreateSession returned error: %v", err)
	}
	if err := users.SetDisabled(ctx, user.ID, true); err != nil {
		t.Fatalf("SetDisabled returned error: %v", err)
	}

	_, ok, err := sessions.GetSession(ctx, "token-user", time.Now())
	if err != nil {
		t.Fatalf("GetSession returned error: %v", err)
	}
	if ok {
		t.Fatalf("expected disabled user session to be rejected")
	}
	if err := sessions.CreateSession(ctx, "token-admin", admin.ID, time.Now().Add(time.Hour)); err != nil {
		t.Fatalf("CreateSession admin returned error: %v", err)
	}
	_, ok, err = sessions.GetSession(ctx, "token-admin", time.Now())
	if err != nil {
		t.Fatalf("GetSession admin returned error: %v", err)
	}
	if !ok {
		t.Fatalf("expected active admin session")
	}
}

func TestSessionStoreDropsPendingUserSession(t *testing.T) {
	ctx := t.Context()
	db, err := Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("Open returned error: %v", err)
	}
	defer db.Close()
	users := NewUserStore(db)
	if err := users.Migrate(ctx); err != nil {
		t.Fatalf("users.Migrate returned error: %v", err)
	}
	user, err := users.CreateUser(ctx, "pending-user", "password", UserRolePending)
	if err != nil {
		t.Fatalf("CreateUser returned error: %v", err)
	}
	sessions := NewSessionStore(db)
	if err := sessions.Migrate(ctx); err != nil {
		t.Fatalf("sessions.Migrate returned error: %v", err)
	}
	if err := sessions.CreateSession(ctx, "token-pending", user.ID, time.Now().Add(time.Hour)); err != nil {
		t.Fatalf("CreateSession returned error: %v", err)
	}

	_, ok, err := sessions.GetSession(ctx, "token-pending", time.Now())
	if err != nil {
		t.Fatalf("GetSession returned error: %v", err)
	}
	if ok {
		t.Fatalf("expected pending user session to be rejected")
	}
	_, ok, err = sessions.GetSession(ctx, "token-pending", time.Now())
	if err != nil {
		t.Fatalf("second GetSession returned error: %v", err)
	}
	if ok {
		t.Fatalf("expected pending user session to be removed")
	}
}
